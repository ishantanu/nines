package model

import (
	"math"
	"sort"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// Failure concentration compares where failures land with where traffic goes,
// across the cohort labels an SLO declares. A single SLO percentage hides this:
// a three-nines result can coexist with one cohort failing constantly.
const (
	// MinConcentrationFailures is the fewest failures needed to read a pattern.
	MinConcentrationFailures = 30
	// ConcentratedTopShare and ConcentratedRepresentation define "concentrated":
	// the top cohort holds at least this share of failures while failing at
	// least this many times its traffic share.
	ConcentratedTopShare       = 0.5
	ConcentratedRepresentation = 2.0
	// MaxContributors bounds the cohorts returned.
	MaxContributors = 10
)

// ConcentrationLimitations always accompany a result.
var ConcentrationLimitations = []string{
	"Computed from observed cohort counts at one cutoff. It shows where failures landed, not why.",
	"Effective cohorts is 1 divided by the sum of squared failure shares; 1 means one cohort holds every failure. Entropy runs from 0 (one cohort) to 1 (failures spread evenly).",
	"With fewer than 30 failures no pattern is reported, because small counts concentrate by chance.",
	"Only the declared cohort labels are examined. Failures concentrated in an undeclared dimension are not visible.",
}

// ConcentrateFailures derives concentration from cohort results. It is pure and
// deterministic. Any cohort with an unknown numerator makes the whole result
// incomplete rather than guessing its failures.
func ConcentrateFailures(items []v1.CohortResult) v1.CohortConcentration {
	out := v1.CohortConcentration{State: "incomplete", Contributors: []v1.CohortContribution{}, Limitations: append([]string{}, ConcentrationLimitations...)}
	var failures, events float64
	type row struct {
		labels           map[string]string
		failures, events float64
	}
	rows := make([]row, 0, len(items))
	for _, item := range items {
		if item.Good == nil || !Finite(item.Total) || item.Total < 0 {
			return out
		}
		f := math.Max(0, item.Total-*item.Good)
		rows = append(rows, row{item.Labels, f, item.Total})
		failures += f
		events += item.Total
	}
	out.Failures, out.Events = failures, events
	if len(rows) == 0 {
		return out
	}
	if failures == 0 {
		out.State = "no_failures"
		return out
	}
	if failures < MinConcentrationFailures {
		out.State = "insufficient_failures"
		return out
	}
	out.State = "available"
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].failures != rows[j].failures {
			return rows[i].failures > rows[j].failures
		}
		return DefinitionDigest(rows[i].labels) < DefinitionDigest(rows[j].labels)
	})
	hhi, entropy, withFailures := 0.0, 0.0, 0
	for _, r := range rows {
		if r.failures == 0 {
			continue
		}
		share := r.failures / failures
		hhi += share * share
		entropy -= share * math.Log(share)
		withFailures++
	}
	out.EffectiveCohorts = 1 / hhi
	if withFailures > 1 {
		out.Entropy = entropy / math.Log(float64(withFailures))
	}
	for i, r := range rows {
		if i >= MaxContributors {
			break
		}
		c := v1.CohortContribution{Labels: r.labels, Failures: r.failures, Events: r.events, FailureShare: r.failures / failures}
		if events > 0 {
			c.EventShare = r.events / events
		}
		if c.EventShare > 0 {
			rep := c.FailureShare / c.EventShare
			c.Representation = &rep
		}
		out.Contributors = append(out.Contributors, c)
	}
	top := out.Contributors[0]
	out.TopShare = top.FailureShare
	out.Verdict = "spread"
	if top.FailureShare >= ConcentratedTopShare && top.Representation != nil && *top.Representation >= ConcentratedRepresentation {
		out.Verdict = "concentrated"
	}
	return out
}
