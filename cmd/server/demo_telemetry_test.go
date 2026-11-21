package main

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

// The typed demo SLOs are measured through Nines' ordinary collection path
// from the in-process synthetic source, and keep updating each minute.
func TestDemoTypedSLOsAreCollectedAndEvaluated(t *testing.T) {
	ctx := context.Background()
	svc := service.New(store.NewMemory(), nil)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	if err := seed(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if svc.Collector != nil {
		t.Fatal("the demo must not configure a default telemetry connection")
	}
	want := map[string]float64{"search-latency": .99, "search-index-freshness": .99, "mobile-sessions": .995, "order-pipeline-throughput": .99}
	check := func() {
		for id, target := range want {
			snap, err := svc.Status(ctx, id, time.Time{})
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			if snap.Reliability == nil || *snap.Reliability <= 0 || *snap.Reliability > 1 {
				t.Fatalf("%s: no measured reliability: %+v", id, snap)
			}
			if snap.Coverage != 1 {
				t.Fatalf("%s: coverage %v, want a full synthetic window", id, snap.Coverage)
			}
			// The freshness incident and throughput stalls spend budget but
			// every typed demo SLO still meets its target.
			if *snap.Reliability < target {
				t.Fatalf("%s: reliability %v below target %v", id, *snap.Reliability, target)
			}
		}
	}
	check()
	// The measurement contract check verifies the synthetic metrics, so the
	// portal shows the typed SLOs as compatible rather than unavailable.
	for _, s := range demoTypedSLOs() {
		if s.Measurement == nil {
			continue
		}
		health, err := svc.CheckMeasurement(ctx, s.ID)
		if err != nil || health.State != "compatible" {
			t.Fatalf("%s: measurement contract %s (%s): %v", s.ID, health.State, health.Message, err)
		}
	}
	// The worker's next tick collects the new minute for every typed SLO.
	before, err := svc.Store.Read(ctx, "search-latency")
	if err != nil {
		t.Fatal(err)
	}
	last := now
	now = now.Add(time.Minute)
	if err := collectDemo(ctx, svc, last, now); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Store.Read(ctx, "search-latency")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Collection.To.After(before.Collection.To) {
		t.Fatalf("collection did not advance: %v then %v", before.Collection.To, after.Collection.To)
	}
	check()

	// Synthetic counts are a function of time only.
	s := demoTypedSLOs()[3]
	a, _ := demoTelemetry{}.Collect(ctx, s, now.Add(-time.Hour), now)
	b, _ := demoTelemetry{}.Collect(ctx, s, now.Add(-time.Hour), now)
	if len(a) != 60 || a[17] != b[17] {
		t.Fatalf("synthetic collection is not deterministic: %d points", len(a))
	}
}

func TestDemoJourneyRunObjectiveIsEvaluated(t *testing.T) {
	ctx := context.Background()
	svc := service.New(store.NewMemory(), nil)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	if err := seed(ctx, svc); err != nil {
		t.Fatal(err)
	}
	views, err := svc.ListJourneys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range views {
		if v.Resource.Manifest.Spec.Agentic == nil {
			continue
		}
		found = true
		o := v.RunObjective
		if o == nil || o.State != "available" || o.Reliability == nil {
			t.Fatalf("run objective not evaluated: %+v", o)
		}
		t.Logf("journey %s: health %s, run reliability %.3f over %d judged runs", v.Resource.Manifest.Metadata.Name, v.Health, *o.Reliability, o.Judged)
	}
	if !found {
		t.Fatal("no agentic demo journey")
	}
}
