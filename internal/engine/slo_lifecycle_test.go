package engine

import (
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestMinEventsInterruptsPendingAndRetainsFiringUntilObservedRecovery(t *testing.T) {
	s := config()
	s.Policy = &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 2, MinCoverage: 1, ForSeconds: 60, MinEvents: 100}
	shortCount, longCount, burn := 99.0, 1000.0, 10.0
	evaluate := func(window int64) model.Snapshot {
		total := longCount
		if window == 60 {
			total = shortCount
		}
		return model.Snapshot{Total: total, Coverage: 1, BurnRate: &burn}
	}
	state, event := advanceWith(s, model.AlertState{}, epoch, evaluate)
	if state.Phase != "inactive" || event != nil {
		t.Fatal("long-window volume bypassed short guard", state, event)
	}
	shortCount, longCount = 1000, 99
	state, event = advanceWith(s, state, epoch.Add(time.Minute), evaluate)
	if state.Phase != "inactive" || event != nil {
		t.Fatal("short-window volume bypassed long guard", state, event)
	}
	shortCount, longCount = 100, 100
	state, _ = advanceWith(s, state, epoch.Add(2*time.Minute), evaluate)
	if state.Phase != "pending" {
		t.Fatal(state)
	}
	shortCount = 1
	state, _ = advanceWith(s, state, epoch.Add(3*time.Minute), evaluate)
	if state.Phase != "inactive" || state.PendingSince != nil {
		t.Fatal("low traffic did not reset pending", state)
	}
	shortCount = 100
	state, _ = advanceWith(s, state, epoch.Add(4*time.Minute), evaluate)
	state, event = advanceWith(s, state, epoch.Add(5*time.Minute), evaluate)
	if state.Phase != "firing" || event == nil || event.Kind != "firing" {
		t.Fatal(state, event)
	}
	shortCount, burn = 1, 0
	state, event = advanceWith(s, state, epoch.Add(6*time.Minute), evaluate)
	if state.Phase != "firing" || event != nil {
		t.Fatal("insufficient traffic falsely resolved episode", state, event)
	}
	shortCount = 100
	state, event = advanceWith(s, state, epoch.Add(7*time.Minute), evaluate)
	if state.Phase != "inactive" || event == nil || event.Kind != "resolved" {
		t.Fatal(state, event)
	}
}

func TestLifecycleSuppressesAllPoliciesWithoutDiscardingMeasurement(t *testing.T) {
	s := config()
	s.Policies = []model.Policy{{Name: "fast", Severity: "critical", ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 2, MinCoverage: 1}}
	r := model.Record{SLO: s, Points: map[int64]model.Point{}}
	for i := 0; i < 3; i++ {
		at := epoch.Add(-time.Duration(i) * time.Minute)
		r.Points[at.Unix()] = model.Point{Time: at, Good: 0, Total: 100}
	}
	for _, lifecycle := range []v1.SLOLifecycle{v1.LifecycleExperimental, v1.LifecycleAspirational, v1.LifecycleDeprecated} {
		r.SLO.Lifecycle = lifecycle
		r.Alert = model.AlertState{Phase: "firing", Policies: map[string]model.AlertState{"fast": {Phase: "firing"}}}
		state, events := AdvanceAllRecord(r, epoch)
		if state.Phase != "inactive" || state.SuppressedReason != "lifecycle_"+string(lifecycle) || len(events) != 0 || state.Policies["fast"].SuppressedReason != state.SuppressedReason {
			t.Fatal(state, events)
		}
		measured := EvaluateRecord(r, epoch, s.WindowSeconds)
		if measured.Total != 300 || measured.Coverage != 1 || measured.Reliability == nil || *measured.Reliability != 0 {
			t.Fatal("suppression lost measurement", measured)
		}
		r.Alert = state
	}
	r.SLO.Lifecycle = v1.LifecycleCommitted
	// Supply fresh observations for the new cutoff, including the completed long window.
	at := epoch.Add(time.Minute)
	r.Points[at.Unix()] = model.Point{Time: at, Total: 100}
	state, events := AdvanceAllRecord(r, at)
	if state.Phase != "firing" || state.SuppressedReason != "" || len(events) != 1 {
		t.Fatal("promotion failed to enable alerts", state, events)
	}
}

func TestProviderWindowTrafficGuardUsesCountsFromEachWholeWindow(t *testing.T) {
	s := config()
	s.Revision = 1
	s.ProviderWindow = &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 15}
	s.Policy = &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 2, MinCoverage: 1, MinEvents: 100}
	r := model.Record{SLO: s, Windows: map[string]model.WindowObservation{}}
	for _, w := range []int64{60, 180} {
		r.Windows[model.WindowKey(w, epoch)] = model.WindowObservation{Time: epoch, WindowSeconds: w, Revision: 1, Total: 99, Coverage: 1}
	}
	state, event := AdvanceRecord(r, epoch)
	if state.Phase != "inactive" || event != nil {
		t.Fatal(state, event)
	}
	for key, observation := range r.Windows {
		observation.Total = 100
		r.Windows[key] = observation
	}
	state, event = AdvanceRecord(r, epoch)
	if state.Phase != "firing" || event == nil {
		t.Fatal(state, event)
	}
}
