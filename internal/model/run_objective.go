package model

import (
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// RunObjectiveLimitations always accompany an evaluation.
var RunObjectiveLimitations = []string{
	"Only runs bound to the journey's current contract count. Runs recorded against another revision are excluded and shown separately.",
	"Passed runs are good; failed and violated runs are bad. Incomplete and inconclusive runs are unjudged and never counted as success.",
	"Run evidence is posted by the party running the journey. Nines does not run journeys or verify that runs were representative.",
	"Evaluation reads at most the latest 200 recorded runs, so a very busy window can hold more runs than were counted.",
}

// MaxEvaluatedRuns is the number of recent runs the store returns.
const MaxEvaluatedRuns = 200

func validateRunObjective(j Journey) error {
	o := j.Spec.RunObjective
	if o == nil {
		return nil
	}
	if j.Spec.Agentic == nil {
		return Invalid("runObjective requires an agentic contract, because only agentic journeys record runs")
	}
	if !Finite(o.Target) || o.Target <= 0 || o.Target >= 1 {
		return Invalid("runObjective.target must be above 0 and below 1")
	}
	if o.WindowSeconds < 3600 || o.WindowSeconds > 31*86400 {
		return Invalid("runObjective.windowSeconds must be between one hour and 31 days")
	}
	if o.MinRuns < 1 || o.MinRuns > 1000 {
		return Invalid("runObjective.minRuns must be between 1 and 1000")
	}
	return nil
}

// EvaluateRunObjective reads recorded runs against an objective. It is pure:
// the same runs and time always give the same result.
func EvaluateRunObjective(o v1.RunObjective, contractDigest string, runs []JourneyRun, now time.Time) v1.JourneyRunObjective {
	now = now.UTC()
	out := v1.JourneyRunObjective{State: "no_runs", Target: o.Target, WindowSeconds: o.WindowSeconds, MinRuns: o.MinRuns, To: now, From: now.Add(-time.Duration(o.WindowSeconds) * time.Second), Limitations: append([]string{}, RunObjectiveLimitations...)}
	out.Truncated = len(runs) >= MaxEvaluatedRuns
	for _, run := range runs {
		finished := run.Evidence.FinishedAt
		if !finished.After(out.From) || finished.After(now) {
			continue
		}
		if run.ContractDigest != contractDigest {
			out.OtherRevision++
			continue
		}
		switch run.Verdict {
		case "passed":
			out.Passed++
		case "failed":
			out.Failed++
		case "violated":
			out.Violated++
		default:
			out.Unjudged++
		}
	}
	out.Judged = out.Passed + out.Failed + out.Violated
	switch {
	case out.Judged == 0:
		out.State = "no_runs"
	case out.Judged < o.MinRuns:
		out.State = "insufficient_runs"
	default:
		out.State = "available"
		reliability := float64(out.Passed) / float64(out.Judged)
		burn := (1 - reliability) / (1 - o.Target)
		remaining := 1 - burn
		out.Reliability, out.BurnRate, out.BudgetRemainingFraction = &reliability, &burn, &remaining
	}
	return out
}
