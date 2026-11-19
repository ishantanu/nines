package intentflow

import (
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

func TestIntentActivationEvidenceStages(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	old, future := now.Add(-10*time.Minute), now.Add(time.Minute)
	for _, tc := range []struct {
		name, state, code string
		alter             func(*v1.IntentActivation)
	}{
		{"active", "verified", "activation_verified", func(*v1.IntentActivation) {}},
		{"file_only", "pending", "evaluation_unverified", func(o *v1.IntentActivation) { o.Integration.Evaluation.State = "unknown" }},
		{"publication_missing", "pending", "publication_unverified", func(o *v1.IntentActivation) { o.Integration.Deployment.State = "not_deployed" }},
		{"native_stored_only", "pending", "evaluation_unverified", func(o *v1.IntentActivation) {
			o.Integration.RuleOutput.Code = "native_slo_no_recordings"
			o.Integration.Evaluation.State = "unknown"
		}},
		{"native_evaluated", "verified", "activation_verified", func(o *v1.IntentActivation) {
			o.Integration.RuleOutput.State = "unsupported"
			o.Integration.RuleOutput.Code = "native_slo_no_recordings"
		}},
		{"unsupported", "unsupported", "publication_unverified", func(o *v1.IntentActivation) { o.Integration.Deployment.State = "unsupported" }},
		{"stale_runtime", "pending", "evaluation_unverified", func(o *v1.IntentActivation) { o.Integration.Evaluation.ObservedAt = &old }},
		{"future_runtime", "pending", "evaluation_unverified", func(o *v1.IntentActivation) { o.Integration.Evaluation.ObservedAt = &future }},
		{"no_runtime_timestamp", "pending", "evaluation_unverified", func(o *v1.IntentActivation) { o.Integration.Evaluation.ObservedAt = nil }},
		{"missing_output", "pending", "output_unverified", func(o *v1.IntentActivation) { o.Integration.RuleOutput = nil }},
		{"wrong_dataset", "pending", "output_unverified", func(o *v1.IntentActivation) { o.Integration.RuleOutput.State = "not_observed" }},
		{"stale_collection", "pending", "collection_unverified", func(o *v1.IntentActivation) { o.Integration.Collection.State = "stale" }},
		{"zero_traffic", "pending", "measurement_unusable", func(o *v1.IntentActivation) { o.Measurement.Total = 0 }},
		{"limited_quality", "pending", "measurement_unusable", func(o *v1.IntentActivation) { o.Measurement.State = "limited" }},
		{"stale_measurement", "pending", "measurement_unusable", func(o *v1.IntentActivation) { o.Measurement.EvaluatedAt = old }},
		{"source_mismatch", "pending", "measurement_unusable", func(o *v1.IntentActivation) { o.Measurement.SourceDigest = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := v1.IntentActivation{CheckedAt: now, SourceDigest: "source", Integration: &v1.IntegrationStatus{Deployment: v1.IntegrationStage{State: "stored"}, Evaluation: v1.IntegrationStage{State: "evaluating", ObservedAt: &now}, RuleOutput: &v1.IntegrationStage{State: "verified", ObservedAt: &now}, Collection: v1.IntegrationStage{State: "collected", ObservedAt: &now}}, Measurement: &v1.MeasurementQuality{State: "usable", Total: 100, Good: 50, EvaluatedAt: now, SourceDigest: "source"}}
			tc.alter(&out)
			classifyActivation(&out, 60)
			if out.State != tc.state || out.Code != tc.code || (out.State == "pending") != (out.RetryAt != nil) {
				t.Fatalf("%+v", out)
			}
		})
	}
}
func TestIntentActivationReadOnlyAndFenced(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := appliedUpdate(t, pg)
			tick := v.ChangeSet.AppliedAt.Add(4 * time.Minute)
			s.Platform.Now = func() time.Time { return tick }
			s.Platform.Collector = &outcomeTelemetry{collectingTelemetry: s.Platform.Collector.(*collectingTelemetry), now: &tick, good: 999}
			dest := &outcomeDestination{t: t, now: &tick}
			s.Platform.RuleManager = &rules.Manager{Tenant: "alpha", External: dest, Targets: map[string]rules.Target{"primary": {Kind: "operator", Connection: "primary"}}}
			collectFreshBaseline(t, s, ctx, "intent-created")
			before, err := s.Repository.Read(ctx, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.InspectActivation(ctx, v.ID)
			if err != nil || result.State != "verified" || result.OperationDigest != v.ChangeSet.Digest || result.Resources[0].Revision != 2 {
				t.Fatalf("%+v %v", result, err)
			}
			after, err := s.Repository.Read(ctx, v.ID)
			if err != nil || hash(before) != hash(after) {
				t.Fatal("activation changed workflow", err)
			}
			dest.failure = true
			result, err = s.InspectActivation(ctx, v.ID)
			if err != nil || result.Code != "evaluation_unverified" || result.RetryAt == nil {
				t.Fatal(result, err)
			}
			dest.failure = false
			dest.hook = func() {
				dest.hook = nil
				_, err := s.Repository.Update(ctx, v.ID, func(r *record, now time.Time) error { touch(r, now); return nil })
				if err != nil {
					t.Error(err)
				}
			}
			if _, err = s.InspectActivation(ctx, v.ID); !errors.Is(err, ErrConflict) {
				t.Fatal("concurrent workflow change accepted", err)
			}
			telemetry := s.Platform.Collector.(*outcomeTelemetry)
			oldRoute := telemetry.Route
			dest.hook = func() { dest.hook = nil; telemetry.Route = "changed-during-inspection" }
			if _, err = s.InspectActivation(ctx, v.ID); !errors.Is(err, ErrConflict) {
				t.Fatal("concurrent source change accepted", err)
			}
			result, err = s.InspectActivation(ctx, v.ID)
			if err != nil || result.Code != "source_changed" || result.Integration != nil {
				t.Fatal("source drift exported runtime evidence", result, err)
			}
			telemetry.Route = oldRoute
			dest.hook = func() {
				dest.hook = nil
				if err := s.Platform.Store.Update(ctx, "intent-created", func(r *model.Record) error { r.SLO.Revision++; return nil }); err != nil {
					t.Error(err)
				}
			}
			if _, err = s.InspectActivation(ctx, v.ID); !errors.Is(err, ErrConflict) {
				t.Fatal("concurrent definition change accepted", err)
			}
			result, err = s.InspectActivation(ctx, v.ID)
			if err != nil || result.Code != "definition_changed" || result.Integration != nil {
				t.Fatal("definition drift exported runtime evidence", result, err)
			}
			s.Platform.TenantID = "other"
			if _, err = s.InspectActivation(ctx, v.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("cross tenant activation accepted", err)
			}
		})
	}
}
func TestIntentActivationRequiresAppliedChanges(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	if _, err := s.InspectActivation(ctx, v.ID); err == nil {
		t.Fatal("unapplied proposal inspected")
	}
}
