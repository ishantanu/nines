package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
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
}

type liveTenant struct {
	runtime tenantRuntime
	stop    func()
	retired bool
}

// Reconcile mounts new registry tenants and retires retired ones. A tenant
// that fails to open is skipped and retried on the next pass.
func (r *tenantReconciler) Reconcile(ctx context.Context) error {
	registered, err := identity.RegisteredTenants(ctx, r.repo)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live == nil {
		r.live = map[string]*liveTenant{}
	}
	for _, t := range registered {
		if r.configured[t.ID] {
			r.log.Error("registered tenant has the ID of a configured tenant; skipped", "tenant", t.ID)
			continue
		}
		live, ok := r.live[t.ID]
		if !ok {
			if live, err = r.open(ctx, t); err != nil {
				r.log.Error("registered tenant could not be opened; retrying", "tenant", t.ID, "error", err)
				continue
			}
			r.live[t.ID] = live
			r.log.Info("tenant mounted", "tenant", t.ID)
		}
		if t.State == identity.TenantRetired && !live.retired {
			r.retire(t.ID, live)
			r.log.Info("tenant retired", "tenant", t.ID)
		}
	}
	return nil
}

func (r *tenantReconciler) open(ctx context.Context, t identity.RegisteredTenant) (*liveTenant, error) {
	resolved := tenancy.Resolved{Config: tenancy.Tenant{ID: t.ID, Name: t.Name, Source: tenancy.Source{Kind: "manual"}}}
	runtime, err := buildTenant(ctx, resolved, r.opts)
	if err != nil {
		return nil, err
	}
	if r.prepare != nil {
		r.prepare(&runtime.Workspace)
	}
	live := &liveTenant{runtime: runtime, stop: func() {}, retired: t.State == identity.TenantRetired}
	// Retired before this process started: serve it read-only, run nothing.
	if !live.retired {
		if err = r.scheduler.Add(runtime.Workspace.Service); err != nil {
			runtime.Close()
			return nil, err
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
			return nil, err
		}
	}
	if !live.retired && r.workers != nil {
		live.stop = r.workers(runtime)
	}
	return live, nil
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
