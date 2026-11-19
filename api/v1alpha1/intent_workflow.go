package v1alpha1

import "time"

// IntentPreparationCheck describes saved preparation evidence, not permission
// to approve/apply and not a fresh provider or model assessment.
type IntentPreparationCheck struct {
	Code    string `json:"code"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// IntentWorkflowStatus is a resumable workflow cursor, not authorization to act.
// Mutation requests must still supply the reviewed version, digest and consent.
type IntentWorkflowStatus struct {
	PreparationChecks          []IntentPreparationCheck `json:"preparationChecks,omitempty"`
	APIVersion                 string                   `json:"apiVersion"`
	Tenant                     string                   `json:"tenant"`
	Conversation               string                   `json:"conversation"`
	Version                    int64                    `json:"version"`
	State                      string                   `json:"state"`
	Phase                      string                   `json:"phase"`
	NextAction                 string                   `json:"nextAction"`
	HumanRequired              bool                     `json:"humanRequired"`
	IndependentApplierRequired bool                     `json:"independentApplierRequired"`
	ChangeSetDigest            string                   `json:"changeSetDigest,omitempty"`
	RollbackDigest             string                   `json:"rollbackDigest,omitempty"`
	IntentDigest               string                   `json:"intentDigest,omitempty"`
	Outcome                    *IntentOutcome           `json:"outcome,omitempty"`
	RetryAt                    *time.Time               `json:"retryAt,omitempty"`
	Message                    string                   `json:"message"`
}
