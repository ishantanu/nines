package v1alpha1

import "time"

// IntentUsage summarizes how the intent workflow was used in one tenant. It is
// computed from stored conversations and contains aggregates only: no actor,
// goal text, measurement or telemetry value appears, and nothing leaves the
// server. It describes workflow progress, not whether any SLO is healthy.
type IntentUsage struct {
	APIVersion string    `json:"apiVersion"`
	Tenant     string    `json:"tenant"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	WindowDays int       `json:"windowDays"`
	// Inspected is the number of conversations in the window that were read.
	Inspected int `json:"inspected"`
	// Truncated is true when the inspection cap stopped the scan early.
	Truncated bool `json:"truncated,omitempty"`
	// Counts are conversations that reached each stage at least once.
	Started  int `json:"started"`
	Proposed int `json:"proposed"`
	Approved int `json:"approved"`
	Applied  int `json:"applied"`
	Rejected int `json:"rejected"`
	// AppliedFirstProposal counts applied conversations whose first proposal
	// was the one applied, with no re-proposal after edits.
	AppliedFirstProposal int `json:"appliedFirstProposal"`
	RolledBack           int `json:"rolledBack"`
	// BacktestAttached counts conversations whose proposal carried a backtest.
	BacktestAttached int `json:"backtestAttached"`
	GuidedRuns       int `json:"guidedRuns"`
	ModelRuns        int `json:"modelRuns"`
	// Medians are nil until at least one conversation reaches the stage.
	MedianSecondsToProposal *int64   `json:"medianSecondsToProposal,omitempty"`
	MedianSecondsToApplied  *int64   `json:"medianSecondsToApplied,omitempty"`
	Limitations             []string `json:"limitations"`
}
