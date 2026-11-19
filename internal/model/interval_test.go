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
		g, n := ClassifyInterval(tc.good, tc.total, tc.rate, tc.floor)
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
