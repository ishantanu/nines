package v1alpha1

import "time"

// IntegrationStage describes observed evidence, not a deployment promise.
type IntegrationStage struct {
	Code       string     `json:"code,omitempty"`
	NextAction string     `json:"nextAction,omitempty"`
	State      string     `json:"state"`
	Message    string     `json:"message"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
}

type IntegrationStatus struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Tenant     string            `json:"tenant"`
	SLOID      string            `json:"sloId"`
	Revision   int64             `json:"revision"`
	CheckedAt  time.Time         `json:"checkedAt"`
	Definition IntegrationStage  `json:"definition"`
	Collection IntegrationStage  `json:"collection"`
	Deployment IntegrationStage  `json:"deployment"`
	Evaluation IntegrationStage  `json:"evaluation"`
	RuleOutput *IntegrationStage `json:"ruleOutput,omitempty"`
}
