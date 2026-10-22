package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type draftModel struct {
	goal v1.ModelGoal
	err  error
	seen string
}

func (d *draftModel) ModelID() string { return "test-model" }
func (d *draftModel) Draft(_ context.Context, text string) (v1.ModelGoal, error) {
	d.seen = text
	return d.goal, d.err
}
func ptr(s string) *string { return &s }
func TestNaturalLanguageDraftUsesLiteralEvidenceAndScope(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	in := v1.DraftIntentInput{Text: "99.9% of attempts should finish within two seconds over 28 days", Metadata: v1.Metadata{Name: "draft-api", Project: "test"}, Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, NoDataPolicy: "unknown"}
	fake := &draftModel{goal: v1.ModelGoal{Outcome: "latency", TargetText: ptr("99.9%"), WindowText: ptr("28 days"), ThresholdText: ptr("two seconds"), Clarifications: []string{}}}
	s.IntentDrafter = fake
	out, err := s.DraftIntent(ctx, in)
	if err != nil || out.Status != "draft" || out.Intent == nil || out.Intent.Spec.Target != .999 || out.Intent.Spec.WindowSeconds != 2419200 || *out.Intent.Spec.ThresholdSeconds != 2 || out.Intent.Spec.Service != "api" || fake.seen != in.Text {
		t.Fatal(out, err)
	}
	if _, err = s.Store.Read(ctx, "draft-api"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("draft wrote resource", err)
	}
	fake.goal.TargetText = ptr("100%")
	out, err = s.DraftIntent(ctx, in)
	if err != nil || out.Intent != nil || out.Status != "needs_clarification" {
		t.Fatal("invented numeric target accepted", out, err)
	}
	fake.goal.TargetText = ptr("99.9%")
	fake.goal.Outcome = "compound"
	out, err = s.DraftIntent(ctx, in)
	if err != nil || out.Intent != nil {
		t.Fatal("joint goal accepted", out, err)
	}
	fake.goal.Outcome = "latency"
	fake.err = errors.New("secret-key-in-provider-failure")
	_, err = s.DraftIntent(ctx, in)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	s.IntentDrafter = nil
	if _, err = s.DraftIntent(ctx, in); !errors.Is(err, model.ErrUnavailable) {
		t.Fatal(err)
	}
}

type backtestSource struct {
	*testtelemetry.Source
	mode       string
	calls      int
	resolution int64
	after      func()
}

func (b *backtestSource) CollectWindowRange(_ context.Context, s model.SLO, from, to time.Time, res int64) ([]model.WindowObservation, error) {
	b.calls++
	b.resolution = res
	if b.mode == "error" {
		return nil, errors.New("provider-password")
	}
	out := []model.WindowObservation{}
	i := 0
	for at := from.Add(time.Duration(res) * time.Second); !at.After(to); at = at.Add(time.Duration(res) * time.Second) {
		i++
		for _, w := range s.Windows() {
			good, total, cov := 99.0, 100.0, 1.0
			switch i {
			case 2:
				cov = .5
			case 3:
				good = 0
				total = 0
			case 4:
				continue
			}
			if b.mode == "invalid" {
				good = 101
			}
			out = append(out, model.WindowObservation{Time: at, WindowSeconds: w, Revision: model.ConfigRevision(s), Good: good, Total: total, Coverage: cov})
		}
	}
	if b.after != nil {
		b.after()
	}
	return out, nil
}
func TestIntentBacktestBoundedReadOnlyAndNoOverlapSummation(t *testing.T) {
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c := &backtestSource{Source: source}
	s.Collector = c
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	in := goal()
	in.Binding = &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}
	in.Intent.Spec.Policy = &v1.BurnRatePolicy{ShortWindowSeconds: 60, LongWindowSeconds: 300, BurnRate: 5, MinCoverage: 1}
	request := v1.IntentBacktestInput{Plan: in, From: now.Add(-25 * time.Minute), To: now.Add(-5 * time.Minute), ResolutionSeconds: 300}
	result, err := s.BacktestIntent(ctx, request)
	if err != nil || len(result.Points) != 4 || result.CompleteCutoffs != 1 || result.NoTrafficCutoffs != 1 || result.UnknownCutoffs != 2 || result.TargetViolationCutoffs != 1 || result.BurnThresholdCutoffs != 1 || c.resolution != 300 {
		t.Fatal(result, err)
	}
	if result.Points[0].Reliability == nil || *result.Points[0].Reliability != .99 || result.Points[3].Reliability != nil || result.Points[2].Reliability != nil {
		t.Fatal(result.Points)
	}
	if _, err = s.Store.Read(ctx, "intent-api"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("backtest wrote SLO", err)
	}
	// Measurement-only reads retain the intent digest but do not propose or
	// require rebasing a historical CUJ link that has since advanced.
	request.Plan.Intent.Spec.Journey = &v1.IntentJourney{Name: "checkout", Step: "request", ExpectedRevision: 1, Required: true}
	journey, e := s.Journeys.JourneyRevision(ctx, "checkout", 0)
	if e != nil {
		t.Fatal(e)
	}
	journey.Manifest.Spec.DisplayName = "Updated checkout"
	if _, e = s.ApplyJourney(ctx, journey.Manifest, 1); e != nil {
		t.Fatal(e)
	}
	if out, e := s.BacktestIntent(ctx, request); e != nil || out.IntentDigest != model.DefinitionDigest(request.Plan.Intent) {
		t.Fatal(out, e)
	}
	previousCalls := c.calls
	request.ResolutionSeconds = 1
	if _, err = s.BacktestIntent(ctx, request); err == nil || c.calls != previousCalls {
		t.Fatal("unbounded request reached provider", err)
	}
	request.ResolutionSeconds = 300
	request.To = now.Add(time.Hour)
	if _, err = s.BacktestIntent(ctx, request); err == nil || c.calls != previousCalls {
		t.Fatal("future query reached provider", err)
	}
	request.To = now.Add(-5 * time.Minute)
	c.mode = "invalid"
	if _, err = s.BacktestIntent(ctx, request); err == nil {
		t.Fatal("invalid count accepted")
	}
	c.mode = "error"
	if _, err = s.BacktestIntent(ctx, request); err == nil || strings.Contains(err.Error(), "password") {
		t.Fatal(err)
	}
	c.mode = ""
	c.after = func() {
		b := testtelemetry.Binding()
		b.Spec.DisplayName = "Changed"
		if _, err := s.ApplyDefinition(ctx, model.Definition{Binding: &b}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.BacktestIntent(ctx, request); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale evidence accepted", err)
	}
}
func TestDriftMonitorPinsBindingBlocksEvaluationAndRecovers(t *testing.T) {
	ctx := context.Background()
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	driver := &backtestSource{Source: source}
	s.Collector = driver
	b := testtelemetry.Binding()
	b.Metadata.Name = "latency"
	b.Spec.Measurement = "latency"
	b.Spec.Metric = "http_duration_seconds"
	b.Spec.SuccessLabels = nil
	threshold := .3
	b.Spec.ThresholdSeconds = &threshold
	if _, err = s.ApplyDefinition(ctx, model.Definition{Binding: &b}, 0); err != nil {
		t.Fatal(err)
	}
	compiled, err := s.CompileBinding(ctx, "latency", v1.CompileBindingInput{ExpectedRevision: 1, Name: "latency-slo", DisplayName: "Latency", Target: .99, WindowSeconds: 3600, Policy: &v1.BurnRatePolicy{ShortWindowSeconds: 60, LongWindowSeconds: 300, BurnRate: 1, MinCoverage: 1}})
	if err != nil {
		t.Fatal(err)
	}
	d := model.SLODefinition(compiled.Manifest)
	if _, err = s.ApplyDefinition(ctx, model.Definition{SLO: &d}, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	s.Now = func() time.Time { return now }
	at := now.Add(-time.Minute)
	if err = s.Store.Update(ctx, "latency-slo", func(r *model.Record) error {
		r.Windows = map[string]model.WindowObservation{}
		for _, w := range r.SLO.Windows() {
			o := model.WindowObservation{Time: at, WindowSeconds: w, Revision: 1, Good: 100, Total: 100, Coverage: 1}
			r.Windows[model.WindowKey(w, at)] = o
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source.Truncated = true
	h, err := s.CheckMeasurement(ctx, "latency-slo")
	if err != nil || h.State != "unknown" || h.EvaluationBlocked {
		t.Fatal(h, err)
	}
	source.Truncated = false
	source.MissingBucket = true
	h, err = s.CheckMeasurement(ctx, "latency-slo")
	if err != nil || h.State != "drifted" || !h.EvaluationBlocked {
		t.Fatal(h, err)
	}
	status, err := s.Status(ctx, "latency-slo", time.Time{})
	if err != nil || status.Reliability != nil || status.DataStatus != "measurement_drift" {
		t.Fatal(status, err)
	}
	recordBefore, _ := s.Store.Read(ctx, "latency-slo")
	beforeCount := len(recordBefore.Windows)
	if _, err = s.Collect(ctx, "latency-slo", at.Add(-time.Minute), at); err == nil || driver.calls != 0 {
		t.Fatal("drifted measurement was queried", err, driver.calls)
	}
	recordAfter, _ := s.Store.Read(ctx, "latency-slo")
	if len(recordAfter.Windows) != beforeCount || recordAfter.Collection.Measurement == nil || !recordAfter.Collection.Measurement.EvaluationBlocked {
		t.Fatal("drift state or samples lost")
	}
	recordAfter.Collection.Measurement.State = "forged"
	checkAgain, _ := s.Store.Read(ctx, "latency-slo")
	if checkAgain.Collection.Measurement.State == "forged" {
		t.Fatal("memory read leaked health pointer")
	}
	// A fresh service instance must retain the persisted block rather than forgetting drift.
	fresh := service.New(s.Store, driver)
	fresh.Now = s.Now
	persisted, err := fresh.Status(ctx, "latency-slo", time.Time{})
	if err != nil || persisted.DataStatus != "measurement_drift" {
		t.Fatal(persisted, err)
	}
	source.Fail = true
	h, err = s.CheckMeasurement(ctx, "latency-slo")
	if err != nil || h.State != "unavailable" || !h.EvaluationBlocked {
		t.Fatal("unavailability cleared drift", h, err)
	}
	source.Fail = false
	source.MissingBucket = false
	h, err = s.CheckMeasurement(ctx, "latency-slo")
	if err != nil || h.State != "compatible" || h.EvaluationBlocked {
		t.Fatal(h, err)
	}
	status, err = s.Status(ctx, "latency-slo", time.Time{})
	if err != nil || status.Reliability == nil || *status.Reliability != 1 {
		t.Fatal(status, err)
	}
	rec, _ := s.Store.Read(ctx, "latency-slo")
	if rec.SLO.MeasurementBinding.Revision != 1 || model.DefinitionDigest(model.DefinitionFromSLO(rec.SLO)) != compiled.Digest {
		t.Fatal("monitor changed definition", rec.SLO)
	}
}

func (b *backtestSource) CollectWindows(ctx context.Context, s model.SLO, from, to time.Time) ([]model.WindowObservation, error) {
	return b.CollectWindowRange(ctx, s, from, to, s.StepSeconds)
}

func TestTargetEditCannotClearPersistedMeasurementBlock(t *testing.T) {
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := s.CompileBinding(ctx, "requests", v1.CompileBindingInput{ExpectedRevision: 1, Name: "blocked-api", DisplayName: "API", Target: .999, WindowSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	d := model.SLODefinition(c.Manifest)
	if _, err = s.ApplyDefinition(ctx, model.Definition{SLO: &d}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Update(ctx, "blocked-api", func(r *model.Record) error {
		r.Collection.Measurement = &v1.BindingHealth{State: "drifted", CheckedAt: s.Now(), Binding: *r.SLO.MeasurementBinding, SLORevision: 1, EvaluationBlocked: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d.Spec.Target = .99
	if _, err = s.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); err != nil {
		t.Fatal(err)
	}
	rec, err := s.Store.Read(ctx, "blocked-api")
	if err != nil || rec.Collection.Measurement == nil || !rec.Collection.Measurement.EvaluationBlocked || rec.Collection.Measurement.SLORevision != 2 {
		t.Fatal(rec.Collection, err)
	}
	source.Fail = true
	health, err := s.CheckMeasurement(ctx, "blocked-api")
	if err != nil || !health.EvaluationBlocked {
		t.Fatal(health, err)
	}
}

func TestBacktestRejectsSourceChangeAndCarriesRouteProvenance(t *testing.T) {
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	driver := &backtestSource{Source: source}
	s.Collector = driver
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	in := goal()
	in.Binding = &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}
	request := v1.IntentBacktestInput{Plan: in, From: now.Add(-25 * time.Minute), To: now.Add(-5 * time.Minute), ResolutionSeconds: 300}
	out, err := s.BacktestIntent(context.Background(), request)
	if err != nil || out.SourceDigest != s.SourceFingerprint("primary") || out.IntentDigest != model.DefinitionDigest(in.Intent) || out.From != request.From || out.To != request.To {
		t.Fatal("missing backtest provenance", out, err)
	}
	driver.after = func() { source.Route = "different-mimir-dataset" }
	if _, err := s.BacktestIntent(context.Background(), request); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("source changed during backtest but stale evidence was accepted", err)
	}
}

func TestSourceChangeInvalidatesMeasurementHealthUntilRechecked(t *testing.T) {
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	compiled, err := s.CompileBinding(ctx, "requests", v1.CompileBindingInput{ExpectedRevision: 1, Name: "routed-slo", DisplayName: "Routed SLO", Target: .999, WindowSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	definition := model.SLODefinition(compiled.Manifest)
	if _, err := s.ApplyDefinition(ctx, model.Definition{SLO: &definition}, 0); err != nil {
		t.Fatal(err)
	}
	original, err := s.CheckMeasurement(ctx, "routed-slo")
	if err != nil || original.State != "compatible" || original.SourceDigest == "" {
		t.Fatal(original, err)
	}
	source.Route = "another-mimir-organization"
	view, err := s.ReadSLO(ctx, "routed-slo")
	if err != nil || view.MeasurementHealth == nil || view.MeasurementHealth.Code != "measurement_source_changed" || !view.MeasurementHealth.EvaluationBlocked || view.Status.Reliability != nil {
		t.Fatal("cached evidence crossed a source change", view, err)
	}
	if _, err := s.Collect(ctx, "routed-slo", s.Now().UTC().Truncate(time.Minute).Add(-2*time.Minute), s.Now().UTC().Truncate(time.Minute).Add(-time.Minute)); err == nil {
		t.Fatal("changed source was queried before measurement revalidation")
	}
	fresh, err := s.CheckMeasurement(ctx, "routed-slo")
	if err != nil || fresh.State != "compatible" || fresh.EvaluationBlocked || fresh.SourceDigest == original.SourceDigest {
		t.Fatal("fresh source evidence did not replace old provenance", fresh, err)
	}
}

type sourceChangingDiscovery struct{ *testtelemetry.Source }

func (d *sourceChangingDiscovery) Discover(ctx context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	out, err := d.Source.Discover(ctx, metric, labels, at)
	d.Route = "changed-during-discovery"
	return out, err
}

func TestSourceChangeDuringDiscoveryRejectsEvidence(t *testing.T) {
	for _, operation := range []string{"compile", "binding-health"} {
		t.Run(operation, func(t *testing.T) {
			s, source, err := testtelemetry.New()
			if err != nil {
				t.Fatal(err)
			}
			s.Collector = &sourceChangingDiscovery{Source: source}
			ctx := context.Background()
			if operation == "compile" {
				in := goal()
				in.Binding = &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}
				_, err = s.PlanIntent(ctx, in)
			} else {
				_, err = s.CheckBinding(ctx, "requests", 1)
			}
			if !errors.Is(err, model.ErrRevisionConflict) {
				t.Fatal("accepted evidence collected across a source change", err)
			}
		})
	}
}
