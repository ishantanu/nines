package model

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func outlierCohort(region string, good, total float64) v1.CohortResult {
	g := good
	return v1.CohortResult{Labels: map[string]string{"region": region}, Good: &g, Total: total}
}

func TestCohortOutliersFlagOnlySignificantShortfalls(t *testing.T) {
	got := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 9990, 10000), outlierCohort("b", 9985, 10000), outlierCohort("c", 9700, 10000)})
	if got.State != "available" || len(got.Items) != 1 || got.Items[0].Labels["region"] != "c" || got.Items[0].ZScore > -3 || got.Items[0].PeerReliability < .99 {
		t.Fatalf("%+v", got)
	}
	// Equal cohorts, and a cohort that is better than its peers, are not flagged.
	if g := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 9990, 10000), outlierCohort("b", 9990, 10000), outlierCohort("c", 9990, 10000)}); len(g.Items) != 0 {
		t.Fatalf("%+v", g)
	}
	better := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 9000, 10000), outlierCohort("b", 9000, 10000), outlierCohort("c", 9999, 10000)})
	for _, o := range better.Items {
		if o.Labels["region"] == "c" {
			t.Fatalf("a better cohort must not be flagged: %+v", better)
		}
	}
	// A small cohort is not compared, however bad it looks.
	if g := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 9990, 10000), outlierCohort("b", 9990, 10000), outlierCohort("tiny", 0, 20)}); len(g.Items) != 0 {
		t.Fatalf("%+v", g)
	}
	// Too few cohorts is insufficient, an unknown numerator is incomplete.
	if g := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 9990, 10000), outlierCohort("b", 5000, 10000)}); g.State != "insufficient" {
		t.Fatalf("%+v", g)
	}
	missing := outlierCohort("b", 0, 10000)
	missing.Good = nil
	if g := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 9990, 10000), missing, outlierCohort("c", 9990, 10000)}); g.State != "incomplete" || len(g.Items) != 0 {
		t.Fatalf("%+v", g)
	}
	// All-good traffic has no variance and must not divide by zero.
	if g := FindCohortOutliers([]v1.CohortResult{outlierCohort("a", 10000, 10000), outlierCohort("b", 10000, 10000), outlierCohort("c", 10000, 10000)}); len(g.Items) != 0 {
		t.Fatalf("%+v", g)
	}
}
