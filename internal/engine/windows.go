package engine

import (
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func EvaluateRecord(r model.Record, at time.Time, window int64) model.Snapshot {
	if r.SLO.ProviderWindow == nil {
		return Evaluate(r.SLO, r.Points, at, window)
	}
	s := r.SLO
	at = at.UTC().Truncate(time.Duration(s.StepSeconds) * time.Second)
	result := Evaluate(s, nil, at, window)
	o, ok := r.Windows[model.WindowKey(window, at)]
	if !ok || o.Revision != model.ConfigRevision(s) {
		return result
	}
	// Reuse event-ratio arithmetic on ONE aggregate, then restore actual evidence.
	result = Evaluate(s, map[int64]model.Point{at.Unix(): {Time: at, Good: o.Good, Total: o.Total}}, at, window)
	result.Coverage = o.Coverage
	// Interval counts have no meaning for overlapping provider observations.
	result.ExpectedIntervals = 0
	result.ObservedIntervals = 0
	if o.Total > 0 {
		result.DataStatus = "partial"
		if o.Coverage == 1 {
			result.DataStatus = "complete"
		}
	} else if o.Coverage == 0 {
		result.DataStatus = "no_data"
	}
	return result
}
func AdvanceRecord(r model.Record, at time.Time) (model.AlertState, *model.AlertEvent) {
	if r.SLO.ProviderWindow == nil {
		return Advance(r.SLO, r.Points, r.Alert, at)
	}
	return advanceWith(r.SLO, r.Alert, at, func(window int64) model.Snapshot { return EvaluateRecord(r, at, window) })
}
