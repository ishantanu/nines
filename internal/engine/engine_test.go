package engine

import (
	"math"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func config() model.SLO {
	return model.SLO{ID: "test", Project: "p", Service: "s", Name: "test", Target: .99, WindowSeconds: 180, StepSeconds: 60}
}
func closeTo(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 1e-9 {
		t.Fatalf("got %v, want %.12g", got, want)
	}
}
func TestReferenceRatiosAndBoundaries(t *testing.T) {
	s := config()
	points := map[int64]model.Point{}
	for i, p := range []model.Point{
		{Time: epoch.Add(-180 * time.Second), Good: 0, Total: 99999}, // excluded left boundary
		{Time: epoch.Add(-120 * time.Second), Good: 1000, Total: 1000},
		{Time: epoch.Add(-60 * time.Second), Good: 900, Total: 1000},
		{Time: epoch, Good: 1000, Total: 1000},
		{Time: epoch.Add(60 * time.Second), Good: 0, Total: 99999}, // excluded future
	} {
		_ = i
		points[p.Time.Unix()] = p
	}
	r := Evaluate(s, points, epoch, 180)
	closeTo(t, r.Reliability, 29.0/30)
	closeTo(t, r.BudgetAllowed, 30)
	closeTo(t, r.BudgetRemaining, -70)
	closeTo(t, r.BudgetRemainingFraction, -7.0/3)
	closeTo(t, r.BurnRate, 10.0/3)
	if r.DataStatus != "complete" || r.Coverage != 1 || r.Bad != 100 {
		t.Fatalf("unexpected snapshot %+v", r)
	}
}
func TestWeightedCountsRatherThanMeanOfRatios(t *testing.T) {
	s := config()
	p := map[int64]model.Point{epoch.Unix(): {Time: epoch, Good: 10000, Total: 10000}, epoch.Add(-60 * time.Second).Unix(): {Time: epoch.Add(-60 * time.Second), Good: 0, Total: 1}}
	r := Evaluate(s, p, epoch, 180)
	closeTo(t, r.Reliability, 10000.0/10001)
	if r.DataStatus != "partial" || r.Coverage != 2.0/3 {
		t.Fatal(r)
	}
}
func TestNoDataAndZeroTrafficStayUnknown(t *testing.T) {
	s := config()
	r := Evaluate(s, nil, epoch, 180)
	if r.Reliability != nil || r.BurnRate != nil || r.DataStatus != "no_data" {
		t.Fatal(r)
	}
	p := map[int64]model.Point{epoch.Unix(): {Time: epoch}}
	r = Evaluate(s, p, epoch, 180)
	if r.Reliability != nil || r.BudgetRemainingFraction != nil || r.DataStatus != "no_traffic" || r.Coverage != 1.0/3 {
		t.Fatal(r)
	}
}

func TestExhaustedBudgetDoesNotBecomeNegativeZero(t *testing.T) {
	s := config()
	s.Target = .9990000000000001
	p := map[int64]model.Point{epoch.Unix(): {Time: epoch, Good: 999, Total: 1000}}
	r := Evaluate(s, p, epoch, 180)
	if *r.BudgetRemaining != 0 || *r.BudgetRemainingFraction != 0 {
		t.Fatal("floating point noise at exhaustion", r)
	}
	p[epoch.Unix()] = model.Point{Time: epoch, Good: 998.999, Total: 1000}
	r = Evaluate(s, p, epoch, 180)
	if *r.BudgetRemaining >= 0 {
		t.Fatal("meaningful budget exhaustion was clamped")
	}
}
func TestDeterministicSummation(t *testing.T) {
	s := config()
	p := map[int64]model.Point{}
	for i := 0; i < 100; i++ {
		at := epoch.Add(-time.Duration(i) * time.Minute)
		p[at.Unix()] = model.Point{Time: at, Good: float64(i) * .1, Total: float64(i) + 1}
	}
	want := Evaluate(s, p, epoch, 6000)
	for i := 0; i < 50; i++ {
		got := Evaluate(s, p, epoch, 6000)
		if math.Float64bits(got.Good) != math.Float64bits(want.Good) || math.Float64bits(got.Total) != math.Float64bits(want.Total) {
			t.Fatal("nondeterministic sum")
		}
	}
}
func TestAlertPendingFiringMissingDataAndRecovery(t *testing.T) {
	s := config()
	s.Policy = &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, ForSeconds: 120, MinCoverage: 1}
	p := map[int64]model.Point{}
	add := func(at time.Time, good float64) { p[at.Unix()] = model.Point{Time: at, Good: good, Total: 100} }
	add(epoch.Add(-time.Minute), 90)
	add(epoch, 90)
	state, event := Advance(s, p, model.AlertState{}, epoch)
	if state.Phase != "pending" || event != nil {
		t.Fatal(state, event)
	}
	add(epoch.Add(time.Minute), 90)
	state, event = Advance(s, p, state, epoch.Add(time.Minute))
	if state.Phase != "pending" || event != nil {
		t.Fatal(state, event)
	}
	add(epoch.Add(2*time.Minute), 90)
	state, event = Advance(s, p, state, epoch.Add(2*time.Minute))
	if state.Phase != "firing" || event == nil || event.Kind != "firing" {
		t.Fatal(state, event)
	}
	state, event = Advance(s, p, state, epoch.Add(2*time.Minute))
	if event != nil {
		t.Fatal("duplicate firing event")
	}
	state, event = Advance(s, p, state, epoch.Add(3*time.Minute))
	if state.Phase != "firing" || event != nil {
		t.Fatal("missing data resolved an alert")
	}
	add(epoch.Add(4*time.Minute), 100)
	add(epoch.Add(5*time.Minute), 100)
	state, event = Advance(s, p, state, epoch.Add(5*time.Minute))
	if state.Phase != "inactive" || event == nil || event.Kind != "resolved" {
		t.Fatal(state, event)
	}
}
func TestPendingCannotSkipEvaluationsOrMoveBackwards(t *testing.T) {
	s := config()
	s.Policy = &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 60, BurnRate: 2, ForSeconds: 60, MinCoverage: 1}
	p := map[int64]model.Point{epoch.Unix(): {Time: epoch, Good: 90, Total: 100}, epoch.Add(2 * time.Minute).Unix(): {Time: epoch.Add(2 * time.Minute), Good: 90, Total: 100}}
	state, _ := Advance(s, p, model.AlertState{}, epoch)
	later, event := Advance(s, p, state, epoch.Add(2*time.Minute))
	if later.Phase != "pending" || !later.PendingSince.Equal(epoch.Add(2*time.Minute)) || event != nil {
		t.Fatal(later, event)
	}
	earlier, event := Advance(s, p, later, epoch)
	if !earlier.LastEvaluated.Equal(*later.LastEvaluated) || event != nil {
		t.Fatal("historical evaluation altered alert state")
	}
}

func TestAllGoodTrafficKeepsFullBudgetAtNearOneTarget(t *testing.T) {
	s := config()
	s.Target = math.Nextafter(1, 0)
	result := Evaluate(s, map[int64]model.Point{epoch.Unix(): {Time: epoch, Good: 10000, Total: 10000}}, epoch, 60)
	closeTo(t, result.BudgetRemainingFraction, 1)
}
