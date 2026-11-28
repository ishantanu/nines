package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

// The reconciler reports what it serves, each pass's outcome, failures by
// stage and class, and when it last succeeded.
func TestReconcilerTelemetry(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	defer otel.SetMeterProvider(previous)

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := identity.NewMemory()
	auth := testIdentity(t, repo)
	// "taken" is already mounted but not configured, so mounting it fails.
	router, err := httpapi.NewRouter([]httpapi.Workspace{
		{ID: "default", Name: "Default", Service: service.New(store.NewMemory(), nil)},
		{ID: "taken", Name: "Taken", Service: service.New(store.NewMemory(), nil)},
	}, log, auth)
	if err != nil {
		t.Fatal(err)
	}
	scheduler, _ := service.NewCollectionScheduler(nil, 1)
	r := &tenantReconciler{repo: repo, opts: tenantOptions{memory: true, identityEnabled: true}, configured: map[string]bool{"default": true}, router: router, auth: auth, scheduler: scheduler, log: log}
	defer r.Close()
	if err = observability.ObserveTenants(r.tenantCounts, r.lastReconciled); err != nil {
		t.Fatal(err)
	}
	register(t, repo, identity.RegisteredTenant{ID: "payments", Name: "Payments", State: identity.TenantActive, Version: 1})
	register(t, repo, identity.RegisteredTenant{ID: "taken", Name: "Taken", State: identity.TenantActive, Version: 1})
	before := time.Now()
	if err = r.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	var rm metricdata.ResourceMetrics
	if err = reader.Collect(ctx, &rm); err != nil {
		t.Fatal(err)
	}
	found := map[string]metricdata.Aggregation{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			found[m.Name] = m.Data
		}
	}
	attr := func(set attribute.Set, key string) string {
		v, _ := set.Value(attribute.Key(key))
		return v.AsString()
	}
	tenants := found["nines.tenants"].(metricdata.Gauge[int64])
	counts := map[string]int64{}
	for _, p := range tenants.DataPoints {
		counts[attr(p.Attributes, "state")+"/"+attr(p.Attributes, "managed_by")] = p.Value
	}
	if counts["active/configuration"] != 1 || counts["active/console"] != 1 || counts["retired/console"] != 0 {
		t.Fatal("nines.tenants", counts)
	}
	reconciles := found["nines.tenant.reconcile"].(metricdata.Sum[int64])
	if len(reconciles.DataPoints) != 1 || attr(reconciles.DataPoints[0].Attributes, "outcome") != "succeeded" || reconciles.DataPoints[0].Value != 1 {
		t.Fatal("nines.tenant.reconcile", reconciles.DataPoints)
	}
	failures := found["nines.tenant.open.failures"].(metricdata.Sum[int64])
	if len(failures.DataPoints) != 1 || attr(failures.DataPoints[0].Attributes, "stage") != "mount" || attr(failures.DataPoints[0].Attributes, "error.class") != "other" {
		t.Fatal("nines.tenant.open.failures", failures.DataPoints)
	}
	last := found["nines.tenant.registry.last_success"].(metricdata.Gauge[float64])
	if len(last.DataPoints) != 1 || last.DataPoints[0].Value < float64(before.Unix()) {
		t.Fatal("nines.tenant.registry.last_success", last.DataPoints)
	}
	if _, ok := found["nines.tenant.reconcile.duration"]; !ok {
		t.Fatal("no reconcile duration")
	}
}
