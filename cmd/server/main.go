package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
)

func main() {
	setup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	telemetry, err := observability.Setup(setup, os.Stdout)
	cancel()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("observability configuration failed", "error", err)
		os.Exit(1)
	}
	err = run(telemetry.Logger)
	if err != nil {
		telemetry.Logger.Error("server stopped; check authentication, tenant and connection configuration")
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	flushErr := telemetry.Shutdown(shutdown)
	cancel()
	if err != nil || flushErr != nil {
		os.Exit(1)
	}
}
func run(log *slog.Logger) error {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	demo := flag.Bool("demo", false, "isolated memory store with generated data; external integrations disabled")
	memory := flag.Bool("memory", false, "ephemeral development store")
	tenantsFile := flag.String("tenants-file", os.Getenv("TENANTS_FILE"), "server-owned YAML/JSON tenant configuration")
	authFile := flag.String("auth-file", os.Getenv("AUTH_CONFIG_FILE"), "server-owned OIDC/LDAP identity configuration")
	extensionLock := flag.String("extension-lock", "", "Administrator-owned lockfile shared across tenants; local files only")
	processRole := flag.String("process-role", "all", "all, control-plane (no scheduled collection), or collector (collection/evaluation only; no HTTP)")
	collectionWorkers := flag.Int("collection-workers", 4, "maximum concurrent scheduled collection jobs per process (1-32)")
	featureList := flag.String("features", os.Getenv(features.Env), "experimental features to turn on: comma-separated gated feature IDs, all or none (default from NINES_FEATURES; the demo turns all on)")
	flag.Parse()
	if *processRole != "all" && *processRole != "control-plane" && *processRole != "collector" {
		return fmt.Errorf("invalid process role")
	}
	if *collectionWorkers < 1 || *collectionWorkers > 32 {
		return fmt.Errorf("collection workers must be between 1 and 32")
	}
	if *processRole == "collector" && (*demo || *memory) {
		return fmt.Errorf("collector role requires persistent tenant stores")
	}

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	token := os.Getenv("API_TOKEN")
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) && len(token) < 24 && *authFile == "" && (*demo || *tenantsFile == "") {
		return fmt.Errorf("non-loopback listening requires API_TOKEN with at least 24 characters")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var authConfig identity.Config
	if *authFile != "" {
		authConfig, err = identity.LoadConfig(*authFile, os.Getenv)
		if err != nil {
			return err
		}
		if *demo {
			return fmt.Errorf("demo and authentication configuration cannot be combined")
		}
	}
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	runtimes, err := initTenantsWithOptions(startup, *tenantsFile, *demo, *memory, token, *authFile != "", *processRole == "collector")
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		for _, runtime := range runtimes {
			runtime.Close()
		}
	}()
	if *demo || *memory {
		log.Warn("using ephemeral tenant stores", "demo", *demo, "tenants", len(runtimes))
	}
	var sharedTemplates *slotemplates.Registry
	if *extensionLock != "" {
		sharedTemplates, err = slotemplates.Load(*extensionLock)
		if err != nil {
			return err
		}
	}
	gates, err := serverFeatures(*featureList, *demo, flagSet("features"))
	if err != nil {
		return err
	}
	if err = checkProviderGates(runtimes, gates); err != nil {
		return err
	}
	log.Info("feature gates", "experimental", gates.List())
	// Settings every tenant gets, including those created while running.
	prepare := func(workspace *httpapi.Workspace) {
		workspace.Features = gates
		workspace.Service.IncidentsEnabled = gates.Enabled("incidents")
		if sharedTemplates != nil {
			workspace.Templates = sharedTemplates
		}
	}
	workspaces := make([]httpapi.Workspace, 0, len(runtimes))
	configured := map[string]bool{}
	for _, runtime := range runtimes {
		workspace := runtime.Workspace
		if sharedTemplates != nil && workspace.Templates != nil {
			return fmt.Errorf("configure shared or tenant-local extension locks, not both")
		}
		prepare(&workspace)
		workspaces = append(workspaces, workspace)
		configured[workspace.ID] = true
	}
	// Tenants created in the platform console live in the identity database,
	// so they exist only when identity is configured and storage is durable.
	registry := *authFile != "" && !*demo && !*memory
	options := tenantOptions{identityEnabled: true, collectorOnly: *processRole == "collector"}
	services := make([]*service.Service, 0, len(runtimes))
	for _, runtime := range runtimes {
		services = append(services, runtime.Workspace.Service)
	}
	scheduler, err := service.NewCollectionScheduler(services, *collectionWorkers)
	if err != nil {
		return err
	}
	if *processRole == "collector" {
		if registry {
			repo, err := openIdentityRepository(ctx)
			if err != nil {
				return err
			}
			defer repo.Close()
			reconciler := &tenantReconciler{repo: repo, opts: options, configured: configured, prepare: prepare, scheduler: scheduler, log: log}
			defer reconciler.Close()
			if err = reconciler.Reconcile(ctx); err != nil {
				return fmt.Errorf("tenant registry: %w", err)
			}
			go reconciler.Run(ctx)
		}
		log.Info("collection worker started", "tenants", len(services), "workers", *collectionWorkers)
		scheduler.Run(ctx, log)
		return nil
	}
	var auth *identity.Service
	var identityRepo identity.Repository
	if *authFile != "" {
		if *memory {
			identityRepo = identity.NewMemory()
		} else if identityRepo, err = openIdentityRepository(ctx); err != nil {
			return err
		}
		repo := identityRepo
		setup, stopSetup := context.WithTimeout(ctx, 60*time.Second)
		known := map[string]bool{}
		for _, w := range workspaces {
			known[w.ID] = true
		}
		auth, err = identity.New(setup, authConfig, repo, known)
		stopSetup()
		if err != nil {
			repo.Close()
			return err
		}
		legacy := map[string]bool{}
		for _, workspace := range workspaces {
			for _, grant := range workspace.Grants {
				if grant.Role == "owner" {
					legacy[workspace.ID] = true
				}
			}
		}
		if err = auth.EnsureAdministration(ctx, legacy); err != nil {
			auth.Close()
			return err
		}
		defer auth.Close()
	}
	router, err := httpapi.NewRouter(workspaces, log, auth)
	if err != nil {
		return err
	}
	var handler http.Handler = router
	var workers sync.WaitGroup
	if auth != nil {
		workers.Go(func() { auth.Run(ctx) })
	}
	if *processRole == "all" {
		workers.Go(func() { scheduler.Run(ctx, log) })
	}
	tenantWorkers := map[string]func(){}
	for _, runtime := range runtimes {
		tenantWorkers[runtime.Workspace.ID] = startTenantWorkers(ctx, runtime, *demo, log)
	}
	defer func() {
		for _, stop := range tenantWorkers {
			stop()
		}
	}()
	if registry && auth != nil {
		reconciler := &tenantReconciler{repo: identityRepo, opts: options, configured: configured, prepare: prepare, router: router, auth: auth, scheduler: scheduler, log: log,
			workers: func(runtime tenantRuntime) func() { return startTenantWorkers(ctx, runtime, false, log) }}
		defer reconciler.Close()
		// Serve every registered tenant from the first request.
		if err = reconciler.Reconcile(ctx); err != nil {
			return fmt.Errorf("tenant registry: %w", err)
		}
		workers.Go(func() { reconciler.Run(ctx) })
	}
	server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		stop()
		workers.Wait()
		return err
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	log.Info("SLO workspace listening", "address", listener.Addr().String(), "demo", *demo, "tenants", len(runtimes))
	select {
	case err = <-result:
		stop()
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
		log.Error("HTTP shutdown", "error", shutdownErr)
	}
	workers.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// startTenantWorkers runs one tenant's background work: measurement checks,
// rule reconciliation, reviews, workflows and alert delivery. The returned
// function stops it and waits, for a tenant retired while the server runs.
func startTenantWorkers(parent context.Context, runtime tenantRuntime, demo bool, log *slog.Logger) func() {
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	svc := runtime.Workspace.Service
	tenantLog := log.With("tenant", runtime.Workspace.ID)
	workers.Go(func() { svc.RunMeasurementMonitor(ctx, tenantLog) })
	ruleManager := runtime.Workspace.Rules
	workers.Go(func() { svc.RunRuleReconciler(ctx, ruleManager, tenantLog) })
	intentConversations := runtime.Workspace.IntentConversations
	workers.Go(func() { intentConversations.RunWorker(ctx, tenantLog) })
	workers.Go(func() { intentConversations.RunOutcomeObserver(ctx, tenantLog) })
	if intentConversations.Archive != nil {
		workers.Go(func() { intentConversations.Archive.Run(ctx, tenantLog) })
	}
	replays := runtime.Workspace.Replays
	workers.Go(func() { replays.RunRetention(ctx, tenantLog) })
	reviews := runtime.Workspace.Investigations
	workers.Go(func() { reviews.RunWorker(ctx, tenantLog) })
	wf := runtime.Workspace.Workflows
	workers.Go(func() { runWorkflowRecovery(ctx, wf, tenantLog) })
	if demo {
		workers.Go(func() { runDemo(ctx, svc, tenantLog) })
	}
	if runtime.WorkflowWebhook != nil {
		workers.Go(func() { runtime.WorkflowWebhook.Run(ctx, tenantLog) })
	}
	if runtime.Webhook != "" {
		dispatcher := service.NewDispatcher(svc.Store, runtime.Webhook)
		dispatcher.TenantID = runtime.Workspace.ID
		workers.Go(func() { dispatcher.Run(ctx, tenantLog) })
	}
	return func() {
		cancel()
		workers.Wait()
	}
}

// openIdentityRepository opens the identity database: AUTH_DATABASE_URL, or
// DATABASE_URL when identity shares the server database.
func openIdentityRepository(ctx context.Context) (identity.Repository, error) {
	db := os.Getenv("AUTH_DATABASE_URL")
	if db == "" {
		db = os.Getenv("DATABASE_URL")
	}
	if db == "" {
		return nil, fmt.Errorf("authentication requires AUTH_DATABASE_URL or DATABASE_URL")
	}
	repo, err := identity.NewPostgres(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("identity database: %w", err)
	}
	return repo, nil
}

func seed(ctx context.Context, svc *service.Service) error {
	now := svc.Now().UTC().Truncate(time.Minute)
	// Classification labels so the demo shows the roll-up by team and product.
	demoLabels := []map[string]string{
		{"team": "platform", "product": "storefront"},
		{"team": "payments", "product": "storefront"},
		{"team": "platform", "product": "fulfillment"},
	}
	for n, name := range []string{"API availability · demo", "Checkout success · demo", "Background jobs · demo"} {
		id := []string{"api-availability", "checkout-success", "background-jobs"}[n]
		c := model.SLO{Labels: demoLabels[n], ID: id, Project: "demo", Service: []string{"gateway", "checkout", "workers"}[n], Name: name, Target: .999, WindowSeconds: 86400, StepSeconds: 60, Policies: []model.Policy{{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, ForSeconds: 0, MinCoverage: 1}, {Name: "sustained-burn", Severity: "warning", ShortWindowSeconds: 1800, LongWindowSeconds: 21600, BurnRate: 6, ForSeconds: 0, MinCoverage: 1}}}
		if err := svc.Create(ctx, c); err != nil {
			return err
		}
		if err := refreshDemoSLO(ctx, svc, id, n, now); err != nil {
			return err
		}
	}
	registerDemoSource(svc)
	for _, c := range demoTypedSLOs() {
		if err := svc.Create(ctx, c); err != nil {
			return err
		}
		// Backfill the same chart history the ingested demo SLOs have, one
		// step inside the retained range: seeding can cross a minute
		// boundary, which moves the retention cutoff forward by one step.
		history := min(2880*c.StepSeconds, c.RetentionSeconds()-c.StepSeconds)
		if _, err := svc.Collect(ctx, c.ID, now.Add(-time.Duration(history)*time.Second), now); err != nil {
			return err
		}
	}
	for _, text := range []string{examples.Checkout, examples.AgentResolution} {
		j, err := model.ParseJourney([]byte(text))
		if err != nil {
			return err
		}
		if j.Spec.Agentic != nil {
			// Show a journey run objective: judged runs against the contract.
			j.Spec.RunObjective = &v1.RunObjective{Target: .7, WindowSeconds: 86400, MinRuns: 5}
		}
		rev, err := svc.ApplyJourney(ctx, j, 0)
		if err != nil {
			return err
		}
		if j.Spec.Agentic != nil {
			for i := 0; i < 8; i++ {
				cost := .08 + float64(i)*.015
				steps := 5 + i
				approval := i != 6
				outcome := "success"
				if i == 7 {
					outcome = "handoff"
				}
				if i == 5 {
					cost = .42
				}
				end := now.Add(-time.Duration(i+1) * 7 * time.Minute)
				_, err = svc.RecordRun(ctx, j.Metadata.Name, model.RunEvidence{ID: fmt.Sprintf("demo-run-%d", i+1), JourneyRevision: rev.Revision, StartedAt: end.Add(-45 * time.Second), FinishedAt: end, Outcome: outcome, CostUSD: &cost, Steps: &steps, ApprovalObserved: &approval})
				if err != nil {
					return err
				}
			}
		}
	}
	graph := model.ServiceDependencyMap{APIVersion: model.APIVersion, Kind: "ServiceDependencyMap", Metadata: model.ResourceMetadata{Name: "commerce", Project: "demo"}, Spec: v1.DependencyMapSpec{DisplayName: "Commerce services", Services: []v1.MapService{{Name: "gateway", DisplayName: "API gateway", SLORefs: []string{"api-availability"}}, {Name: "checkout", DisplayName: "Checkout", SLORefs: []string{"checkout-success"}}, {Name: "workers", DisplayName: "Background workers", SLORefs: []string{"background-jobs"}}, {Name: "payments", DisplayName: "Payment provider", SLORefs: []string{}}}, Dependencies: []v1.DeclaredDependency{{Caller: "gateway", Callee: "checkout", Critical: true, Description: "Checkout requests"}, {Caller: "checkout", Callee: "payments", Critical: true, Description: "Payment authorization"}, {Caller: "checkout", Callee: "workers", Critical: false, Description: "Asynchronous fulfillment"}}}}
	_, err := svc.ApplyDefinition(ctx, model.Definition{DependencyMap: &graph}, 0)
	return err
}

// Keep the synthetic demo current without contacting any external source.
func refreshDemoSLO(ctx context.Context, svc *service.Service, id string, n int, now time.Time) error {
	record, err := svc.Store.Read(ctx, id)
	if err != nil {
		return err
	}
	// Refresh the retained history horizon, including the minimum chart history
	// for short-window objectives. Objective evaluation still uses its own window.
	// One step inside the retained range, in case the clock crosses a step
	// boundary before the samples are stored.
	count := min(2880, int(record.SLO.RetentionSeconds()/record.SLO.StepSeconds)-1)
	points := make([]model.Point, 0, count)
	for i := count - 1; i >= 0; i-- {
		total := float64(10000 + (i%17)*100)
		bad := .0002
		if n == 1 && int64(i)*record.SLO.StepSeconds < 1800 && svc.TenantID != "sandbox" {
			bad = .03
		}
		if n == 2 {
			bad = .0007
		}
		points = append(points, model.Point{Time: now.Add(-time.Duration(i*int(record.SLO.StepSeconds)) * time.Second), Good: total * (1 - bad), Total: total})
	}
	return svc.Ingest(ctx, id, points)
}
func runDemo(ctx context.Context, svc *service.Service, log *slog.Logger) {
	last := svc.Now().UTC().Truncate(time.Minute)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := svc.Now().UTC().Truncate(time.Minute)
			if now.Equal(last) {
				continue
			}
			for n, id := range []string{"api-availability", "checkout-success", "background-jobs"} {
				if err := refreshDemoSLO(ctx, svc, id, n, now); err != nil && ctx.Err() == nil {
					log.Error("refresh demo samples", "error", err)
				}
			}
			if err := collectDemo(ctx, svc, last, now); err != nil && ctx.Err() == nil {
				log.Error("collect synthetic demo telemetry", "error", err)
			}
			last = now
		}
	}
}
