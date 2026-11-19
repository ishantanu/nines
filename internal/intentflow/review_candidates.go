package intentflow

import (
	"encoding/json"
	"maps"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// Inspect only the newest receipt: an invalid/newer result must not silently
// fall back to an older successful discovery. No provider/model call is made.
func candidateReviewReceipt(v v1.IntentConversation) *v1.IntentReviewCandidateReceipt {
	for i := len(v.Runs) - 1; i >= 0; i-- {
		run := v.Runs[i]
		for j := len(run.Steps) - 1; j >= 0; j-- {
			step := run.Steps[j]
			if step.Tool != "discover_measurement_candidates" {
				continue
			}
			out := &v1.IntentReviewCandidateReceipt{State: "invalid", RunID: run.ID, RunInputDigest: run.InputDigest, StepDigest: step.Digest, IntentDigest: step.IntentDigest, At: step.At}
			var report v1.IntentCandidateDiscovery
			if run.Mode != "model" || !run.ModelConsent || !auditDigestPattern.MatchString(run.InputDigest) || !auditDigestPattern.MatchString(step.Digest) || len(step.Result) == 0 || len(step.Result) > 64<<10 || bytesDigest(step.Result) != step.Digest || json.Unmarshal(step.Result, &report) != nil || report.APIVersion != v1.APIVersion || report.Tenant != v.Tenant || !report.HumanConfirmationRequired || report.ObservedAt.IsZero() || step.At.IsZero() || report.ObservedAt.After(step.At) || step.At.Before(run.CreatedAt) || step.At.After(run.UpdatedAt) || !auditDigestPattern.MatchString(report.SourceDigest) || !auditDigestPattern.MatchString(report.CatalogDigest) || report.CatalogDigest != run.TemplateCatalogDigest {
				return out
			}
			if v.Goal == nil || v.Plan == nil || report.Scope.Source != v.Scope.Source || report.Scope.Service != v.Scope.Service || report.Scope.Project != v.Scope.Metadata.Project || !maps.Equal(report.Scope.Population, v.Scope.Population) || report.SourceDigest != hash(struct{ Tenant, Source, Route string }{v.Tenant, v.Scope.Source, v.Plan.SourceDigest}) || (v.Template != nil && report.CatalogDigest != v.Template.CatalogDigest) || (v.PreparationEvidence != nil && report.CatalogDigest != v.PreparationEvidence.Report.CatalogDigest) {
				out.State = "stale"
				return out
			}
			out.Report = clone(&report)
			switch {
			case step.IntentDigest == "":
				out.State = "unverified"
			case !auditDigestPattern.MatchString(step.IntentDigest):
				out.State = "invalid"
				out.Report = nil
			case step.IntentDigest != hash(*v.Goal):
				out.State = "prior_goal"
			default:
				out.State = "recorded"
			}
			return out
		}
	}
	return nil
}
