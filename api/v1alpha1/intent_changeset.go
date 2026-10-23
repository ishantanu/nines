package v1alpha1

import (
	"encoding/json"
	"time"
)

type IntentDefinitionChange struct {
	Kind             string          `json:"kind"`
	Name             string          `json:"name"`
	Action           string          `json:"action"`
	ExpectedRevision int64           `json:"expectedRevision"`
	Digest           string          `json:"digest"`
	Manifest         json.RawMessage `json:"manifest"`
	Previous         json.RawMessage `json:"previous,omitempty"`
}
type IntentAppliedResource struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}
type IntentChangeSet struct {
	RollbackHistory []IntentRollback         `json:"rollbackHistory,omitempty"`
	Recommendation  *IntentRecommendation    `json:"recommendation,omitempty"`
	ApprovalReason  string                   `json:"approvalReason,omitempty"`
	RejectedBy      *InvestigationActor      `json:"rejectedBy,omitempty"`
	RejectedAt      *time.Time               `json:"rejectedAt,omitempty"`
	RejectionReason string                   `json:"rejectionReason,omitempty"`
	Rollback        *IntentRollback          `json:"rollback,omitempty"`
	SourceDigest    string                   `json:"sourceDigest"`
	AppliedBy       *InvestigationActor      `json:"appliedBy,omitempty"`
	ID              string                   `json:"id"`
	Digest          string                   `json:"digest"`
	State           string                   `json:"state"`
	CreatedAt       time.Time                `json:"createdAt"`
	CreatedBy       InvestigationActor       `json:"createdBy"`
	IntentDigest    string                   `json:"intentDigest"`
	Binding         IntentSelection          `json:"binding"`
	Changes         []IntentDefinitionChange `json:"changes"`
	ApprovedBy      *InvestigationActor      `json:"approvedBy,omitempty"`
	ApprovedAt      *time.Time               `json:"approvedAt,omitempty"`
	AppliedAt       *time.Time               `json:"appliedAt,omitempty"`
	Applied         []IntentAppliedResource  `json:"applied,omitempty"`
}
type ReviewIntentChangeSet struct {
	Reason          string `json:"reason,omitempty"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Digest          string `json:"digest"`
	Confirm         bool   `json:"confirm"`
}

type IntentResourceVerification struct {
	IntentAppliedResource
	State           string `json:"state"`
	CurrentRevision int64  `json:"currentRevision"`
	CurrentDigest   string `json:"currentDigest"`
	Health          string `json:"health"`
}
type IntentChangeSetVerification struct {
	Operation      string                       `json:"operation,omitempty"`
	RollbackDigest string                       `json:"rollbackDigest,omitempty"`
	SourceState    string                       `json:"sourceState"`
	APIVersion     string                       `json:"apiVersion"`
	Tenant         string                       `json:"tenant"`
	Conversation   string                       `json:"conversation"`
	Digest         string                       `json:"digest"`
	Resources      []IntentResourceVerification `json:"resources"`
}

type IntentApplyReceipt struct {
	Evidence  *IntentOperationEvidence `json:"evidence,omitempty"`
	Resources []IntentAppliedResource  `json:"resources"`
	Actor     InvestigationActor       `json:"actor"`
	At        time.Time                `json:"at"`
}

type IntentKubernetesExport struct {
	APIVersion      string `json:"apiVersion"`
	Tenant          string `json:"tenant"`
	Namespace       string `json:"namespace"`
	ChangeSetDigest string `json:"changeSetDigest"`
	YAML            string `json:"yaml"`
}
