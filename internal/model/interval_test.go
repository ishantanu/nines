package model

import (
	"testing"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestClassifyIntervalJudgesAgainstTheErrorRateAndFloor(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		good, total, rate, floor float64
		wantGood, wantTotal      float64
	}{
		{"clean interval is good", 100, 100, 0.06, 0, 1, 1},
		{"noise below the line is good", 95, 100, 0.06, 0, 1, 1},
		{"exactly at the line is good", 94, 100, 0.06, 0, 1, 1},
		{"just over the line is bad", 93, 100, 0.06, 0, 0, 1},
		{"total outage is bad", 0, 50, 0.06, 0, 0, 1},
		{"no traffic is not judged", 0, 0, 0.06, 0, 0, 0},
		{"below the floor is not judged", 1, 4, 0.06, 5, 0, 0},
		{"at the floor is judged", 5, 5, 0.06, 5, 1, 1},
		{"a quiet bad interval counts like a busy one", 0, 1, 0.06, 0, 0, 1},
	} {
		g, n := ClassifyInterval(tc.good, tc.total, v1alpha1.BadIntervalPolicy{ErrorRate: tc.rate, MinEvents: tc.floor})
		if g != tc.wantGood || n != tc.wantTotal {
			t.Errorf("%s: got (%v,%v), want (%v,%v)", tc.name, g, n, tc.wantGood, tc.wantTotal)
		}
	}
}

func TestBadIntervalValidation(t *testing.T) {
	base := func() SLO {
		return SLO{ID: "api", Project: "p", Service: "s", Name: "api", Target: .999, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "sum(increase(g[1m]))", TotalQuery: "sum(increase(t[1m]))"}
	}
	ok := base()
	policy := v1alpha1.BadIntervalPolicy{ErrorRate: 0.06, MinEvents: 5}
	ok.BadInterval = &policy
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid policy was rejected: %v", err)
	}
	for name, mutate := range map[string]func(*SLO){
		"error rate zero": func(s *SLO) { s.BadInterval.ErrorRate = 0 },
		"error rate one":  func(s *SLO) { s.BadInterval.ErrorRate = 1 },
		"negative min":    func(s *SLO) { s.BadInterval.MinEvents = -1 },
		"no queries":      func(s *SLO) { s.GoodQuery, s.TotalQuery = "", "" },
		"non-promql":      func(s *SLO) { s.QueryLanguage = "datadog" },
		"target of one":   func(s *SLO) { s.Target = 1 },
	} {
		s := base()
		policy := v1alpha1.BadIntervalPolicy{ErrorRate: 0.06, MinEvents: 5}
		s.BadInterval = &policy
		mutate(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestClassifyIntervalJudgesThroughputAgainstTheMinimum(t *testing.T) {
	p := v1alpha1.BadIntervalPolicy{MinGoodEvents: 500}
	for _, tc := range []struct {
		name                string
		good, total         float64
		wantGood, wantTotal float64
	}{
		{"at the minimum is good", 500, 500, 1, 1},
		{"above the minimum is good", 900, 1000, 1, 1},
		{"below the minimum is bad", 499, 499, 0, 1},
		// A stalled pipeline is the failure throughput exists to catch: an
		// observed interval with nothing processed is judged, not skipped.
		{"an observed idle interval is bad", 0, 0, 0, 1},
		{"failures do not matter, only volume", 600, 5000, 1, 1},
	} {
		g, n := ClassifyInterval(tc.good, tc.total, p)
		if g != tc.wantGood || n != tc.wantTotal {
			t.Errorf("%s: got (%v,%v), want (%v,%v)", tc.name, g, n, tc.wantGood, tc.wantTotal)
		}
	}
}

func TestThroughputValidation(t *testing.T) {
	base := func(p v1alpha1.BadIntervalPolicy) SLO {
		return SLO{ID: "etl", Project: "p", Service: "s", Name: "etl", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "sum(increase(done[1m]))", TotalQuery: "sum(increase(seen[1m]))", BadInterval: &p}
	}
	if err := base(v1alpha1.BadIntervalPolicy{MinGoodEvents: 500}).Validate(); err != nil {
		t.Fatalf("a valid throughput policy was rejected: %v", err)
	}
	for name, p := range map[string]v1alpha1.BadIntervalPolicy{
		"both rules":                {MinGoodEvents: 500, ErrorRate: 0.05},
		"minEvents with throughput": {MinGoodEvents: 500, MinEvents: 10},
		"neither rule":              {},
		"negative minimum":          {MinGoodEvents: -1},
		"absurd minimum":            {MinGoodEvents: 1e19},
	} {
		if err := base(p).Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
