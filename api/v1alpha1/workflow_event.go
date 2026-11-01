package v1alpha1

import "time"

// WorkflowEvent is a minimal notification, not an authorization to act.
type WorkflowEvent struct {
	APIVersion       string    `json:"apiVersion"`
	ID               string    `json:"id"`
	Tenant           string    `json:"tenant"`
	Conversation     string    `json:"conversation"`
	Action           string    `json:"action"`
	Outcome          string    `json:"outcome"`
	At               time.Time `json:"at"`
	Version          int64     `json:"version"`
	ChangeSetDigest  string    `json:"changeSetDigest,omitempty"`
	ObservationState string    `json:"observationState,omitempty"`
}
