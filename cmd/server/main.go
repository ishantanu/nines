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
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/service"
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
	flag.Parse()
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
	runtimes, err := initTenantsWithIdentity(startup, *tenantsFile, *demo, *memory, token, *authFile != "")
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
	workspaces := make([]httpapi.Workspace, 0, len(runtimes))
	for _, runtime := range runtimes {
		workspaces = append(workspaces, runtime.Workspace)
	}
	var auth *identity.Service
	if *authFile != "" {
		var repo identity.Repository
		if *memory {
			repo = identity.NewMemory()
		} else {
			db := os.Getenv("AUTH_DATABASE_URL")
			if db == "" {
				db = os.Getenv("DATABASE_URL")
			}
			if db == "" {
				return fmt.Errorf("authentication requires AUTH_DATABASE_URL or DATABASE_URL")
			}
			repo, err = identity.NewPostgres(ctx, db)
			if err != nil {
				return fmt.Errorf("identity database: %w", err)
			}
		}
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
	handler, err := httpapi.NewMulti(workspaces, log, auth)
	if err != nil {
		return err
	}
	var workers sync.WaitGroup
	if auth != nil {
		workers.Go(func() { auth.Run(ctx) })
	}
	for _, runtime := range runtimes {
		svc := runtime.Workspace.Service
		tenantLog := log.With("tenant", runtime.Workspace.ID)
		workers.Go(func() { svc.Run(ctx, tenantLog) })
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
		if *demo {
			workers.Go(func() { runDemo(ctx, svc, tenantLog) })
		}
		if runtime.Webhook != "" {
			dispatcher := service.NewDispatcher(svc.Store, runtime.Webhook)
			dispatcher.TenantID = runtime.Workspace.ID
			workers.Go(func() { dispatcher.Run(ctx, tenantLog) })
		}
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

func seed(ctx context.Context, svc *service.Service) error {
	now := svc.Now().UTC().Truncate(time.Minute)
	for n, name := range []string{"API availability · demo", "Checkout success · demo", "Background jobs · demo"} {
		id := []string{"api-availability", "checkout-success", "background-jobs"}[n]
		c := model.SLO{ID: id, Project: "demo", Service: []string{"gateway", "checkout", "workers"}[n], Name: name, Target: .999, WindowSeconds: 86400, StepSeconds: 60, Policies: []model.Policy{{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, ForSeconds: 0, MinCoverage: 1}, {Name: "sustained-burn", Severity: "warning", ShortWindowSeconds: 1800, LongWindowSeconds: 21600, BurnRate: 6, ForSeconds: 0, MinCoverage: 1}}}
		if err := svc.Create(ctx, c); err != nil {
			return err
		}
		if err := refreshDemoSLO(ctx, svc, id, n, now); err != nil {
			return err
		}
	}
	for _, text := range []string{examples.Checkout, examples.AgentResolution} {
		j, err := model.ParseJourney([]byte(text))
		if err != nil {
			return err
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
	// A shorter configured window also shortens accepted sample retention.
	count := min(2880, int(2*record.SLO.WindowSeconds/record.SLO.StepSeconds))
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
			last = now
		}
	}
}
