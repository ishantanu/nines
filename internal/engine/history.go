package engine

import (
	"math"
	"sort"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type HistoryPoint struct {
	ShortCoverage           float64   `json:"shortCoverage"`
	LongCoverage            float64   `json:"longCoverage"`
	At                      time.Time `json:"at"`
	IntervalReliability     *float64  `json:"intervalReliability"`
	Reliability             *float64  `json:"reliability"`
	BudgetRemainingFraction *float64  `json:"budgetRemainingFraction"`
	BurnRate                *float64  `json:"burnRate"`
	ShortBurnRate           *float64  `json:"shortBurnRate"`
	LongBurnRate            *float64  `json:"longBurnRate"`
	Coverage                float64   `json:"coverage"`
	IntervalCoverage        float64   `json:"intervalCoverage"`
	BelowTarget             bool      `json:"belowTarget"`
	TargetViolated          bool      `json:"targetViolated"`
	BurnThresholdBreached   bool      `json:"burnThresholdBreached"`
}
type ViolationSpan struct {
	Kind    string    `json:"kind"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Buckets int       `json:"buckets"`
}
type History struct {
	MeasurementMode    string             `json:"measurementMode"`
	CoverageBasis      string             `json:"coverageBasis"`
	Source             string             `json:"source"`
	SLOID              string             `json:"sloId"`
	ConfigRevision     int64              `json:"configRevision"`
	Target             float64            `json:"target"`
	WindowSeconds      int64              `json:"windowSeconds"`
	StepSeconds        int64              `json:"stepSeconds"`
	ResolutionSeconds  int64              `json:"resolutionSeconds"`
	From               time.Time          `json:"from"`
	To                 time.Time          `json:"to"`
	Policies           []model.Policy     `json:"policies,omitempty"`
	Policy             *model.Policy      `json:"policy,omitempty"`
	Points             []HistoryPoint     `json:"points"`
	Violations         []ViolationSpan    `json:"violations"`
	AlertEvents        []model.AlertEvent `json:"alertEvents"`
	AlertEventsBounded bool               `json:"alertEventsBounded"`
}
type accumulator struct{ value, correction float64 }

func (a *accumulator) add(v float64) {
	y := v - a.correction
	t := a.value + y
	a.correction = (t - a.value) - y
	a.value = t
}

type indexedPoints struct {
	points      []model.Point
	good, total []float64
}

const historyBlock = 64

func indexPoints(points map[int64]model.Point) indexedPoints {
	out := indexedPoints{points: make([]model.Point, 0, len(points))}
	for _, p := range points {
		out.points = append(out.points, p)
	}
	sort.Slice(out.points, func(i, j int) bool { return out.points[i].Time.Before(out.points[j].Time) })
	for start := 0; start < len(out.points); start += historyBlock {
		var g, t accumulator
		for end := start; end < len(out.points) && end < start+historyBlock; end++ {
			g.add(out.points[end].Good)
			t.add(out.points[end].Total)
		}
		out.good = append(out.good, g.value)
		out.total = append(out.total, t.value)
	}
	return out
}
func (x indexedPoints) evaluate(s model.SLO, at time.Time, window int64) model.Snapshot {
	from := at.Add(-time.Duration(window) * time.Second)
	left := sort.Search(len(x.points), func(i int) bool { return x.points[i].Time.After(from) })
	right := sort.Search(len(x.points), func(i int) bool { return x.points[i].Time.After(at) })
	var good, total accumulator
	for i := left; i < right; {
		if i%historyBlock == 0 && i+historyBlock <= right {
			good.add(x.good[i/historyBlock])
			total.add(x.total[i/historyBlock])
			i += historyBlock
		} else {
			good.add(x.points[i].Good)
			total.add(x.points[i].Total)
			i++
		}
	}
	r := model.Snapshot{SLOID: s.ID, At: at, From: from, ExpectedIntervals: window / s.StepSeconds, ObservedIntervals: int64(right - left), Good: good.value, Total: total.value, DataStatus: "no_data"}
	r.Coverage = float64(r.ObservedIntervals) / float64(r.ExpectedIntervals)
	r.Bad = math.Max(0, r.Total-r.Good)
	if r.Total > 0 {
		ratio := r.Good / r.Total
		burn := (r.Bad / r.Total) / (1 - s.Target)
		remaining := r.Total*(1-s.Target) - r.Bad
		if math.Abs(remaining) <= 8*(math.Nextafter(r.Total, math.Inf(1))-r.Total) {
			remaining = 0
		}
		fraction := remaining / (r.Total * (1 - s.Target))
		r.Reliability = &ratio
		r.BurnRate = &burn
		r.BudgetRemainingFraction = &fraction
		r.DataStatus = "partial"
		if r.Coverage == 1 {
			r.DataStatus = "complete"
		}
	} else if r.ObservedIntervals > 0 {
		r.DataStatus = "no_traffic"
	}
	return r
}

// HistorySeries evaluates weighted intervals without bridging gaps or replacing
// missing/zero-traffic data with success. All history uses the current revision.
func HistorySeries(s model.SLO, points map[int64]model.Point, from, to time.Time, resolution int64) History {
	index := indexPoints(points)
	return historyWith(s, from, to, resolution, func(at time.Time, window int64) model.Snapshot { return index.evaluate(s, at, window) })
}
func HistoryRecord(r model.Record, from, to time.Time, resolution int64) History {
	if r.SLO.ProviderWindow == nil {
		return HistorySeries(r.SLO, r.Points, from, to, resolution)
	}
	return historyWith(r.SLO, from, to, resolution, func(at time.Time, window int64) model.Snapshot { return EvaluateRecord(r, at, window) })
}
func historyWith(s model.SLO, from, to time.Time, resolution int64, evaluate func(time.Time, int64) model.Snapshot) History {
	source := s.Source
	if source == "" {
		source = "primary"
	}
	out := History{Source: source, SLOID: s.ID, ConfigRevision: model.ConfigRevision(s), Target: s.Target, WindowSeconds: s.WindowSeconds, StepSeconds: s.StepSeconds, ResolutionSeconds: resolution, From: from, To: to, Policies: s.Policies, Policy: s.Policy, Points: []HistoryPoint{}, Violations: []ViolationSpan{}, AlertEvents: []model.AlertEvent{}, AlertEventsBounded: true}
	out.MeasurementMode = "interval_counts"
	out.CoverageBasis = "interval_presence"
	if s.ProviderWindow != nil {
		out.MeasurementMode = "provider_window"
		out.CoverageBasis = "scrape_density"
	}
	for at := from; !at.After(to); at = at.Add(time.Duration(resolution) * time.Second) {
		rolling := evaluate(at, s.WindowSeconds)
		intervalWindow := resolution
		if s.ProviderWindow != nil {
			intervalWindow = s.StepSeconds
		}
		interval := evaluate(at, intervalWindow)
		p := HistoryPoint{At: at, IntervalReliability: interval.Reliability, Reliability: rolling.Reliability, BudgetRemainingFraction: rolling.BudgetRemainingFraction, BurnRate: rolling.BurnRate, Coverage: rolling.Coverage, IntervalCoverage: interval.Coverage}
		p.BelowTarget = interval.Reliability != nil && interval.Coverage == 1 && *interval.Reliability < s.Target
		p.TargetViolated = rolling.BudgetRemainingFraction != nil && rolling.Coverage == 1 && *rolling.BudgetRemainingFraction < 0
		if policy := s.Policy; policy != nil {
			short, long := evaluate(at, policy.ShortWindowSeconds), evaluate(at, policy.LongWindowSeconds)
			p.ShortCoverage = short.Coverage
			p.LongCoverage = long.Coverage
			p.ShortBurnRate = short.BurnRate
			p.LongBurnRate = long.BurnRate
			p.BurnThresholdBreached = short.BurnRate != nil && long.BurnRate != nil && short.Coverage >= policy.MinCoverage && long.Coverage >= policy.MinCoverage && short.Total >= policy.MinEvents && long.Total >= policy.MinEvents && *short.BurnRate >= policy.BurnRate && *long.BurnRate >= policy.BurnRate
		}
		out.Points = append(out.Points, p)
	}
	for _, kind := range []string{"interval_target", "slo_target", "burn_threshold"} {
		var span *ViolationSpan
		for _, p := range out.Points {
			active := p.BelowTarget
			switch kind {
			case "slo_target":
				active = p.TargetViolated
			case "burn_threshold":
				active = p.BurnThresholdBreached
			}
			if !active {
				if span != nil {
					out.Violations = append(out.Violations, *span)
					span = nil
				}
				continue
			}
			if span == nil {
				span = &ViolationSpan{Kind: kind, From: p.At, To: p.At}
			}
			span.To = p.At
			span.Buckets++
		}
		if span != nil {
			out.Violations = append(out.Violations, *span)
		}
	}
	return out
}
