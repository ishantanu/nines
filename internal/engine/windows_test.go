package engine

import (
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestProviderWindowsNeverSumShortEstimates(t *testing.T) {
	at := time.Unix(1800000000, 0).UTC()
	s := model.SLO{ID: "latency", Target: .99, WindowSeconds: 120, StepSeconds: 10, Revision: 2, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 2}, Policy: &model.Policy{ShortWindowSeconds: 20, LongWindowSeconds: 40, BurnRate: 5, MinCoverage: 1}}
	r := model.Record{SLO: s, Points: map[int64]model.Point{}, Windows: map[string]model.WindowObservation{}}
	for i := 0; i < 12; i++ {
		ts := at.Add(-time.Duration(i) * 10 * time.Second)
		r.Points[ts.Unix()] = model.Point{Time: ts, Good: 95, Total: 100}
	}
	for _, w := range s.Windows() {
		o := model.WindowObservation{Time: at, WindowSeconds: w, Revision: 2, Good: 82, Total: 100, Coverage: 1}
		r.Windows[model.WindowKey(w, at)] = o
	}
	got := EvaluateRecord(r, at, 120)
	if got.Reliability == nil || *got.Reliability != .82 || got.Good != 82 || got.Total != 100 || got.Coverage != 1 {
		t.Fatal(got)
	}
	state, event := AdvanceRecord(r, at)
	if state.Phase != "firing" || event == nil {
		t.Fatal(state, event)
	}
	h := HistoryRecord(r, at.Add(-10*time.Second), at, 10)
	if h.Points[0].Reliability != nil || h.Points[1].Reliability == nil || *h.Points[1].Reliability != .82 || !h.Points[1].BurnThresholdBreached {
		t.Fatal(h)
	}
	// Evidence from another revision is never reused, and missing/partial short
	// windows cannot fire or falsely resolve an already firing episode.
	o := r.Windows[model.WindowKey(120, at)]
	o.Revision = 1
	r.Windows[model.WindowKey(120, at)] = o
	if EvaluateRecord(r, at, 120).Reliability != nil {
		t.Fatal("old revision reused")
	}
	o = r.Windows[model.WindowKey(20, at)]
	o.Coverage = .5
	r.Windows[model.WindowKey(20, at)] = o
	r.Alert = state
	next, e := AdvanceRecord(r, at.Add(10*time.Second))
	if next.Phase != "firing" || e != nil {
		t.Fatal(next, e)
	}
	r.Alert = model.AlertState{}
	next, e = AdvanceRecord(r, at)
	if next.Phase == "firing" || e != nil {
		t.Fatal("partial coverage fired", next, e)
	}
}
