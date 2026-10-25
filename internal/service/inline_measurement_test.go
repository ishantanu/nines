package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func inlineSLO() v1.ServiceLevelObjective {
	return v1.NewSLO("inline-api", "test", v1.SLOSpec{DisplayName: "API availability", Service: "api", Source: "primary", Target: .999, WindowSeconds: 3600, StepSeconds: 60, Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "Completed API requests; excludes requests never reaching the server."}})
}

// This wrapper deliberately offers no topology/catalog interface.
type sloOnlyStore struct{ store.Store }

func TestInlineMeasurementNeedsNoCatalogAndVersionsWithSLO(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	source := &testtelemetry.Source{}
	s := service.New(sloOnlyStore{memory}, source)
	manifest := inlineSLO()
	compiled, e := s.CompileMeasurement(ctx, manifest)
	if e != nil {
		t.Fatal(e)
	}
	if compiled.MeasurementDigest == "" || compiled.Digest != model.DefinitionDigest(manifest) || strings.Contains(compiled.YAML, "providerWindow:") || strings.Contains(compiled.YAML, "measurementBinding:") {
		t.Fatal(compiled)
	}
	if rows, _ := memory.List(ctx); len(rows) != 0 {
		t.Fatal("compile wrote an SLO")
	}
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: manifest.Metadata, Spec: v1.IntentSpec{Description: "API requests should succeed", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, StepSeconds: 60, Measurement: manifest.Spec.Measurement, NoDataPolicy: "unknown"}}
	goal.Spec.Lifecycle = v1.LifecycleAspirational
	goal.Spec.Quality = &v1.MeasurementQualityPolicy{MinEvents: 100, CohortLabels: []string{"region"}}
	goal.Spec.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
	p, e := s.PlanIntent(ctx, v1.PlanIntentInput{Intent: goal})
	if e != nil || p.Status != "ready" || p.Compiled == nil {
		t.Fatal(p, e)
	}
	if p.Compiled.Manifest.Spec.Lifecycle != v1.LifecycleAspirational || p.Compiled.Manifest.Spec.Quality.MinEvents != 100 || p.Compiled.Manifest.Spec.ErrorBudgetPolicy.Owner != "oncall" || p.Compiled.Manifest.Spec.Intent.Lifecycle != v1.LifecycleAspirational {
		t.Fatal("inline intent lifecycle lost", p.Compiled)
	}
	raw, _ := json.Marshal(p.Compiled)
	if strings.Contains(string(raw), `"binding":`) || strings.Contains(string(raw), `"definition":`) {
		t.Fatal("inline result exposes empty binding", string(raw))
	}
	s.Store = memory // Compilation above deliberately had no catalog interface.
	d, e := model.ParseDefinition([]byte(compiled.YAML))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ApplyDefinition(ctx, d, 0); e != nil {
		t.Fatal(e)
	}
	record, _ := memory.Read(ctx, manifest.Metadata.Name)
	if record.SLO.ProviderWindow.GoodSelector != `http_requests_total{service="api",status="200"}` {
		t.Fatal(record.SLO)
	}
	// Mutation of a returned contract must not mutate the store.
	record.SLO.Measurement.Labels["service"] = "elsewhere"
	saved, _ := memory.Read(ctx, manifest.Metadata.Name)
	if saved.SLO.Measurement.Labels["service"] != "api" {
		t.Fatal("measurement alias escaped store")
	}
	health, e := s.CheckMeasurement(ctx, manifest.Metadata.Name)
	if e != nil || health.State != "compatible" || health.MeasurementDigest != compiled.MeasurementDigest {
		t.Fatal(health, e)
	}
	// Update the same identity and archive its original contract and evidence.
	manifest.Spec.Measurement.SuccessLabels = map[string]string{"status": "500"}
	next := model.Definition{SLO: (*model.SLODefinition)(&manifest)}
	if _, e = s.ApplyDefinition(ctx, next, 1); e != nil {
		t.Fatal(e)
	}
	active, _ := memory.Read(ctx, manifest.Metadata.Name)
	if active.SLO.Revision != 2 || active.Collection.Measurement != nil {
		t.Fatal(active)
	}
	archive, e := s.SLORevision(ctx, manifest.Metadata.Name, 1)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(archive)
	if !strings.Contains(string(b), `"200"`) {
		t.Fatal("original measurement not archived", string(b))
	}
}
func TestInlineMeasurementRejectsContradictionsAndUnverifiedEvidence(t *testing.T) {
	s := service.New(store.NewMemory(), &testtelemetry.Source{})
	ctx := context.Background()
	for _, mutate := range []func(*v1.ServiceLevelObjective){
		func(m *v1.ServiceLevelObjective) {
			m.Spec.ProviderWindow = &v1.ProviderWindow{GoodSelector: "up", TotalSelector: "up", ScrapeIntervalSeconds: 10}
		},
		func(m *v1.ServiceLevelObjective) { m.Spec.GoodQuery = "vector(1)"; m.Spec.TotalQuery = "vector(1)" },
		func(m *v1.ServiceLevelObjective) {
			m.Spec.MeasurementBinding = &v1.BindingReference{Name: "unrelated", Revision: 1, Digest: "bad"}
		},
		func(m *v1.ServiceLevelObjective) {
			m.Spec.Measurement.SuccessLabels = map[string]string{"service": "different"}
		},
		func(m *v1.ServiceLevelObjective) { m.Spec.Measurement.ScrapeIntervalSeconds = 0 },
		func(m *v1.ServiceLevelObjective) { m.Spec.Measurement.Type = "latency" },
	} {
		m := inlineSLO()
		mutate(&m)
		if _, e := s.CompileMeasurement(ctx, m); e == nil {
			t.Fatal("invalid measurement accepted", m)
		}
	}
	source := &testtelemetry.Source{Truncated: true}
	s = service.New(store.NewMemory(), source)
	if _, e := s.CompileMeasurement(ctx, inlineSLO()); e == nil {
		t.Fatal("truncated evidence accepted")
	}
	source.Truncated = false
	source.Fail = true
	if _, e := s.CompileMeasurement(ctx, inlineSLO()); e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatal("provider failure leaked or succeeded", e)
	}
	source.Fail = false
	m := inlineSLO()
	m.Spec.Measurement.Type = "latency"
	m.Spec.Measurement.Metric = "http_duration_seconds"
	m.Spec.Measurement.SuccessLabels = nil
	threshold := .3
	m.Spec.Measurement.ThresholdSeconds = &threshold
	if _, e := s.CompileMeasurement(ctx, m); e != nil {
		t.Fatal(e)
	}
	source.MissingBucket = true
	if _, e := s.CompileMeasurement(ctx, m); e == nil {
		t.Fatal("missing latency bucket accepted")
	}
}
func TestInlineIntentBacktestAndProvenance(t *testing.T) {
	source := &testtelemetry.Source{}
	s := service.New(sloOnlyStore{store.NewMemory()}, source)
	ctx := context.Background()
	m := inlineSLO()
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: m.Metadata, Spec: v1.IntentSpec{Description: "Review success", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: m.Spec.Target, WindowSeconds: m.Spec.WindowSeconds, StepSeconds: m.Spec.StepSeconds, Measurement: m.Spec.Measurement, NoDataPolicy: "unknown"}}
	in := v1.PlanIntentInput{Intent: goal}
	p, e := s.PlanIntent(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	forged := model.SLODefinition(p.Compiled.Manifest)
	forged.Spec.Measurement = &v1.MeasurementSpec{}
	if forged.Validate() == nil {
		t.Fatal("forged inline intent accepted")
	}
	in.Binding = &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}
	if _, e = s.PlanIntent(ctx, in); e == nil {
		t.Fatal("two measurement owners accepted")
	}
	in.Binding = nil
	to := time.Now().UTC().Truncate(5 * time.Minute).Add(-5 * time.Minute)
	back, e := s.BacktestIntent(ctx, v1.IntentBacktestInput{Plan: in, From: to.Add(-time.Hour), To: to, ResolutionSeconds: 300})
	if e != nil || len(back.Points) == 0 || back.ManifestDigest != p.Compiled.Digest {
		t.Fatal(back, e)
	}
	if _, e = s.Store.Read(ctx, m.Metadata.Name); !errors.Is(e, model.ErrNotFound) {
		t.Fatal("backtest wrote SLO", e)
	}
}

func TestInlineMeasurementDriftBlocksUntilContractIsVerified(t *testing.T) {
	source := &testtelemetry.Source{}
	s := service.New(store.NewMemory(), source)
	ctx := context.Background()
	m := inlineSLO()
	threshold := .3
	m.Spec.Measurement.Type = "latency"
	m.Spec.Measurement.Metric = "http_duration_seconds"
	m.Spec.Measurement.ThresholdSeconds = &threshold
	m.Spec.Measurement.SuccessLabels = nil
	if _, e := s.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&m)}, 0); e != nil {
		t.Fatal(e)
	}
	health, e := s.CheckMeasurement(ctx, m.Metadata.Name)
	if e != nil || health.State != "compatible" || health.EvaluationBlocked {
		t.Fatal(health, e)
	}
	source.MissingBucket = true
	health, e = s.CheckMeasurement(ctx, m.Metadata.Name)
	if e != nil || health.State != "drifted" || !health.EvaluationBlocked {
		t.Fatal(health, e)
	}
	source.Fail = true
	health, e = s.CheckMeasurement(ctx, m.Metadata.Name)
	if e != nil || !health.EvaluationBlocked {
		t.Fatal("unavailable check cleared drift blocker", health, e)
	}
	source.MissingBucket = false
	source.Fail = false
	health, e = s.CheckMeasurement(ctx, m.Metadata.Name)
	if e != nil || health.State != "compatible" || health.EvaluationBlocked {
		t.Fatal(health, e)
	}
}
