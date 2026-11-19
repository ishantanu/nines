package intentflow

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func checkState(t *testing.T, v v1.IntentConversation, code string) string {
	t.Helper()
	for _, c := range workflowStatus(v).PreparationChecks {
		if c.Code == code {
			return c.State
		}
	}
	t.Fatal("missing diagnostic", code)
	return ""
}
func TestPreparationChecksMissingEvidenceDoesNotChangeGates(t *testing.T) {
	v := v1.IntentConversation{IntentConversationSummary: v1.IntentConversationSummary{State: "draft"}}
	if checkState(t, v, "goal") != "missing" || checkState(t, v, "measurement") != "missing" || checkState(t, v, "measurement_confirmation") != "missing" || checkState(t, v, "historical_evidence") != "not_recorded" || checkState(t, v, "current_provider_evidence") != "not_checked" {
		t.Fatal("invented evidence")
	}
	before := hash(v)
	status := workflowStatus(v)
	if status.NextAction != "run" || status.HumanRequired || hash(v) != before {
		t.Fatal("diagnostics changed workflow or input")
	}
	v.ChangeSet = &v1.IntentChangeSet{State: "approved", Digest: "reviewed"}
	status = workflowStatus(v)
	if len(status.PreparationChecks) != 0 || status.NextAction != "apply" || !status.IndependentApplierRequired {
		t.Fatal("diagnostics changed frozen review semantics")
	}
}
func TestPreparationChecksRecordedStaleAndTamperedEvidence(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := input()
	in.Goal.Spec.StepSeconds = 60
	in.Goal.Spec.Measurement = templateMeasurement()
	plan, err := s.Platform.PlanIntent(ctx, v1.PlanIntentInput{Intent: *in.Goal})
	if err != nil {
		t.Fatal(err)
	}
	v := v1.IntentConversation{Goal: in.Goal, Plan: &plan, Selection: &v1.IntentBindingConfirmation{IntentDigest: plan.IntentDigest, MeasurementDigest: plan.Compiled.MeasurementDigest}}
	for _, code := range []string{"goal", "measurement", "compilation", "measurement_confirmation"} {
		if checkState(t, v, code) != "recorded" {
			t.Fatal(code)
		}
	}
	v.Backtest = &v1.IntentBacktest{IntentDigest: plan.IntentDigest, ManifestDigest: plan.Compiled.Digest, SourceDigest: plan.SourceDigest}
	if checkState(t, v, "historical_evidence") != "recorded" {
		t.Fatal("lost saved history")
	}
	changed := clone(v)
	changed.Goal.Spec.Target = .99
	if checkState(t, changed, "compilation") != "stale" || checkState(t, changed, "measurement_confirmation") != "stale" || checkState(t, changed, "historical_evidence") != "stale" {
		t.Fatal("accepted changed goal")
	}
	changed = clone(v)
	changed.Selection.MeasurementDigest = "different"
	if checkState(t, changed, "measurement_confirmation") != "stale" {
		t.Fatal("accepted changed measurement")
	}
	changed = clone(v)
	changed.Plan.Compiled.Manifest.Spec.Target = .98
	if checkState(t, changed, "compilation") != "stale" {
		t.Fatal("accepted changed compiled manifest")
	}
	changed = clone(v)
	changed.Backtest.SourceDigest = "different"
	if checkState(t, changed, "historical_evidence") != "stale" {
		t.Fatal("accepted different historical source")
	}
}

func TestPreparationChecksRetainLegacyBindingConfirmation(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := input()
	plan, err := s.Platform.PlanIntent(ctx, v1.PlanIntentInput{Intent: *in.Goal, Binding: &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	v := v1.IntentConversation{Goal: in.Goal, Plan: &plan, Selection: &v1.IntentBindingConfirmation{IntentDigest: plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}}
	if checkState(t, v, "measurement") != "recorded" || checkState(t, v, "measurement_confirmation") != "recorded" {
		t.Fatal("legacy evidence was lost")
	}
	v.Selection.Binding.ExpectedRevision = 2
	if checkState(t, v, "measurement_confirmation") != "stale" {
		t.Fatal("mismatched legacy revision accepted")
	}
}
