package engine

import (
	"math"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestHistoryIndexedSumsMatchEvaluationAndKeepGaps(t *testing.T) {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	slo := model.SLO{ID: "s", Target: .99, StepSeconds: 60, WindowSeconds: 3600, Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 600, BurnRate: 2, MinCoverage: 1}}
	points := map[int64]model.Point{}
	for i := 0; i < 500; i++ {
		ts := at.Add(-time.Duration(i) * time.Minute)
		if i%37 == 0 {
			continue
		}
		total := float64(100 + (i%19)*10)
		if i == 12 {
			total = 0
		}
		points[ts.Unix()] = model.Point{Time: ts, Good: total * .98, Total: total}
	}
	index := indexPoints(points)
	for i := 0; i < 200; i++ {
		for _, window := range []int64{60, 300, 3600} {
			ts := at.Add(-time.Duration(i) * time.Minute)
			want := Evaluate(slo, points, ts, window)
			got := index.evaluate(slo, ts, window)
			if math.Abs(want.Good-got.Good) > 1e-8 || math.Abs(want.Total-got.Total) > 1e-8 || got.Coverage != want.Coverage {
				t.Fatal(i, window, got, want)
			}
			if (got.Reliability == nil) != (want.Reliability == nil) {
				t.Fatal("zero/missing data became reliability")
			}
		}
	}
	history := HistorySeries(slo, points, at.Add(-20*time.Minute), at, 60)
	if len(history.Points) != 21 {
		t.Fatal(len(history.Points))
	}
	if history.Points[20].IntervalReliability != nil {
		t.Fatal("missing interval bridged")
	}
	if history.Points[8].IntervalReliability != nil {
		t.Fatal("zero traffic became success")
	}
	for _, p := range history.Points {
		if p.Coverage < 1 && p.TargetViolated {
			t.Fatal("partial window declared violation")
		}
	}
}
func TestHistorySeparatesIntervalRollingAndBurnConditions(t *testing.T) {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	s := model.SLO{ID: "s", Target: .99, StepSeconds: 60, WindowSeconds: 600, Policy: &model.Policy{ShortWindowSeconds: 120, LongWindowSeconds: 300, BurnRate: 2, MinCoverage: 1}}
	points := map[int64]model.Point{}
	for i := 0; i < 20; i++ {
		ts := at.Add(-time.Duration(i) * time.Minute)
		good := 100.
		if i < 5 {
			good = 95
		}
		points[ts.Unix()] = model.Point{Time: ts, Good: good, Total: 100}
	}
	h := HistorySeries(s, points, at.Add(-9*time.Minute), at, 60)
	last := h.Points[len(h.Points)-1]
	if !last.BelowTarget || !last.TargetViolated || !last.BurnThresholdBreached {
		t.Fatal(last)
	}
	kinds := map[string]bool{}
	for _, v := range h.Violations {
		kinds[v.Kind] = true
	}
	if len(kinds) != 3 {
		t.Fatal(h.Violations)
	}
}
