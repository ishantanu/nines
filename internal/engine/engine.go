// Package engine defines independent, deterministic event-ratio SLO semantics.
package engine

import (
	"math"
	"sort"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func Evaluate(s model.SLO, points map[int64]model.Point, at time.Time, window int64) model.Snapshot {
	at = at.UTC().Truncate(time.Duration(s.StepSeconds) * time.Second)
	from := at.Add(-time.Duration(window) * time.Second)
	r := model.Snapshot{SLOID: s.ID, At: at, From: from, ExpectedIntervals: window / s.StepSeconds, DataStatus: "no_data"}
	// Sorting makes floating-point summation deterministic across map iteration,
	// backfills and live ingestion. Compensated sums reduce accumulation error.
	keys := make([]int64, 0, len(points))
	for k := range points {
		if k > from.Unix() && k <= at.Unix() {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var cg, ct float64
	for _, k := range keys {
		p := points[k]
		yg := p.Good - cg
		tg := r.Good + yg
		cg = (tg - r.Good) - yg
		r.Good = tg
		yt := p.Total - ct
		tt := r.Total + yt
		ct = (tt - r.Total) - yt
		r.Total = tt
		r.ObservedIntervals++
	}
	r.Coverage = float64(r.ObservedIntervals) / float64(r.ExpectedIntervals)
	r.Bad = math.Max(0, r.Total-r.Good)
	if r.Total > 0 {
		reliability := r.Good / r.Total
		allowed := r.Total * (1 - s.Target)
		remaining := allowed - r.Bad
		// Subtraction and decimal target conversion can leave a few ULPs of
		// residual at exact exhaustion. Treat only that representational noise
		// as zero, rather than reporting a negative-zero budget.
		tolerance := 8 * (math.Nextafter(r.Total, math.Inf(1)) - r.Total)
		if math.Abs(remaining) <= tolerance {
			remaining = 0
		}
		fraction := remaining / allowed
		burn := (r.Bad / r.Total) / (1 - s.Target)
		r.Reliability = &reliability
		r.BudgetAllowed = &allowed
		r.BudgetRemaining = &remaining
		r.BudgetRemainingFraction = &fraction
		r.BurnRate = &burn
		r.DataStatus = "partial"
		if r.ObservedIntervals == r.ExpectedIntervals {
			r.DataStatus = "complete"
		}
	} else if r.ObservedIntervals > 0 {
		r.DataStatus = "no_traffic"
	}
	return r
}

// Advance never resolves a firing alert on missing data. Pending requires
// consecutive evaluation intervals and both burn windows to meet the threshold.
func Advance(s model.SLO, points map[int64]model.Point, previous model.AlertState, at time.Time) (model.AlertState, *model.AlertEvent) {
	return advanceWith(s, previous, at, func(window int64) model.Snapshot { return Evaluate(s, points, at, window) })
}
func advanceWith(s model.SLO, previous model.AlertState, at time.Time, evaluate func(int64) model.Snapshot) (model.AlertState, *model.AlertEvent) {
	if previous.Phase == "" {
		previous.Phase = "inactive"
	}
	if s.Policy == nil {
		return previous, nil
	}
	at = at.UTC().Truncate(time.Duration(s.StepSeconds) * time.Second)
	if previous.LastEvaluated != nil && at.Before(*previous.LastEvaluated) {
		return previous, nil
	}
	next := previous
	next.LastEvaluated = &at
	p := s.Policy
	short := evaluate(p.ShortWindowSeconds)
	long := evaluate(p.LongWindowSeconds)
	sufficient := short.BurnRate != nil && long.BurnRate != nil && short.Coverage >= p.MinCoverage && long.Coverage >= p.MinCoverage
	if !sufficient {
		if previous.Phase != "firing" {
			next.Phase = "inactive"
			next.PendingSince = nil
		}
		return next, nil
	}
	breach := *short.BurnRate >= p.BurnRate && *long.BurnRate >= p.BurnRate
	if !breach {
		next.Phase = "inactive"
		next.PendingSince = nil
		if previous.Phase == "firing" {
			return next, event(s, "resolved", at, *short.BurnRate, *long.BurnRate)
		}
		return next, nil
	}
	if previous.Phase == "firing" {
		return next, nil
	}
	gap := previous.LastEvaluated == nil || at.Sub(*previous.LastEvaluated) > time.Duration(s.StepSeconds)*time.Second
	if previous.Phase != "pending" || previous.PendingSince == nil || gap {
		next.Phase = "pending"
		next.PendingSince = &at
	}
	if at.Sub(*next.PendingSince) >= time.Duration(p.ForSeconds)*time.Second {
		next.Phase = "firing"
		next.PendingSince = nil
		return next, event(s, "firing", at, *short.BurnRate, *long.BurnRate)
	}
	return next, nil
}
func event(s model.SLO, kind string, at time.Time, short, long float64) *model.AlertEvent {
	return &model.AlertEvent{ID: model.NewID(), SLOID: s.ID, Kind: kind, Time: at, ShortBurn: short, LongBurn: long}
}
