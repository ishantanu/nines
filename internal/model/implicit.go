package model

import (
	"fmt"
	"math"
)

// Users come to rely on the service level they actually receive: the implicit
// SLO. When a target changes, these notes compare it with what the service has
// delivered over the SLO window, so a change that users will feel is reviewed
// as such. They are advice shown with the plan; they never block an apply.

const (
	// LargeErrorBudgetChange is the factor by which the allowed error rate must
	// change to be called large (99.9% to 99.99% is a tenfold change).
	LargeErrorBudgetChange = 10.0
	// MaterialGapFactor is how many times more failure the new target must
	// allow than the service delivers before loosening is called out.
	MaterialGapFactor = 10.0
	// observedErrorFloor stops a perfect window from producing infinite ratios.
	observedErrorFloor = 1e-6
)

// ImplicitTargetNotes compares a changed target with delivered reliability.
// observed is nil when the window has no measured reliability. The notes are
// empty when the target did not change.
func ImplicitTargetNotes(oldTarget, newTarget float64, observed *float64, coverage float64) []string {
	if oldTarget == newTarget || !Finite(oldTarget) || !Finite(newTarget) || oldTarget <= 0 || newTarget <= 0 || oldTarget >= 1 || newTarget >= 1 {
		return nil
	}
	notes := []string{}
	oldAllowed, newAllowed := 1-oldTarget, 1-newTarget
	if factor := math.Max(oldAllowed/newAllowed, newAllowed/oldAllowed); factor >= LargeErrorBudgetChange-1e-9 {
		direction := "tightens"
		if newTarget < oldTarget {
			direction = "relaxes"
		}
		notes = append(notes, fmt.Sprintf("This change %s the allowed failure rate by %.0f times (%.4g%% to %.4g%%). A change this large alters the error budget and what users can expect.", direction, factor, oldTarget*100, newTarget*100))
	}
	if observed == nil || coverage < 1 {
		return append(notes, "Reliability delivered over the SLO window is not fully measured, so this target change cannot be compared with what users have been receiving.")
	}
	delivered := *observed
	deliveredError := math.Max(1-delivered, observedErrorFloor)
	switch {
	case newTarget > oldTarget && newTarget > delivered:
		notes = append(notes, fmt.Sprintf("The new target %.4g%% is above the %.4g%% delivered over the last window, so the SLO would start out unmet and its budget would burn from the first minute.", newTarget*100, delivered*100))
	case newTarget < oldTarget && newAllowed/deliveredError >= MaterialGapFactor:
		notes = append(notes, fmt.Sprintf("The service has delivered %.4g%%, but the new target %.4g%% allows about %.0f times more failures. Users tend to rely on the level they actually get, so relaxing toward the target can feel like a regression: communicate the change before relying on the extra room.", delivered*100, newTarget*100, newAllowed/deliveredError))
	}
	return notes
}
