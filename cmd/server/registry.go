package main

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

// registryInterval bounds how long a tenant created on another process
// takes to appear on this one.
const registryInterval = 10 * time.Second

// tenantReconciler serves the tenants platform administrators create while
// the server runs. Tenants from the configuration file are never touched.
type tenantReconciler struct {
	repo       identity.Repository
	opts       tenantOptions
	configured map[string]bool
	// prepare applies process-wide settings, such as feature gates, to a
	// tenant before it is served.
	prepare func(*httpapi.Workspace)
	// router, auth and workers are nil in a collector-only process.
	router    *httpapi.Router
	auth      *identity.Service
	scheduler *service.CollectionScheduler
	workers   func(tenantRuntime) func()
	log       *slog.Logger

	mu   sync.Mutex
	live map[string]*liveTenant
	// lastSuccess is when a reconcile last completed, for telemetry.
	lastSuccess atomic.Pointer[time.Time]
}

type liveTenant struct {
	runtime tenantRuntime
	stop    func()
	name    string
	retired bool
}

// Reconcile mounts new registry tenants and retires retired ones. A tenant
// that fails to open is skipped and retried on the next pass.
func (r *tenantReconciler) Reconcile(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		observability.TenantsReconciled(ctx, time.Since(start), err)
		if err == nil {
			now := time.Now()
			r.lastSuccess.Store(&now)
		}
	}()
	registered, err := identity.RegisteredTenants(ctx, r.repo)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live == nil {
		r.live = map[string]*liveTenant{}
	}
	present := map[string]bool{}
	for _, t := range registered {
		if r.configured[t.ID] {
			r.log.Error("registered tenant has the ID of a configured tenant; skipped", "tenant", t.ID)
			continue
		}
		present[t.ID] = true
		live, ok := r.live[t.ID]
		if t.State == identity.TenantDeleting {
			// Its data is being deleted: stop serving it.
			if ok {
				r.remove(t.ID, live)
			}
			continue
		}
		if !ok {
			var stage string
			var openErr error
			if live, stage, openErr = r.open(ctx, t); openErr != nil {
				// Only a class is logged: the error can carry configuration.
				observability.TenantOpenFailed(ctx, stage, openErr)
				r.log.Error("registered tenant could not be served; retrying. Check database connectivity and connection settings", "tenant", t.ID, "stage", stage, "error.class", observability.ErrorClass(openErr))
				continue
			}
			r.live[t.ID] = live
			r.log.Info("tenant mounted", "tenant", t.ID)
		}
		if live.name != t.Name {
			if r.router != nil {
				r.router.Rename(t.ID, t.Name)
			}
			live.name = t.Name
		}
		if t.State == identity.TenantRetired && !live.retired {
			r.retire(t.ID, live)
			r.log.Info("tenant retired", "tenant", t.ID)
		}
	}
	for id, live := range r.live {
		if !present[id] {
			r.remove(id, live)
		}
	}
	return nil
}

// open serves a registered tenant. On failure it reports the stage that
// failed: open (its stores), schedule or mount.
func (r *tenantReconciler) open(ctx context.Context, t identity.RegisteredTenant) (*liveTenant, string, error) {
	resolved := tenancy.Resolved{Config: tenancy.Tenant{ID: t.ID, Name: t.Name, Source: tenancy.Source{Kind: "manual"}}}
	runtime, err := buildTenant(ctx, resolved, r.opts)
	if err != nil {
		return nil, "open", err
	}
	if r.prepare != nil {
		r.prepare(&runtime.Workspace)
	}
	live := &liveTenant{runtime: runtime, stop: func() {}, name: t.Name, retired: t.State == identity.TenantRetired}
	// Retired before this process started: serve it read-only, run nothing.
	if !live.retired {
		if err = r.scheduler.Add(runtime.Workspace.Service); err != nil {
			runtime.Close()
			return nil, "schedule", err
		}
	}
	if r.router != nil {
		if r.auth != nil {
			r.auth.AddTenant(t.ID)
		}
		err = r.router.Mount(runtime.Workspace)
		if err == nil && live.retired {
			err = r.router.Retire(t.ID)
		}
		if err != nil {
			r.scheduler.Remove(runtime.Workspace.Service)
			runtime.Close()
			return nil, "mount", err
		}
	}
	if !live.retired && r.workers != nil {
		live.stop = r.workers(runtime)
	}
	return live, "", nil
}

// tenantCounts reports the tenants this process serves, for nines.tenants.
func (r *tenantReconciler) tenantCounts() []observability.TenantCount {
	r.mu.Lock()
	defer r.mu.Unlock()
	active, retired := int64(0), int64(0)
	for _, live := range r.live {
		if live.retired {
			retired++
		} else {
			active++
		}
	}
	return []observability.TenantCount{
		{State: "active", ManagedBy: "configuration", Count: int64(len(r.configured))},
		{State: "active", ManagedBy: "console", Count: active},
		{State: "retired", ManagedBy: "console", Count: retired},
	}
}

func (r *tenantReconciler) lastReconciled() time.Time {
	if at := r.lastSuccess.Load(); at != nil {
		return *at
	}
	return time.Time{}
}

func (r *tenantReconciler) retire(id string, live *liveTenant) {
	r.scheduler.Remove(live.runtime.Workspace.Service)
	live.stop()
	live.stop = func() {}
	if r.router != nil {
		_ = r.router.Retire(id)
	}
	live.retired = true
}

// remove stops serving a deleted tenant and closes its stores.
func (r *tenantReconciler) remove(id string, live *liveTenant) {
	r.scheduler.Remove(live.runtime.Workspace.Service)
	live.stop()
	if r.router != nil {
		r.router.Unmount(id)
	}
	live.runtime.Close()
	delete(r.live, id)
	r.log.Info("tenant removed", "tenant", id)
}

// Run reconciles until ctx ends.
func (r *tenantReconciler) Run(ctx context.Context) {
	tick := time.NewTicker(registryInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := r.Reconcile(ctx); err != nil && ctx.Err() == nil {
				r.log.Warn("tenant registry unavailable; serving the tenants already loaded", "error", err)
			}
		}
	}
}

// Close stops every registry tenant's work and closes its stores.
func (r *tenantReconciler) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, live := range r.live {
		live.stop()
		live.runtime.Close()
	}
	r.live = nil
}
