package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

func testIdentity(t *testing.T, repo identity.Repository) *identity.Service {
	t.Helper()
	config, err := identity.ParseConfig([]byte("publicURL: http://127.0.0.1:8080\nproviders:\n- id: local\n  name: Local\n  type: local\n"), func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(context.Background(), config, repo, map[string]bool{"default": true})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func register(t *testing.T, repo identity.Repository, tenant identity.RegisteredTenant) {
	t.Helper()
	b, _ := json.Marshal(tenant)
	if err := repo.Update(context.Background(), "tenant:"+tenant.ID, func(tx identity.Tx) error {
		return tx.Put("tenant", "global", tenant.ID, b, time.Time{})
	}); err != nil {
		t.Fatal(err)
	}
}

// Tenants registered while the server runs are mounted, scheduled and given
// workers; retiring one stops its work and leaves it readable. Configured
// tenants are never touched.
func TestReconcilerMountsAndRetiresRegisteredTenants(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := identity.NewMemory()
	auth := testIdentity(t, repo)
	configuredSvc := service.New(store.NewMemory(), nil)
	router, err := httpapi.NewRouter([]httpapi.Workspace{{ID: "default", Name: "Default", Service: configuredSvc, Workflows: workflow.New(configuredSvc, workflow.NewMemory()), Grants: []tenancy.Grant{{Token: "owner-token", Role: "owner"}}}}, log, auth)
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := service.NewCollectionScheduler([]*service.Service{configuredSvc}, 1)
	if err != nil {
		t.Fatal(err)
	}
	var running atomic.Int64
	r := &tenantReconciler{
		repo: repo, opts: tenantOptions{memory: true, identityEnabled: true}, configured: map[string]bool{"default": true},
		prepare: func(w *httpapi.Workspace) { w.Features = features.All() },
		router:  router, auth: auth, scheduler: scheduler, log: log,
		workers: func(tenantRuntime) func() {
			running.Add(1)
			return func() { running.Add(-1) }
		},
	}
	defer r.Close()

	now := time.Now().UTC()
	register(t, repo, identity.RegisteredTenant{ID: "payments", Name: "Payments", State: identity.TenantActive, CreatedAt: now, Version: 1})
	register(t, repo, identity.RegisteredTenant{ID: "default", Name: "Impostor", State: identity.TenantActive, CreatedAt: now, Version: 1})
	if err = r.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if mounted, retired := router.Mounted("payments"); !mounted || retired {
		t.Fatalf("payments mounted=%v retired=%v", mounted, retired)
	}
	if len(scheduler.Services) != 2 || running.Load() != 1 {
		t.Fatalf("scheduled %d tenants, %d with workers", len(scheduler.Services), running.Load())
	}
	if got := r.live["default"]; got != nil {
		t.Fatal("a registry entry replaced a configured tenant")
	}
	// Reconciling again changes nothing.
	if err = r.Reconcile(ctx); err != nil || len(scheduler.Services) != 2 || running.Load() != 1 {
		t.Fatal("a second pass changed the tenants", err)
	}

	retiredAt := now.Add(time.Minute)
	register(t, repo, identity.RegisteredTenant{ID: "payments", Name: "Payments", State: identity.TenantRetired, CreatedAt: now, RetiredAt: &retiredAt, Version: 2})
	if err = r.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if mounted, retired := router.Mounted("payments"); !mounted || !retired {
		t.Fatalf("after retiring: mounted=%v retired=%v", mounted, retired)
	}
	if len(scheduler.Services) != 1 || running.Load() != 0 {
		t.Fatalf("a retired tenant is still collected (%d) or working (%d)", len(scheduler.Services), running.Load())
	}

	// A process that starts after the tenant was retired serves it read-only
	// and runs nothing for it.
	fresh, err := httpapi.NewRouter([]httpapi.Workspace{{ID: "default", Name: "Default", Service: service.New(store.NewMemory(), nil)}}, log, testIdentity(t, repo))
	if err != nil {
		t.Fatal(err)
	}
	freshScheduler, _ := service.NewCollectionScheduler(nil, 1)
	running.Store(0)
	later := &tenantReconciler{repo: repo, opts: r.opts, configured: r.configured, prepare: r.prepare, router: fresh, scheduler: freshScheduler, log: log, workers: r.workers}
	defer later.Close()
	if err = later.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if mounted, retired := fresh.Mounted("payments"); !mounted || !retired || len(freshScheduler.Services) != 0 || running.Load() != 0 {
		t.Fatalf("restart: mounted=%v retired=%v scheduled=%d working=%d", mounted, retired, len(freshScheduler.Services), running.Load())
	}
}

// A collector-only process schedules registered tenants without serving them.
func TestReconcilerInACollectorOnlySchedules(t *testing.T) {
	repo := identity.NewMemory()
	register(t, repo, identity.RegisteredTenant{ID: "payments", Name: "Payments", State: identity.TenantActive, Version: 1})
	scheduler, _ := service.NewCollectionScheduler(nil, 1)
	r := &tenantReconciler{repo: repo, opts: tenantOptions{memory: true, collectorOnly: true}, scheduler: scheduler, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	defer r.Close()
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(scheduler.Services) != 1 || scheduler.Services[0].TenantID != "payments" {
		t.Fatal("the collector did not schedule the registered tenant")
	}
}
