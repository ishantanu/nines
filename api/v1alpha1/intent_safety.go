package v1alpha1

import "time"

// IntentSafetyException is an explicit, short-lived owner approval for corrective
// work. It never bypasses authorization, source identity or revision fences.
type IntentSafetyException struct {
	Reason     string             `json:"reason"`
	ApprovedBy InvestigationActor `json:"approvedBy"`
	ApprovedAt time.Time          `json:"approvedAt"`
	ExpiresAt  time.Time          `json:"expiresAt"`
}
type IntentSafetyCheck struct {
	BurnState         string                 `json:"burnState,omitempty"`
	State             string                 `json:"state"`
	Message           string                 `json:"message"`
	CheckedAt         time.Time              `json:"checkedAt"`
	MeasurementDigest string                 `json:"measurementDigest,omitempty"`
	WindowSeconds     int64                  `json:"windowSeconds,omitempty"`
	Quality           *MeasurementQuality    `json:"quality,omitempty"`
	Budget            *ErrorBudgetDecision   `json:"budget,omitempty"`
	Exception         *IntentSafetyException `json:"exception,omitempty"`
}

// IntentObservationPolicy is frozen in the approved change-set digest.
type IntentObservationPolicy struct {
	WarmupSeconds   int64 `json:"warmupSeconds"`
	ObserveSeconds  int64 `json:"observeSeconds"`
	DeadlineSeconds int64 `json:"deadlineSeconds"`
}
type IntentOutcome struct {
	BurnState           string               `json:"burnState,omitempty"`
	APIVersion          string               `json:"apiVersion"`
	Tenant              string               `json:"tenant"`
	Conversation        string               `json:"conversation"`
	ChangeSetDigest     string               `json:"changeSetDigest"`
	Operation           string               `json:"operation"`
	OperationDigest     string               `json:"operationDigest"`
	SLOID               string               `json:"sloId"`
	Revision            int64                `json:"revision"`
	DefinitionDigest    string               `json:"definitionDigest"`
	SourceDigest        string               `json:"sourceDigest"`
	State               string               `json:"state"`
	Code                string               `json:"code"`
	Message             string               `json:"message"`
	StartedAt           time.Time            `json:"startedAt"`
	CheckedAt           time.Time            `json:"checkedAt"`
	WarmupUntil         time.Time            `json:"warmupUntil"`
	DeadlineAt          time.Time            `json:"deadlineAt"`
	HealthySince        *time.Time           `json:"healthySince,omitempty"`
	NextCheckAt         *time.Time           `json:"nextCheckAt,omitempty"`
	Quality             *MeasurementQuality  `json:"quality,omitempty"`
	Budget              *ErrorBudgetDecision `json:"budget,omitempty"`
	Deployment          *IntegrationStage    `json:"deployment,omitempty"`
	Evaluation          *IntegrationStage    `json:"evaluation,omitempty"`
	RuleOutput          *IntegrationStage    `json:"ruleOutput,omitempty"`
	Reliability         *float64             `json:"reliability,omitempty"`
	BaselineReliability *float64             `json:"baselineReliability,omitempty"`
	Comparison          string               `json:"comparison"`
	RollbackStatus      string               `json:"rollbackStatus"`
	Limitations         []string             `json:"limitations"`
}
