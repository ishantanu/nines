package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type evidenceSource struct {
	*testtelemetry.Source
	mode             string
	discovery, reads int
	after            func()
	selectors        []string
}

func (e *evidenceSource) Discover(ctx context.Context, m string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	e.discovery++
	out, err := e.Source.Discover(ctx, m, labels, at)
	if m == "http_duration_seconds" {
		for _, s := range out.Series {
			if s["le"] == "0.3" {
				s["le"] = "0.03"
			}
		}
	}
	return out, err
}
func (e *evidenceSource) CollectWindowRange(ctx context.Context, s model.SLO, from, to time.Time, res int64) ([]model.WindowObservation, error) {
	e.reads++
	if e.after != nil {
		e.after()
	}
	if e.mode == "error" {
		return nil, errors.New("fixture-private-provider-key")
	}
	if e.mode == "cancel" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	e.selectors = append(e.selectors, s.ProviderWindow.GoodSelector)
	out := []model.WindowObservation{}
	for at := from.Add(time.Duration(res) * time.Second); !at.After(to); at = at.Add(time.Duration(res) * time.Second) {
		if e.mode == "missing" || (e.mode == "stale" && at.After(to.Add(-3*time.Duration(s.StepSeconds)*time.Second))) {
			continue
		}
		for _, w := range s.Windows() {
			o := model.WindowObservation{Time: at, Revision: model.ConfigRevision(s), WindowSeconds: w, Good: 99, Total: 100, Coverage: 1}
			if e.mode == "partial" {
				o.Coverage = .5
			}
			if e.mode == "zero" {
				o.Good = 0
				o.Total = 0
			}
			if e.mode == "invalid" {
				o.Good = math.NaN()
			}
			out = append(out, o)
		}
	}
	return out, nil
}
func evidenceFixture(t *testing.T) (*service.Service, *evidenceSource, v1.IntentEvidenceInput) {
	t.Helper()
	source := &evidenceSource{Source: &testtelemetry.Source{Route: "synthetic-route"}}
	s := service.New(store.NewMemory(), source)
	s.Now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "preview", Project: "test"}, Spec: v1.IntentSpec{Description: "Completed API attempts", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, StepSeconds: 60, NoDataPolicy: "unknown", Measurement: inlineSLO().Spec.Measurement, Policies: []v1.BurnRatePolicy{{Name: "fast", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 1800, BurnRate: 6, MinCoverage: 1}}}}
	in := evidenceRequest(t, s, goal)
	source.discovery = 0
	source.reads = 0
	return s, source, in
}
func evidenceRequest(t *testing.T, s *service.Service, goal v1.SLOIntent) v1.IntentEvidenceInput {
	t.Helper()
	plan, err := s.PlanIntent(context.Background(), v1.PlanIntentInput{Intent: goal})
	if err != nil || plan.Compiled == nil {
		t.Fatal(plan, err)
	}
	var registry *slotemplates.Registry
	catalog, err := registry.List("")
	if err != nil {
		t.Fatal(err)
	}
	return v1.IntentEvidenceInput{Plan: v1.PlanIntentInput{Intent: goal}, IntentDigest: plan.IntentDigest, MeasurementDigest: plan.Compiled.MeasurementDigest, SourceDigest: model.DefinitionDigest(struct{ Tenant, Source, Route string }{s.TenantID, goal.Spec.Source, s.SourceFingerprint(goal.Spec.Source)}), CatalogDigest: catalog.Digest, ExpectedRevision: plan.ExpectedRevision}
}
func TestIntentEvidenceCurrentHistoryTradeoffsAndNoWrites(t *testing.T) {
	s, source, in := evidenceFixture(t)
	target := .99
	in.Alternatives = []v1.IntentEvidenceAlternative{{Name: "lower-target", Target: &target}, {Name: "longer-window", Window: "2h"}}
	to := s.Now().Add(-time.Minute)
	in.History = &v1.IntentEvidenceRange{From: to.Add(-10 * time.Minute), To: to, ResolutionSeconds: 60}
	before := model.DefinitionDigest(in)
	out, err := s.PrepareIntentEvidence(context.Background(), in, nil)
	if err != nil || out.State != "observed" || len(out.Comparisons) != 3 || out.AdapterCalls != 7 || out.MaxAdapterCalls != 7 || source.discovery != 1 || source.reads != 6 {
		t.Fatal(out, err, source.discovery, source.reads)
	}
	row := out.Comparisons[0]
	if row.Current.Total != 100 || row.Current.Good != 99 || row.Current.Reliability == nil || *row.Current.Reliability != .99 || row.Current.Freshness.State != "fresh" || row.Current.State != "usable" || row.History.CompleteCutoffs != 10 || row.History.TargetViolationCutoffs != 10 {
		t.Fatal("wrong independent window arithmetic", row)
	}
	if out.Comparisons[1].History.TargetViolationCutoffs != 0 || math.Abs(row.AllowedBadEventsPerMillion-1000) > 1e-8 || math.Abs(out.Comparisons[1].AllowedBadEventsPerMillion-10000) > 1e-8 || row.Policies[0].LongWindowBudgetFraction != 3 {
		t.Fatal("wrong explicit trade-off", out.Comparisons)
	}
	if !out.HumanConfirmationRequired || out.Digest == "" || model.DefinitionDigest(in) != before {
		t.Fatal("proposal or input changed")
	}
	resources, err := s.Store.List(context.Background())
	if err != nil || len(resources) != 0 {
		t.Fatal("evidence wrote an SLO", err)
	}
	in.History = nil
	in.Alternatives = nil
	source.reads = 0
	source.discovery = 0
	out, err = s.PrepareIntentEvidence(context.Background(), in, nil)
	if err != nil || source.reads != 1 || out.Comparisons[0].HistoryState != "not_requested" || out.Comparisons[0].History != nil {
		t.Fatal("implicit historical read", out, err)
	}
}
func TestIntentEvidenceMissingPartialZeroStaleAndProviderErrors(t *testing.T) {
	for _, mode := range []string{"missing", "partial", "zero", "stale", "error", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			s, source, in := evidenceFixture(t)
			source.mode = mode
			out, err := s.PrepareIntentEvidence(context.Background(), in, nil)
			if err != nil || out.State != "partial" {
				t.Fatal(out, err)
			}
			row := out.Comparisons[0]
			if mode == "error" || mode == "invalid" {
				if row.State != "provider_error" || row.Current != nil {
					t.Fatal(row)
				}
			} else {
				if row.State != "insufficient_evidence" {
					t.Fatal(row)
				}
				if mode == "stale" && row.Current.Freshness.State != "stale" {
					t.Fatal(row.Current)
				}
				if (mode == "zero" || mode == "missing") && row.Current.Reliability != nil {
					t.Fatal("invented success", row.Current)
				}
				if mode == "partial" && row.Current.Coverage != .5 {
					t.Fatal(row.Current)
				}
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "fixture-private-provider-key") {
				t.Fatal("provider error escaped")
			}
		})
	}
}
func TestIntentEvidenceRejectsChangedProvenanceAndUnboundedHistory(t *testing.T) {
	for _, change := range []func(*v1.IntentEvidenceInput){
		func(in *v1.IntentEvidenceInput) { in.IntentDigest = "changed" },
		func(in *v1.IntentEvidenceInput) { in.MeasurementDigest = "changed" },
		func(in *v1.IntentEvidenceInput) { in.SourceDigest = "changed" },
		func(in *v1.IntentEvidenceInput) { in.CatalogDigest = "changed" },
		func(in *v1.IntentEvidenceInput) { in.ExpectedRevision = 2 },
		func(in *v1.IntentEvidenceInput) { in.Plan.Intent.Spec.Measurement.SuccessLabels["status"] = "500" },
		func(in *v1.IntentEvidenceInput) {
			in.Alternatives = []v1.IntentEvidenceAlternative{{Name: "too-short", Window: "30ms"}}
		},
		func(in *v1.IntentEvidenceInput) {
			in.History = &v1.IntentEvidenceRange{From: time.Unix(1800000000, 0).Add(-100 * time.Hour), To: time.Unix(1800000000, 0), ResolutionSeconds: 1}
		},
	} {
		s, source, in := evidenceFixture(t)
		change(&in)
		if _, err := s.PrepareIntentEvidence(context.Background(), in, nil); err == nil || source.discovery != 0 || source.reads != 0 {
			t.Fatal("invalid evidence initiated reads", err)
		}
	}
	s, source, in := evidenceFixture(t)
	source.after = func() { source.Route = "changed" }
	if _, err := s.PrepareIntentEvidence(context.Background(), in, nil); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("changed route accepted", err)
	}
	s, source, in = evidenceFixture(t)
	source.after = func() {
		if err := s.Create(context.Background(), model.SLO{ID: "preview", Name: "Concurrent", Project: "test", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.PrepareIntentEvidence(context.Background(), in, nil); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("changed revision accepted", err)
	}
}
func TestIntentEvidencePreservesMillisecondLatencyAndUnsupportedCapabilities(t *testing.T) {
	s, source, in := evidenceFixture(t)
	threshold := .03
	goal := in.Plan.Intent
	goal.Spec.Outcome = "latency"
	goal.Spec.ThresholdSeconds = &threshold
	goal.Spec.Measurement = &v1.MeasurementSpec{Type: "latency", Metric: "http_duration_seconds", Labels: map[string]string{"service": "api"}, ThresholdSeconds: &threshold, ScrapeIntervalSeconds: 10, ObservationPoint: "Completed request duration"}
	in = evidenceRequest(t, s, goal)
	source.selectors = nil
	out, err := s.PrepareIntentEvidence(context.Background(), in, nil)
	if err != nil || out.Comparisons[0].Current == nil || len(source.selectors) != 1 || !strings.Contains(source.selectors[0], `le="0.03"`) || strings.Contains(source.selectors[0], `le="30"`) {
		t.Fatal(out, source.selectors, err)
	}
	s.Collector = &evidenceNoDiscovery{Collector: source, route: source.SourceFingerprint()}
	out, err = s.PrepareIntentEvidence(context.Background(), in, nil)
	if err != nil || out.State != "unsupported" || out.AdapterCalls != 0 {
		t.Fatal(out, err)
	}
}

type evidenceNoDiscovery struct {
	service.Collector
	route string
}

func (e *evidenceNoDiscovery) SourceFingerprint() string { return e.route }

func TestIntentEvidenceConfiguredQualityAndCancelledReads(t *testing.T) {
	for _, cohorts := range []bool{false, true} {
		s, _, in := evidenceFixture(t)
		goal := in.Plan.Intent
		goal.Spec.Quality = &v1.MeasurementQualityPolicy{MinEvents: 200}
		if cohorts {
			goal.Spec.Quality.CohortLabels = []string{"region"}
		}
		in = evidenceRequest(t, s, goal)
		out, err := s.PrepareIntentEvidence(context.Background(), in, nil)
		if err != nil || out.State != "partial" || out.Comparisons[0].Current.State != "limited" || out.Comparisons[0].Current.MinimumEvents != 200 {
			t.Fatal(out, err)
		}
		if cohorts && !strings.Contains(strings.Join(out.Comparisons[0].Gaps, ","), "cohorts_not_inspected") {
			t.Fatal("cohort gap hidden")
		}
	}
	s, source, in := evidenceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.PrepareIntentEvidence(ctx, in, nil)
	if !errors.Is(err, context.Canceled) || source.reads != 0 {
		t.Fatal("cancelled request queried provider", err)
	}
}

func TestIntentEvidenceFirstReviewCapturesProvenanceWithoutApproval(t *testing.T) {
	s, _, in := evidenceFixture(t)
	in.IntentDigest = ""
	in.MeasurementDigest = ""
	in.SourceDigest = ""
	in.CatalogDigest = ""
	out, err := s.PrepareIntentEvidence(context.Background(), in, nil)
	if err != nil || len(out.SourceDigest) != 64 || len(out.CatalogDigest) != 64 || len(out.IntentDigest) != 64 || len(out.MeasurementDigest) != 64 || !out.HumanConfirmationRequired || in.SourceDigest != "" {
		t.Fatal(out, err)
	}
}
