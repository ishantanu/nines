package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/auditarchive"
	"github.com/ishantanu/nines/internal/datasources"
	"github.com/ishantanu/nines/internal/identity"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/intentmodel"
	"github.com/ishantanu/nines/internal/investigation"
	"github.com/ishantanu/nines/internal/kubediscovery"
	"github.com/ishantanu/nines/internal/operator"
	"github.com/ishantanu/nines/internal/providers"
	"github.com/ishantanu/nines/internal/replay"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/systemone"
	"github.com/ishantanu/nines/internal/telemetryprovider"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
	"github.com/ishantanu/nines/internal/workflowhook"
)

type tenantRuntime struct {
	WorkflowWebhook     *workflowhook.Dispatcher
	ManagedSources      *datasources.Registry
	IntentConversations intentflow.Repository
	Replays             replay.Repository
	Workspace           httpapi.Workspace
	Repository          workflow.Repository
	Investigations      investigation.Repository
	Webhook             string
}

func (r tenantRuntime) Close() {
	if r.ManagedSources != nil {
		r.ManagedSources.Close()
	}
	if r.IntentConversations != nil {
		r.IntentConversations.Close()
	}
	if r.Replays != nil {
		r.Replays.Close()
	}
	if r.Investigations != nil {
		r.Investigations.Close()
	}
	r.Repository.Close()
	r.Workspace.Service.Store.Close()
}
func initTenants(ctx context.Context, file string, demo, memory bool, token string) ([]tenantRuntime, error) {
	return initTenantsWithIdentity(ctx, file, demo, memory, token, false)
}
func initTenantsWithIdentity(ctx context.Context, file string, demo, memory bool, token string, identityEnabled bool) ([]tenantRuntime, error) {
	var configured []tenancy.Resolved
	if demo {
		for _, t := range []struct{ id, name string }{{"default", "Commerce · demo"}, {"sandbox", "Sandbox · demo"}} {
			r := tenancy.Resolved{Config: tenancy.Tenant{ID: t.id, Name: t.name, Source: tenancy.Source{Kind: "manual"}}}
			if token != "" {
				r.Grants = []tenancy.Grant{{Token: token, Role: "owner"}}
			}
			configured = append(configured, r)
		}
	} else if file != "" {
		var err error
		if identityEnabled {
			configured, err = tenancy.LoadForIdentity(file, os.Getenv)
		} else {
			configured, err = tenancy.Load(file, os.Getenv)
		}
		if err != nil {
			return nil, err
		}
	} else {
		r := tenancy.Resolved{Config: tenancy.Tenant{ID: "default", Name: "Default workspace", Source: tenancy.Source{Kind: "manual"}}, Webhook: os.Getenv("ALERT_WEBHOOK_URL")}
		if token != "" {
			r.Grants = []tenancy.Grant{{Token: token, Role: "owner"}}
		}
		source, mimir := os.Getenv("PROMETHEUS_URL"), os.Getenv("MIMIR_URL")
		if source != "" && mimir != "" {
			return nil, fmt.Errorf("configure PROMETHEUS_URL or MIMIR_URL, not both")
		}
		if mimir != "" {
			r.Config.Source = tenancy.Source{Kind: "mimir", URL: mimir, MimirOrgID: os.Getenv("MIMIR_TENANT_ID"), BasicUsername: os.Getenv("MIMIR_USERNAME")}
			r.BearerToken = os.Getenv("MIMIR_BEARER_TOKEN")
			r.BasicPassword = os.Getenv("MIMIR_PASSWORD")
		} else if source != "" {
			r.Config.Source = tenancy.Source{Kind: "prometheus", URL: source}
			r.BearerToken = os.Getenv("PROMETHEUS_BEARER_TOKEN")
		}
		configured = []tenancy.Resolved{r}
	}
	out := []tenantRuntime{}
	fail := func(err error) ([]tenantRuntime, error) {
		for _, r := range out {
			r.Close()
		}
		return nil, err
	}
	for _, t := range configured {
		var scopedDSN string
		var st store.Store
		var repository workflow.Repository
		if demo || memory {
			st = store.NewMemory()
			repository = workflow.NewMemory()
		} else {
			connection := os.Getenv("DATABASE_URL")
			if connection == "" {
				return fail(fmt.Errorf("set DATABASE_URL, or explicitly use -demo / -memory"))
			}
			scoped, err := store.TenantDatabaseURL(ctx, connection, t.Config.ID)
			if err != nil {
				return fail(fmt.Errorf("tenant %s database namespace: %w", t.Config.ID, err))
			}
			scopedDSN = scoped
			st, err = store.NewPostgres(ctx, scoped)
			if err != nil {
				return fail(fmt.Errorf("tenant %s initialize database: %w", t.Config.ID, err))
			}
			repository, err = workflow.NewPostgres(ctx, scoped)
			if err != nil {
				st.Close()
				return fail(fmt.Errorf("tenant %s initialize workflows: %w", t.Config.ID, err))
			}
		}
		var collector service.Collector
		if t.Config.Source.Kind != "manual" {
			var err error
			collector, err = openProvider(t.Config.Source, t.Credentials, t.BearerToken, t.BasicPassword)
			if err != nil {
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("tenant %s source configuration: %w", t.Config.ID, err))
			}
		}
		if t.Webhook != "" {
			u, err := url.Parse(t.Webhook)
			if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("tenant %s invalid alert webhook URL", t.Config.ID))
			}
		}
		sources := map[string]service.MetricSource{}
		ruleTargets := map[string]rules.Target{}
		connections := []tenancy.Connection{{Name: "primary", Source: t.Config.Source}}
		addTarget := func(name string, source tenancy.Source, bearer, basicPassword, rulesBearer, rulesPassword string, credentials map[string]string) {
			if cfg := source.Rules; cfg != nil {
				username := source.BasicUsername
				if cfg.BearerTokenEnv != "" || cfg.BasicUsername != "" {
					bearer = rulesBearer
					basicPassword = rulesPassword
					username = cfg.BasicUsername
				}
				ruleTargets[name] = rules.Target{NewRelicAccountID: source.Options["accountID"], NewRelicEntityGUID: cfg.EntityGUID, NewRelicUserKey: credentials["userKey"], DatadogAPIKey: credentials["apiKey"], DatadogApplicationKey: credentials["applicationKey"], Connection: name, Kind: source.Kind, QueryURL: source.URL, RulerURL: cfg.RulerURL, MimirOrgID: source.MimirOrgID, GoogleProjectID: source.Options["projectID"], GoogleServiceID: source.Options["serviceID"], GoogleAuth: source.Options["auth"], Dataset: source.Options["dataset"], Namespace: cfg.Namespace, Directory: cfg.Directory, Reload: cfg.Reload, AutoDeploy: cfg.AutoDeploy, Publisher: cfg.Publisher, BearerToken: bearer, BasicUsername: username, BasicPassword: basicPassword}
				if cfg.Mode == "operator" {
					target := ruleTargets[name]
					target.ProviderKind = source.Kind
					target.Kind = "operator"
					ruleTargets[name] = target
				}
			}
		}
		addTarget("primary", t.Config.Source, t.BearerToken, t.BasicPassword, t.RulesBearerToken, t.RulesBasicPassword, t.Credentials)
		for _, connection := range t.Connections {
			cfg := connection.Config
			var named service.Collector
			if cfg.Kind != "manual" {
				var err error
				named, err = openProvider(cfg.Source, connection.Credentials, connection.BearerToken, connection.BasicPassword)
				if err != nil {
					repository.Close()
					st.Close()
					return fail(fmt.Errorf("connection %s initialization: %w", cfg.Name, err))
				}
			}
			sources[cfg.Name] = service.MetricSource{Kind: cfg.Kind, Collector: named, CollectionMode: cfg.CollectionMode}
			connections = append(connections, cfg)
			addTarget(cfg.Name, cfg.Source, connection.BearerToken, connection.BasicPassword, connection.RulesBearerToken, connection.RulesBasicPassword, connection.Credentials)
		}

		var reviewer rm.Model
		svc := service.New(st, collector)
		if !demo && t.Config.KubernetesDiscovery != nil {
			reader, err := kubediscovery.InCluster(*t.Config.KubernetesDiscovery)
			if err != nil {
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("tenant %s Kubernetes discovery: %w", t.Config.ID, err))
			}
			svc.KubernetesDiscovery = reader
		}
		if !demo {
			drafter, err := intentmodel.FromEnvironment(t.Config.IntentModel, os.Getenv)
			if err != nil {
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("tenant %s intent model configuration: %w", t.Config.ID, err))
			}
			if drafter != nil {
				svc.IntentDrafter = drafter
				reviewer = drafter
			}
			if t.Config.ReviewModel != nil {
				configured, err := intentmodel.FromEnvironment(t.Config.ReviewModel, os.Getenv)
				if err != nil {
					repository.Close()
					st.Close()
					return fail(fmt.Errorf("tenant %s review model configuration: %w", t.Config.ID, err))
				}
				reviewer = nil
				if configured != nil {
					reviewer = configured
				}
			}
		}
		svc.TenantID = t.Config.ID
		svc.TenantName = t.Config.Name
		svc.SourceKind = t.Config.Source.Kind
		svc.CollectionMode = t.Config.Source.CollectionMode
		svc.Sources = sources
		var managed *datasources.Registry
		var err error
		if !demo && os.Getenv("NINES_SOURCE_HOSTS") != "" {
			var connectionRepo identity.Repository
			if memory {
				connectionRepo = identity.NewMemory()
			} else {
				connectionRepo, err = identity.NewPostgres(ctx, scopedDSN)
				if err != nil {
					return fail(err)
				}
			}
			reserved, hosts, orgs := map[string]bool{"primary": true}, map[string]bool{}, map[string]bool{}
			for _, host := range strings.Split(os.Getenv("NINES_SOURCE_HOSTS"), ",") {
				if host = strings.TrimSpace(host); host != "" {
					hosts[strings.ToLower(host)] = true
				}
			}
			for _, connection := range connections {
				reserved[connection.Name] = true
				if connection.MimirOrgID != "" {
					orgs[connection.MimirOrgID] = true
				}
			}
			routes := datasources.ProviderRoutes{}
			for kind, values := range t.Config.ManagedProviderRoutes {
				routes[kind] = append([]string{}, values...)
			}
			for _, connection := range connections {
				if connection.Kind == "newrelic" {
					routes["newrelic"] = append(routes["newrelic"], connection.Options["accountID"])
					if connection.Rules != nil && connection.Rules.EntityGUID != "" {
						routes["newrelic-entity"] = append(routes["newrelic-entity"], connection.Rules.EntityGUID)
					}
				}
				if connection.Kind == "victoriametrics" && connection.Options["accountID"] != "" {
					route := connection.Options["accountID"]
					if project := connection.Options["projectID"]; project != "" {
						route += ":" + project
					}
					routes["victoriametrics"] = append(routes["victoriametrics"], route)
				}
				if connection.Kind == "dash0" {
					routes["dash0"] = append(routes["dash0"], connection.Options["dataset"])
				}
			}
			managed, err = datasources.New(ctx, connectionRepo, t.Config.ID, reserved, hosts, orgs, os.Getenv("NINES_SOURCE_ENCRYPTION_KEY"), os.Getenv("NINES_MANAGED_RULE_DIRECTORY"), st, routes)
			if err != nil {
				connectionRepo.Close()
				return fail(err)
			}
			svc.DynamicSource = managed.Source
		}
		stored, err := st.List(ctx)
		if err != nil {
			repository.Close()
			st.Close()
			return fail(err)
		}
		for _, objective := range stored {
			if err := svc.ValidateSource(objective); err != nil {
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("stored SLO %s: %w", objective.ID, err))
			}
		}
		svc.Demo = demo
		wf := workflow.New(svc, repository)
		auth := "none"
		if t.BearerToken != "" {
			auth = "bearer"
		} else if t.BasicPassword != "" {
			auth = "basic"
		}

		var invRepo investigation.Repository
		if demo || memory {
			invRepo = investigation.NewMemory()
		} else {
			invRepo, err = investigation.NewPostgres(ctx, scopedDSN)
			if err != nil {
				repository.Close()
				st.Close()
				return fail(err)
			}
		}
		var replayRepo replay.Repository
		if demo || memory {
			replayRepo = replay.NewMemory()
		} else {
			replayRepo, err = replay.NewPostgres(ctx, scopedDSN)
			if err != nil {
				invRepo.Close()
				repository.Close()
				st.Close()
				return fail(err)
			}
		}
		var intentRepo intentflow.Repository
		if demo || memory {
			intentRepo = intentflow.NewMemory()
		} else {
			intentRepo, err = intentflow.NewPostgres(ctx, scopedDSN)
			if err != nil {
				replayRepo.Close()
				invRepo.Close()
				repository.Close()
				st.Close()
				return fail(err)
			}
		}

		var notification *workflowhook.Dispatcher
		if t.WorkflowWebhook != nil {
			journal, ok := intentRepo.(workflowhook.Repository)
			if !ok {
				if managed != nil {
					managed.Close()
				}
				intentRepo.Close()
				replayRepo.Close()
				invRepo.Close()
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("tenant %s workflow journal does not support webhook delivery", t.Config.ID))
			}
			notification, err = workflowhook.New(journal, t.Config.ID, *t.WorkflowWebhook)
			if err != nil {
				if managed != nil {
					managed.Close()
				}
				intentRepo.Close()
				replayRepo.Close()
				invRepo.Close()
				repository.Close()
				st.Close()
				return fail(fmt.Errorf("tenant %s: %w", t.Config.ID, err))
			}
		}
		runtime := tenantRuntime{WorkflowWebhook: notification, ManagedSources: managed, IntentConversations: intentRepo, Replays: replayRepo, Investigations: invRepo, Workspace: httpapi.Workspace{ManagedSources: managed, IntentConversations: intentflow.New(svc, intentRepo), Replays: replay.New(svc, replayRepo), Investigations: investigation.New(svc, invRepo), Connections: connections, Rules: &rules.Manager{Tenant: t.Config.ID, Targets: ruleTargets, External: &operator.Publisher{Tenant: t.Config.ID, Store: st}}, ID: t.Config.ID, Name: t.Config.Name, Service: svc, Source: t.Config.Source, SourceAuth: auth, AccessConfig: t.Config.Access, Grants: t.Grants, Anonymous: len(t.Grants) == 0 && !identityEnabled, Workflows: wf}, Repository: repository, Webhook: t.Webhook}
		runtime.Workspace.Replays.Privacy = t.Config.ReplayPrivacy
		if t.Config.SLOExtensionLock != "" {
			extensionPath := t.Config.SLOExtensionLock
			if !filepath.IsAbs(extensionPath) {
				extensionPath = filepath.Join(filepath.Dir(file), extensionPath)
			}
			registry, err := slotemplates.Load(extensionPath)
			if err != nil {
				runtime.Close()
				return fail(fmt.Errorf("tenant %s extension initialization: %w", t.Config.ID, err))
			}
			runtime.Workspace.Templates = registry
		}
		if t.Config.AuditArchive != nil {
			if memory && !demo {
				runtime.Close()
				return fail(fmt.Errorf("tenant %s auditArchive requires durable PostgreSQL storage outside demo mode", t.Config.ID))
			}
			sink, e := auditarchive.NewS3(ctx, *t.Config.AuditArchive)
			if e != nil {
				runtime.Close()
				return fail(fmt.Errorf("tenant %s audit archive initialization failed: %w", t.Config.ID, e))
			}
			runtime.Workspace.IntentConversations.Archive = &auditarchive.Worker{Tenant: t.Config.ID, Config: *t.Config.AuditArchive, Repository: intentRepo, Sink: sink}
		}
		svc.RuleManager = runtime.Workspace.Rules
		runtime.Workspace.Rules.PublicationGuard = svc.PinRuleDestination
		if managed != nil {
			runtime.Workspace.Rules.DynamicTarget = managed.Target
		}
		runtime.Workspace.Investigations.Model = reviewer
		runtime.Workspace.IntentConversations.Model = reviewer
		if !demo {
			assessor, err := systemone.New(t.Config.SystemOne, os.Getenv)
			if err != nil {
				runtime.Close()
				return fail(fmt.Errorf("tenant %s System One configuration: %w", t.Config.ID, err))
			}
			if assessor != nil {
				runtime.Workspace.IntentConversations.SystemOne = assessor
			}
		}
		out = append(out, runtime)
		if demo {
			if err := seed(ctx, svc); err != nil {
				return fail(err)
			}
			if err := seedReplay(ctx, runtime.Workspace.Replays); err != nil {
				return fail(err)
			}
		}
	}
	return out, nil
}

func runWorkflowRecovery(ctx context.Context, wf *workflow.Service, log *slog.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := wf.Recover(attempt)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Error("recover approved workflows", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func openProvider(cfg tenancy.Source, credentials map[string]string, bearer, password string) (service.Collector, error) {
	values := make(map[string]string, len(credentials)+3)
	for key, value := range credentials {
		values[key] = value
	}
	if bearer != "" {
		values["bearerToken"] = bearer
	}
	if cfg.BasicUsername != "" {
		values["basicUsername"] = cfg.BasicUsername
	}
	if password != "" {
		values["basicPassword"] = password
	}
	return providers.Default.Open(cfg.Kind, telemetryprovider.Configuration{Options: cfg.Options, Endpoint: cfg.URL, Dataset: cfg.MimirOrgID, Credentials: values})
}
