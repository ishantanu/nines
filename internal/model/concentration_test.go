package model

import (
	"math"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func cohort(region string, good, total float64) v1.CohortResult {
	g := good
	return v1.CohortResult{Labels: map[string]string{"region": region}, Good: &g, Total: total}
}

func TestConcentrationFindsAnOutageHiddenInAGoodAggregate(t *testing.T) {
	// 100,000 events, 99.92% overall (80 failures): passes a three-nines target,
	// yet 70 of the 80 failures come from one small cohort.
	items := []v1.CohortResult{cohort("eu", 49990, 50000), cohort("us", 49940, 49950), cohort("ap", 0, 50)}
	// eu 10 failures, us 10 failures, ap 50 failures out of 50 events.
	c := ConcentrateFailures(items)
	if c.State != "available" || c.Failures != 70 || c.Events != 100000 {
		t.Fatalf("%+v", c)
	}
	top := c.Contributors[0]
	if top.Labels["region"] != "ap" || math.Abs(top.FailureShare-50.0/70) > 1e-12 {
		t.Fatalf("top contributor: %+v", top)
	}
	if top.Representation == nil || *top.Representation < 1000 {
		t.Fatalf("a cohort with 0.05%% of traffic and 71%% of failures is heavily over-represented: %+v", top.Representation)
	}
	if c.Verdict != "concentrated" || c.TopShare < 0.7 {
		t.Fatalf("verdict %q top %v", c.Verdict, c.TopShare)
	}
	if c.EffectiveCohorts < 1 || c.EffectiveCohorts > 3 {
		t.Fatalf("effective cohorts %v", c.EffectiveCohorts)
	}
}

func TestConcentrationSpreadEvenly(t *testing.T) {
	items := []v1.CohortResult{cohort("a", 990, 1000), cohort("b", 990, 1000), cohort("c", 990, 1000), cohort("d", 990, 1000)}
	c := ConcentrateFailures(items)
	if c.State != "available" || c.Failures != 40 || c.Verdict != "spread" {
		t.Fatalf("%+v", c)
	}
	if math.Abs(c.EffectiveCohorts-4) > 1e-9 || math.Abs(c.Entropy-1) > 1e-9 {
		t.Fatalf("even spread must give 4 effective cohorts and entropy 1: %v %v", c.EffectiveCohorts, c.Entropy)
	}
}

func TestConcentrationNeverGuessesFromMissingOrThinData(t *testing.T) {
	missing := cohort("a", 0, 100)
	missing.Good = nil
	if c := ConcentrateFailures([]v1.CohortResult{cohort("b", 90, 100), missing}); c.State != "incomplete" || len(c.Contributors) != 0 {
		t.Fatalf("a missing numerator must not be guessed: %+v", c)
	}
	if c := ConcentrateFailures([]v1.CohortResult{cohort("a", 100, 100), cohort("b", 200, 200)}); c.State != "no_failures" {
		t.Fatalf("%+v", c)
	}
	if c := ConcentrateFailures([]v1.CohortResult{cohort("a", 95, 100), cohort("b", 100, 100)}); c.State != "insufficient_failures" || c.Verdict != "" {
		t.Fatalf("five failures are too few to read a pattern: %+v", c)
	}
	if c := ConcentrateFailures(nil); c.State != "incomplete" {
		t.Fatalf("%+v", c)
	}
}

func TestConcentrationOrderingIsDeterministicAndBounded(t *testing.T) {
	var items []v1.CohortResult
	for i := 0; i < 15; i++ {
		items = append(items, cohort(string(rune('a'+i)), 900, 1000)) // equal failures
	}
	first := ConcentrateFailures(items)
	reversed := make([]v1.CohortResult, len(items))
	for i, it := range items {
		reversed[len(items)-1-i] = it
	}
	second := ConcentrateFailures(reversed)
	if len(first.Contributors) != MaxContributors {
		t.Fatalf("contributors must be bounded: %d", len(first.Contributors))
	}
	for i := range first.Contributors {
		if first.Contributors[i].Labels["region"] != second.Contributors[i].Labels["region"] {
			t.Fatal("ordering depends on input order")
		}
	}
}
