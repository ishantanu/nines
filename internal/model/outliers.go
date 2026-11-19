package model

import (
	"math"
	"sort"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// Relative comparison asks a different question from a target: is one cohort
// doing significantly worse than the rest, even while the SLO is met? It uses a
// two-proportion z-test of each cohort against all other cohorts combined.
const (
	// OutlierZ is the shortfall, in standard errors, that is called an outlier.
	OutlierZ = 3.0
	// MinOutlierEvents is the fewest events a cohort and its peers each need.
	MinOutlierEvents = 100
	// MinOutlierCohorts is the fewest cohorts for a peer comparison to mean anything.
	MinOutlierCohorts = 3
	MaxOutliers       = 10
)

var OutlierLimitations = []string{
	"Each cohort is compared with all other cohorts combined at one cutoff. A significant gap shows a difference, not its cause.",
	"A z-score of -3 or lower is flagged. Checking many cohorts makes some chance flags likely, so treat a flag as a reason to look.",
	"Cohorts or peer groups with fewer than 100 events are not compared. Only declared cohort labels are examined.",
	"This is evidence for review. It is not an alert, does not measure the SLO target and changes no rule.",
}

// FindCohortOutliers is pure and deterministic. Any unknown numerator makes the
// result incomplete instead of guessing.
func FindCohortOutliers(items []v1.CohortResult) v1.CohortOutliers {
	out := v1.CohortOutliers{State: "insufficient", Threshold: -OutlierZ, Items: []v1.CohortOutlier{}, Limitations: append([]string{}, OutlierLimitations...)}
	var good, total float64
	for _, it := range items {
		if it.Good == nil {
			out.State = "incomplete"
			return out
		}
		good += *it.Good
		total += it.Total
	}
	if len(items) < MinOutlierCohorts {
		return out
	}
	out.State = "available"
	for _, it := range items {
		restTotal := total - it.Total
		if it.Total < MinOutlierEvents || restTotal < MinOutlierEvents {
			continue
		}
		p1 := *it.Good / it.Total
		p2 := (good - *it.Good) / restTotal
		pooled := good / total
		se := math.Sqrt(pooled * (1 - pooled) * (1/it.Total + 1/restTotal))
		if se == 0 || !Finite(se) {
			continue
		}
		z := (p1 - p2) / se
		if z <= -OutlierZ {
			out.Items = append(out.Items, v1.CohortOutlier{Labels: it.Labels, Total: it.Total, Reliability: p1, PeerReliability: p2, ZScore: z})
		}
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].ZScore < out.Items[j].ZScore })
	if len(out.Items) > MaxOutliers {
		out.Items = out.Items[:MaxOutliers]
	}
	return out
}
