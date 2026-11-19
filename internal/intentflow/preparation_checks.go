package intentflow

import (
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
)

// preparationChecks uses only this saved snapshot. It does not make historical
// sampling mandatory, revalidate source routes or change any governance gate.
func preparationChecks(v v1.IntentConversation) []v1.IntentPreparationCheck {
	checks := []v1.IntentPreparationCheck{}
	add := func(code, state, message string) {
		checks = append(checks, v1.IntentPreparationCheck{Code: code, State: state, Message: message})
	}
	goalDigest := ""
	if v.Goal == nil {
		add("goal", "missing", "Supply a typed reliability goal.")
	} else if model.ValidateIntent(*v.Goal) != nil {
		add("goal", "invalid", "Review the saved goal; it no longer satisfies the intent contract.")
	} else {
		goalDigest = hash(*v.Goal)
		add("goal", "recorded", "A typed goal is saved. Its description does not prove measurement semantics.")
	}
	if v.Goal != nil && v.Goal.Spec.Measurement != nil {
		add("measurement", "recorded", "An inline measurement is saved; review its population, success criteria, observation point and exclusions.")
	} else if v.Selection != nil && v.Selection.Binding.Name != "" {
		add("measurement", "recorded", "A legacy measurement selection is saved; its confirmation and revision still require review.")
	} else {
		add("measurement", "missing", "Define an inline measurement or select an existing shared measurement. Candidate metric names alone are insufficient.")
	}
	compiled := savedCompilationMatches(v, goalDigest)
	if v.Plan == nil || v.Plan.Compiled == nil {
		add("compilation", "missing", "Verify and compile the exact measurement before freezing a proposal.")
	} else if !compiled {
		add("compilation", "stale", "The saved compilation does not match this goal and manifest. Prepare fresh evidence.")
	} else {
		add("compilation", "recorded", "A matching compilation is saved. This does not establish current traffic, coverage or deployed rules.")
	}
	if v.Selection == nil {
		add("measurement_confirmation", "missing", "An authorized human must confirm the exact measurement; the agent cannot confirm it.")
	} else if !compiled || v.Selection.IntentDigest != goalDigest || !selectionMatchesCompilation(v) {
		add("measurement_confirmation", "stale", "The saved confirmation does not match this goal and compilation. Request a fresh human review.")
	} else {
		add("measurement_confirmation", "recorded", "A matching measurement confirmation is saved. Change approval remains separate.")
	}
	if v.Backtest == nil {
		add("historical_evidence", "not_recorded", "No historical sampling is saved. Run it only for an explicitly requested range; absence is not a failed backtest.")
	} else if !compiled || v.Backtest.IntentDigest != goalDigest || v.Backtest.ManifestDigest != v.Plan.Compiled.Digest || v.Backtest.SourceDigest != v.Plan.SourceDigest {
		add("historical_evidence", "stale", "Historical sampling belongs to a different goal, manifest or saved source route.")
	} else {
		add("historical_evidence", "recorded", "Matching sampled history is saved. Inspect unknown and no-traffic cutoffs; this is not continuous alert replay.")
	}
	if v.PreparationEvidence == nil {
		add("preparation_evidence", "not_recorded", "No provider evidence receipt is attached. Explicit collection requires a confirmed inline measurement.")
	} else if !preparationEvidenceMatches(v) {
		add("preparation_evidence", "stale", "The attached receipt does not match the saved goal, measurement, manifest or source provenance.")
	} else {
		add("preparation_evidence", "recorded", "A matching server-generated receipt is attached. Inspect its state, quality and gaps; this status does not revalidate the provider.")
	}
	add("current_provider_evidence", "not_checked", "This status reads saved evidence only. Use intent evidence for an explicit current provider read; no current source or catalog verification is claimed.")
	return checks
}
func savedCompilationMatches(v v1.IntentConversation, goalDigest string) bool {
	if goalDigest == "" || v.Plan == nil || v.Plan.Status != "ready" || v.Plan.SourceDigest == "" || v.Plan.Compiled == nil || v.Plan.IntentDigest != goalDigest || hash(v.Plan.Intent) != goalDigest {
		return false
	}
	c := v.Plan.Compiled
	if v.Goal.Spec.Measurement != nil && c.MeasurementDigest != service.MeasurementDigest(model.SLODefinition(c.Manifest).SLO()) {
		return false
	}
	return c.Manifest.Metadata == v.Goal.Metadata && c.Manifest.Spec.Intent != nil && hash(c.Manifest.Spec.Intent) == hash(&v.Goal.Spec) && c.Digest == hash(c.Manifest) && model.SLODefinition(c.Manifest).Validate() == nil
}
func selectionMatchesCompilation(v v1.IntentConversation) bool {
	c := v.Plan.Compiled
	if v.Goal.Spec.Measurement != nil {
		return c.MeasurementDigest == service.MeasurementDigest(model.SLODefinition(c.Manifest).SLO()) && v.Selection.MeasurementDigest == c.MeasurementDigest && v.Selection.Binding.Name == ""
	}
	return c.Binding.Name != "" && v.Selection.MeasurementDigest == "" && v.Selection.Binding.Name == c.Binding.Name && v.Selection.Binding.ExpectedRevision == c.Binding.Revision
}
