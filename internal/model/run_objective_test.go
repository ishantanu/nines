package model

import (
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

var objectiveNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

func run(verdict, digest string, hoursAgo float64) JourneyRun {
	return JourneyRun{ContractDigest: digest, Verdict: verdict, Evidence: RunEvidence{FinishedAt: objectiveNow.Add(-time.Duration(hoursAgo * float64(time.Hour)))}}
}

func TestRunObjectiveCountsOnlyJudgedRunsOnTheCurrentContract(t *testing.T) {
	o := v1.RunObjective{Target: 0.9, WindowSeconds: 24 * 3600, MinRuns: 5}
	runs := []JourneyRun{
		run("passed", "d2", 1), run("passed", "d2", 2), run("passed", "d2", 3), run("passed", "d2", 4),
		run("failed", "d2", 5), run("violated", "d2", 6),
		run("incomplete", "d2", 7), run("inconclusive", "d2", 8), // never counted as success
		run("passed", "d1", 9),  // another revision's contract
		run("passed", "d2", 30), // outside the window
	}
	got := EvaluateRunObjective(o, "d2", runs, objectiveNow)
	if got.State != "available" || got.Passed != 4 || got.Failed != 1 || got.Violated != 1 || got.Unjudged != 2 || got.OtherRevision != 1 || got.Judged != 6 {
		t.Fatalf("%+v", got)
	}
	if got.Reliability == nil || *got.Reliability != 4.0/6 {
		t.Fatalf("reliability: %v", got.Reliability)
	}
	// 1/3 of runs bad against a 10% allowance: a 3.33x burn, and the budget is overspent.
	if got.BurnRate == nil || *got.BurnRate < 3.33 || *got.BurnRate > 3.34 || *got.BudgetRemainingFraction >= 0 {
		t.Fatalf("burn %v budget %v", got.BurnRate, got.BudgetRemainingFraction)
	}
	if len(got.Limitations) == 0 {
		t.Fatal("limitations must accompany every result")
	}
}

func TestRunObjectiveNeverReadsThinOrEmptyEvidence(t *testing.T) {
	o := v1.RunObjective{Target: 0.9, WindowSeconds: 3600, MinRuns: 5}
	none := EvaluateRunObjective(o, "d", nil, objectiveNow)
	if none.State != "no_runs" || none.Reliability != nil {
		t.Fatalf("%+v", none)
	}
	onlyUnjudged := EvaluateRunObjective(o, "d", []JourneyRun{run("incomplete", "d", 0.1), run("inconclusive", "d", 0.2)}, objectiveNow)
	if onlyUnjudged.State != "no_runs" || onlyUnjudged.Unjudged != 2 || onlyUnjudged.Reliability != nil {
		t.Fatalf("unjudged runs must not read as success: %+v", onlyUnjudged)
	}
	thin := EvaluateRunObjective(o, "d", []JourneyRun{run("passed", "d", 0.1), run("passed", "d", 0.2)}, objectiveNow)
	if thin.State != "insufficient_runs" || thin.Reliability != nil || thin.Judged != 2 {
		t.Fatalf("two perfect runs are not enough to read: %+v", thin)
	}
	many := make([]JourneyRun, MaxEvaluatedRuns)
	for i := range many {
		many[i] = run("passed", "d", 0.01)
	}
	if got := EvaluateRunObjective(o, "d", many, objectiveNow); !got.Truncated {
		t.Fatal("a full run page may hide older runs and must say so")
	}
}

func TestRunObjectiveValidation(t *testing.T) {
	base := func() Journey {
		j := Journey(v1.CriticalUserJourney{})
		j.Spec.Agentic = &v1.AgenticContract{SuccessDefinition: "x"}
		j.Spec.RunObjective = &v1.RunObjective{Target: 0.95, WindowSeconds: 86400, MinRuns: 10}
		return j
	}
	if err := validateRunObjective(base()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Journey){
		"no agentic contract": func(j *Journey) { j.Spec.Agentic = nil },
		"target one":          func(j *Journey) { j.Spec.RunObjective.Target = 1 },
		"target zero":         func(j *Journey) { j.Spec.RunObjective.Target = 0 },
		"short window":        func(j *Journey) { j.Spec.RunObjective.WindowSeconds = 60 },
		"long window":         func(j *Journey) { j.Spec.RunObjective.WindowSeconds = 32 * 86400 },
		"no min runs":         func(j *Journey) { j.Spec.RunObjective.MinRuns = 0 },
		"huge min runs":       func(j *Journey) { j.Spec.RunObjective.MinRuns = 1001 },
	} {
		j := base()
		mutate(&j)
		if err := validateRunObjective(j); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
