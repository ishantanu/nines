package intentflow

import (
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// Supplied facts come only from the saved typed goal/scope and confirmation.
// Free text is not guessed or parsed here; unresolved semantics stay unresolved.
func clarificationFields(v v1.IntentConversation, fields []string) (supplied, unresolved []string, valid bool) {
	known := map[string]bool{"service": v.Scope.Service != "", "source": v.Scope.Source != "", "project": v.Scope.Metadata.Project != "", "population": len(v.Scope.Population) > 0, "outcome": false, "target": false, "window": false, "threshold": false, "metric": false, "success": false, "observationPoint": false, "exclusions": false, "confirmation": false, "semantics": false}
	if g := v.Goal; g != nil {
		known["outcome"] = g.Spec.Outcome != ""
		known["target"] = g.Spec.Target > 0 && g.Spec.Target < 1
		known["window"] = g.Spec.WindowSeconds > 0
		known["threshold"] = !model.ThresholdMeasurement(g.Spec.Outcome) || g.Spec.ThresholdSeconds != nil
		known["exclusions"] = g.Spec.Exclusions != nil
		if m := g.Spec.Measurement; m != nil {
			known["metric"] = m.Metric != ""
			known["success"] = len(m.SuccessLabels) > 0 || (model.ThresholdMeasurement(m.Type) && m.ThresholdSeconds != nil && *m.ThresholdSeconds > 0)
			known["observationPoint"] = m.ObservationPoint != ""
		}
		known["confirmation"] = v.Selection != nil && v.Selection.IntentDigest == hash(*g) && savedCompilationMatches(v, hash(*g)) && selectionMatchesCompilation(v)
	}
	supplied, unresolved = []string{}, []string{}
	seen := map[string]bool{}
	for _, field := range fields {
		value, ok := known[field]
		if !ok || seen[field] {
			return nil, nil, false
		}
		seen[field] = true
		if value {
			supplied = append(supplied, field)
		} else {
			unresolved = append(unresolved, field)
		}
	}
	return supplied, unresolved, true
}
