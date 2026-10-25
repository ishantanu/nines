package v1alpha1

import "time"

// ErrorBudgetPolicy is an SLO-owned operational response policy. It grants no
// deployment permission and never changes targets or external release systems.
type ErrorBudgetPolicy struct {
	Owner       string  `json:"owner" yaml:"owner"`
	RunbookURL  string  `json:"runbookURL" yaml:"runbookURL"`
	ReviewBelow float64 `json:"reviewBelow" yaml:"reviewBelow"`
	OnExhausted string  `json:"onExhausted,omitempty" yaml:"onExhausted,omitempty"`
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
}
