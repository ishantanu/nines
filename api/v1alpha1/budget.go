package v1alpha1

import "time"

// ErrorBudgetPolicy is an SLO-owned operational response policy. It grants no
// deployment permission and never changes targets or external release systems.
type ErrorBudgetPolicy struct {
	Owner       string  `json:"owner" yaml:"owner"`
	RunbookURL  string  `json:"runbookURL" yaml:"runbookURL"`
	ReviewBelow float64 `json:"reviewBelow" yaml:"reviewBelow"`
	OnExhausted string  `json:"onExhausted,omitempty" yaml:"onExhausted,omitempty"`
	// Bands state, as the remaining budget falls, the response the owner has
	// agreed for each level. They communicate the response; they never change
	// the decision state or grant any permission.
	Bands []BudgetBand `json:"bands,omitempty" yaml:"bands,omitempty"`
	// PostmortemAbove is the share of the error budget a single incident may
	// consume before a postmortem is required, for example 0.2. At exactly the
	// threshold none is required. Zero turns the requirement off.
	PostmortemAbove float64 `json:"postmortemAbove,omitempty" yaml:"postmortemAbove,omitempty"`
}

// BudgetBand is one agreed response level, active when the remaining budget
// fraction is at or below Below.
type BudgetBand struct {
	Name     string  `json:"name" yaml:"name"`
	Below    float64 `json:"below" yaml:"below"`
	Response string  `json:"response" yaml:"response"`
}

// BudgetBandMatch is the most severe band that applies to the current budget.
type BudgetBandMatch struct {
	Name     string  `json:"name"`
	Below    float64 `json:"below"`
	Response string  `json:"response"`
}
type BudgetEvidence struct {
	Coverage      float64        `json:"coverage"`
	Total         float64        `json:"total"`
	MinimumEvents float64        `json:"minimumEvents"`
	Freshness     Freshness      `json:"freshness"`
	Checks        []QualityCheck `json:"checks"`
}
type ErrorBudgetDecision struct {
	Evidence                *BudgetEvidence `json:"evidence,omitempty"`
	APIVersion              string          `json:"apiVersion"`
	Tenant                  string          `json:"tenant"`
	SLOID                   string          `json:"sloId"`
	Revision                int64           `json:"revision"`
	DefinitionDigest        string          `json:"definitionDigest"`
	SourceDigest            string          `json:"sourceDigest"`
	PolicyDigest            string          `json:"policyDigest,omitempty"`
	ObservedAt              time.Time       `json:"observedAt"`
	EvaluatedAt             time.Time       `json:"evaluatedAt"`
	State                   string          `json:"state"`
	ReasonCodes             []string        `json:"reasonCodes"`
	Message                 string          `json:"message"`
	Owner                   string          `json:"owner,omitempty"`
	RunbookURL              string          `json:"runbookURL,omitempty"`
	BudgetRemainingFraction *float64        `json:"budgetRemainingFraction"`
	QualityState            string          `json:"qualityState"`
	Advisory                bool            `json:"advisory"`
	// Band is the agreed response level for the remaining budget, when bands are
	// configured and one applies. It never changes State.
	Band *BudgetBandMatch `json:"band,omitempty"`
	// RequiredPostmortems are recent incidents on this SLO whose postmortem
	// the policy requires (postmortemAbove) and that have none published yet.
	// They never change State.
	RequiredPostmortems []RequiredPostmortem `json:"requiredPostmortems,omitempty"`
}

// RequiredPostmortem is an incident that consumed more budget than the
// policy allows without a postmortem.
type RequiredPostmortem struct {
	IncidentID string  `json:"incidentId"`
	Title      string  `json:"title"`
	Consumed   float64 `json:"consumed"`
	Threshold  float64 `json:"threshold"`
}
