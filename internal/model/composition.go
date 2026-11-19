package model

import (
	"math"
	"sort"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// AvailabilityFact is a committed availability SLO assigned to a service.
type AvailabilityFact struct {
	SLOID  string
	Target float64
}

// CompositionLimitations always accompany a result. The product assumes the
// critical dependencies fail independently and are all required in series.
var CompositionLimitations = []string{
	"The ceiling multiplies the lowest committed availability target of each directly declared critical dependency and assumes they fail independently. Shared infrastructure makes failures correlated, so the real result can be better or worse.",
	"Only committed availability SLOs count. Experimental, aspirational, deprecated and non-availability SLOs are ignored, and dependencies without one are listed as unmeasured.",
	"Only declared critical edges are used. Observed or non-critical edges, retries, caches and hedged calls are not modelled.",
	"The ceiling bounds what can be promised; it does not show that the service meets its SLO.",
}

const compositionTolerance = 1e-12

// ComposeAvailability checks each caller's availability SLO against the product
// of its critical callees' committed availability promises. It is pure: the
// same input always gives the same ordered output.
func ComposeAvailability(deps []v1.DeclaredDependency, facts map[string][]AvailabilityFact) []v1.CompositionCheck {
	callees := map[string][]string{}
	seen := map[[2]string]bool{}
	for _, d := range deps {
		key := [2]string{d.Caller, d.Callee}
		if !d.Critical || d.Caller == d.Callee || seen[key] {
			continue
		}
		seen[key] = true
		callees[d.Caller] = append(callees[d.Caller], d.Callee)
	}
	callers := make([]string, 0, len(facts))
	for service := range facts {
		callers = append(callers, service)
	}
	sort.Strings(callers)
	out := []v1.CompositionCheck{}
	for _, caller := range callers {
		names := append([]string{}, callees[caller]...)
		sort.Strings(names)
		promises := []v1.CompositionPromise{}
		unmeasured := []string{}
		ceiling, known := 1.0, 0
		for _, callee := range names {
			weakest, ok := weakestPromise(facts[callee])
			if !ok {
				unmeasured = append(unmeasured, callee)
				continue
			}
			promises = append(promises, v1.CompositionPromise{Service: callee, SLOID: weakest.SLOID, Target: weakest.Target})
			ceiling *= weakest.Target
			known++
		}
		sorted := append([]AvailabilityFact{}, facts[caller]...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].SLOID < sorted[j].SLOID })
		for _, own := range sorted {
			check := v1.CompositionCheck{Service: caller, SLOID: own.SLOID, Target: own.Target, Dependencies: promises, Unmeasured: unmeasured}
			switch known {
			case 0:
				check.Verdict = "no_ceiling"
			default:
				c, headroom := ceiling, ceiling-own.Target
				check.Ceiling, check.Headroom = &c, &headroom
				switch {
				case own.Target > ceiling+compositionTolerance:
					check.Verdict = "exceeds_ceiling"
				case len(unmeasured) > 0:
					check.Verdict = "within_known_ceiling"
				default:
					check.Verdict = "within_ceiling"
				}
				if math.Abs(headroom) < compositionTolerance {
					zero := 0.0
					check.Headroom = &zero
				}
			}
			out = append(out, check)
		}
	}
	return out
}

func weakestPromise(facts []AvailabilityFact) (AvailabilityFact, bool) {
	if len(facts) == 0 {
		return AvailabilityFact{}, false
	}
	weakest := facts[0]
	for _, f := range facts[1:] {
		if f.Target < weakest.Target || (f.Target == weakest.Target && f.SLOID < weakest.SLOID) {
			weakest = f
		}
	}
	return weakest, true
}

// IsAvailabilitySLO reports whether an SLO is declared as an availability ratio.
// SLOs defined by custom queries cannot be classified and are not included.
func IsAvailabilitySLO(s SLO) bool {
	if s.Measurement != nil {
		return s.Measurement.Type == "availability"
	}
	return s.Intent != nil && s.Intent.Outcome == "availability"
}
