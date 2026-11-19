package v1alpha1

import "time"

// IntentActivation is an as-of receipt from explicit, bounded runtime reads.
// It neither persists an outcome nor authorizes publication or application.
type IntentActivation struct {
	APIVersion      string                       `json:"apiVersion"`
	Tenant          string                       `json:"tenant"`
	Conversation    string                       `json:"conversation"`
	Version         int64                        `json:"version"`
	ChangeSetDigest string                       `json:"changeSetDigest"`
	Operation       string                       `json:"operation"`
	OperationDigest string                       `json:"operationDigest"`
	SourceDigest    string                       `json:"sourceDigest"`
	CheckedAt       time.Time                    `json:"checkedAt"`
	RetryAt         *time.Time                   `json:"retryAt,omitempty"`
	State           string                       `json:"state"`
	Code            string                       `json:"code"`
	Message         string                       `json:"message"`
	NextAction      string                       `json:"nextAction"`
	Resources       []IntentResourceVerification `json:"resources"`
	Integration     *IntegrationStatus           `json:"integration,omitempty"`
	Measurement     *MeasurementQuality          `json:"measurement,omitempty"`
	Limitations     []string                     `json:"limitations"`
}
