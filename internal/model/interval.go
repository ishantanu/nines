package model

import (
	"math"

	"github.com/ishantanu/nines/api/v1alpha1"
)

// Interval-judged SLOs count judged intervals rather than events.
//
// For each step interval with total events T and good events G:
//   - Good-minute (errorRate): T below the judging floor (at least one event,
//     or MinEvents when larger) is not judged and adds nothing to either
//     count. Otherwise the interval counts 1 toward the total, and 1 toward
//     good when the error rate (T-G)/T is at most ErrorRate.
//   - Throughput (minGoodEvents): every observed interval counts 1 toward the
//     total, and 1 toward good when G is at least MinGoodEvents. An interval
//     with no data is absent, not bad: missing telemetry stays unknown.
//
// The engine and the generated recording rules use this one definition. Each
// judged interval has equal weight, so a quiet interval counts as much as a
// busy one. That is intended for noisy or volume-based signals, and the
// opposite of an event-weighted ratio.

// JudgingFloor is the fewest events an interval needs to be judged.
func JudgingFloor(minEvents float64) float64 { return math.Max(1, minEvents) }

// ClassifyInterval turns one interval's event counts into judged-interval
// counts: (1,1) good, (0,1) bad, (0,0) not judged.
func ClassifyInterval(good, total float64, p v1alpha1.BadIntervalPolicy) (float64, float64) {
	if p.Throughput() {
		if good >= p.MinGoodEvents {
			return 1, 1
		}
		return 0, 1
	}
	if total < JudgingFloor(p.MinEvents) {
		return 0, 0
	}
	if (total-good)/total <= p.ErrorRate {
		return 1, 1
	}
	return 0, 1
}

func validateBadInterval(s SLO) error {
	b := s.BadInterval
	if b == nil {
		return nil
	}
	if b.Throughput() {
		if !Finite(b.MinGoodEvents) || b.MinGoodEvents > 1e18 {
			return Invalid("badInterval.minGoodEvents must be a finite count up to 1e18")
		}
		if b.ErrorRate != 0 || b.MinEvents != 0 {
			return Invalid("badInterval sets either minGoodEvents (throughput) or errorRate with optional minEvents (good-minute), not both")
		}
	} else {
		if b.MinGoodEvents < 0 || !Finite(b.MinGoodEvents) {
			return Invalid("badInterval.minGoodEvents must be a positive count")
		}
		if !Finite(b.ErrorRate) || b.ErrorRate <= 0 || b.ErrorRate >= 1 {
			return Invalid("badInterval needs errorRate above 0 and below 1, or a positive minGoodEvents for throughput")
		}
		if !Finite(b.MinEvents) || b.MinEvents < 0 || b.MinEvents > 1e18 {
			return Invalid("badInterval.minEvents must be a finite count between 0 and 1e18")
		}
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
