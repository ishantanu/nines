package v1alpha1

import "time"

// RunObjective is an optional reliability objective for an agentic journey,
// measured from its recorded runs rather than from linked SLOs. Runs exist only
// for journeys with an agentic contract.
type RunObjective struct {
	// Target is the share of judged runs that must pass (above 0, below 1).
	Target float64 `json:"target" yaml:"target"`
	// WindowSeconds is how far back runs count, from one hour to 31 days.
	WindowSeconds int64 `json:"windowSeconds" yaml:"windowSeconds"`
	// MinRuns is the fewest judged runs needed before the objective is read.
	MinRuns int `json:"minRuns" yaml:"minRuns"`
}

// JourneyRunObjective is the evaluated result. Passed runs are good; failed and
// violated runs are bad; incomplete and inconclusive runs are unjudged and are
// never counted as success. Only runs bound to the journey's current contract
// count. States: available, no_runs, insufficient_runs.
type JourneyRunObjective struct {
	State                   string    `json:"state"`
	Target                  float64   `json:"target"`
	WindowSeconds           int64     `json:"windowSeconds"`
	MinRuns                 int       `json:"minRuns"`
	From                    time.Time `json:"from"`
	To                      time.Time `json:"to"`
	Passed                  int       `json:"passed"`
	Failed                  int       `json:"failed"`
	Violated                int       `json:"violated"`
	Unjudged                int       `json:"unjudged"`
	OtherRevision           int       `json:"otherRevision"`
	Judged                  int       `json:"judged"`
	Reliability             *float64  `json:"reliability"`
	BurnRate                *float64  `json:"burnRate"`
	BudgetRemainingFraction *float64  `json:"budgetRemainingFraction"`
	// Truncated is true when the run store returned its retention limit, so the
	// window may contain more runs than were evaluated.
	Truncated   bool     `json:"truncated"`
	Limitations []string `json:"limitations"`
}
