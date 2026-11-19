package model

import "math"

// Good-interval SLOs count judged intervals rather than events.
//
// For each step interval with total events T and good events G:
//   - T below the judging floor (at least one event, or MinEvents when larger)
//     is not judged: it adds nothing to either count.
//   - otherwise the interval is judged: it counts 1 toward the total, and 1
//     toward good when the error rate (T-G)/T is at most ErrorRate.
//
// The engine and the generated recording rules use this one definition. Each
// judged interval has equal weight, so a quiet interval counts as much as a
// busy one. That is intended for noisy signals, and the opposite of an
// event-weighted ratio.

// JudgingFloor is the fewest events an interval needs to be judged.
func JudgingFloor(minEvents float64) float64 { return math.Max(1, minEvents) }

// ClassifyInterval turns one interval's event counts into judged-interval
// counts: (1,1) good, (0,1) bad, (0,0) not judged.
func ClassifyInterval(good, total float64, errorRate, minEvents float64) (float64, float64) {
	if total < JudgingFloor(minEvents) {
		return 0, 0
	}
	if (total-good)/total <= errorRate {
		return 1, 1
	}
	return 0, 1
}

func validateBadInterval(s SLO) error {
	b := s.BadInterval
	if b == nil {
		return nil
	}
	if !Finite(b.ErrorRate) || b.ErrorRate <= 0 || b.ErrorRate >= 1 {
		return Invalid("badInterval.errorRate must be above 0 and below 1")
	}
	if !Finite(b.MinEvents) || b.MinEvents < 0 || b.MinEvents > 1e18 {
		return Invalid("badInterval.minEvents must be a finite count between 0 and 1e18")
	}
	if s.GoodQuery == "" || s.TotalQuery == "" || s.ProviderWindow != nil || s.Measurement != nil || s.MeasurementBinding != nil || s.Intent != nil {
		return Invalid("badInterval requires goodQuery and totalQuery interval counts, not an inline measurement, binding or intent")
	}
	if s.QueryLanguage != "" && s.QueryLanguage != "promql" {
		return Invalid("badInterval supports PromQL sources only")
	}
	if s.Target <= 0 || s.Target >= 1 {
		return Invalid("badInterval needs a target between 0 and 1")
	}
	return nil
}
