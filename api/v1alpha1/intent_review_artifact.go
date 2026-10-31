package v1alpha1

import "time"

// IntentReviewArtifact is an auditable snapshot for external review workflows.
// Its contents may include sensitive definitions, names and human review notes.
type IntentReviewArtifact struct {
	APIVersion     string               `json:"apiVersion"`
	Tenant         string               `json:"tenant"`
	Conversation   string               `json:"conversation"`
	Version        int64                `json:"version"`
	CapturedAt     time.Time            `json:"capturedAt"`
	Status         IntentWorkflowStatus `json:"status"`
	ChangeSet      IntentChangeSet      `json:"changeSet"`
	AuditIntegrity string               `json:"auditIntegrity"`
	AuditThrough   int64                `json:"auditThrough"`
	Audit          []IntentAuditEvent   `json:"audit"`
	Limitations    []string             `json:"limitations"`
}
