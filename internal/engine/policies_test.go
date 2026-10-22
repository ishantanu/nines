package engine

import (
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestSeverityPoliciesHaveIndependentEpisodesAndMissingData(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Minute)
	s := model.SLO{ID: "api", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 60, WindowSeconds: 600, Policies: []model.Policy{{Name: "fast", Severity: "critical", ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 5, MinCoverage: 1}, {Name: "slow", Severity: "warning", ShortWindowSeconds: 120, LongWindowSeconds: 300, BurnRate: 1, ForSeconds: 60, MinCoverage: 1}}}
	r := model.Record{SLO: s, Points: map[int64]model.Point{}}
	for i := 0; i < 10; i++ {
		ts := at.Add(-time.Duration(i) * time.Minute)
		r.Points[ts.Unix()] = model.Point{Time: ts, Good: 98, Total: 100}
	}
	state, events := AdvanceAllRecord(r, at)
	if len(events) != 0 || state.Policies["slow"].Phase != "pending" || state.Policies["fast"].Phase != "inactive" {
		t.Fatal(state, events)
	}
	r.Alert = state
	next := at.Add(time.Minute)
	r.Points[next.Unix()] = model.Point{Time: next, Good: 98, Total: 100}
	state, events = AdvanceAllRecord(r, next)
	if len(events) != 1 || events[0].Policy != "slow" || events[0].Severity != "warning" {
		t.Fatal(state, events)
	}
	r.Alert = state
	next = next.Add(time.Minute)
	r.Points[next.Unix()] = model.Point{Time: next, Good: 80, Total: 100}
	state, events = AdvanceAllRecord(r, next)
	if len(events) != 1 || events[0].Policy != "fast" || events[0].Severity != "critical" || state.Phase != "firing" {
		t.Fatal(state, events)
	}
	r.Alert = state
	r.Points = nil
	state, events = AdvanceAllRecord(r, next.Add(time.Minute))
	if len(events) != 0 || state.Policies["slow"].Phase != "firing" || state.Policies["fast"].Phase != "firing" {
		t.Fatal("missing data resolved alerts", state, events)
	}
}
