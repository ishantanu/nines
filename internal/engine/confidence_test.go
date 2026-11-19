package engine

import (
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func confidencePolicy(confidence float64) *model.Policy {
	return &model.Policy{Name: "fast", Severity: "critical", ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 14.4, MinCoverage: 1, Confidence: confidence}
}
func steady(good, total float64) map[int64]model.Point {
	points := map[int64]model.Point{}
	for _, offset := range []int{-120, -60, 0} {
		at := epoch.Add(time.Duration(offset) * time.Second)
		points[at.Unix()] = model.Point{Time: at, Good: good, Total: total}
	}
	return points
}

func TestConfidenceSuppressesLowTrafficBreachesButNotBusyOnes(t *testing.T) {
	// A 20% error rate is a 20x burn on a 99% target: over the 14.4x policy.
	for _, tc := range []struct {
		name       string
		confidence float64
		good       float64
		total      float64
		fires      bool
	}{
		{"plain policy fires on ten events", 0, 8, 10, true},
		{"confidence holds back ten events", 0.99, 8, 10, false},
		{"confidence still fires on a large clear breach", 0.99, 8000, 10000, true},
		{"confidence does not fire below the threshold at any volume", 0.99, 9990, 10000, false},
	} {
		s := config()
		s.Policy = confidencePolicy(tc.confidence)
		state, _ := Advance(s, steady(tc.good, tc.total), model.AlertState{}, epoch)
		if (state.Phase == "firing") != tc.fires {
			t.Errorf("%s: phase %q, want fires=%v", tc.name, state.Phase, tc.fires)
		}
	}
}
