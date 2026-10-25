package v1alpha1

import (
	"encoding/json"
	"time"
)

// IntentRecommendation is an evidence summary for review, not hidden model reasoning.
type IntentRecommendation struct {
	BudgetDecision    *ErrorBudgetDecision   `json:"budgetDecision,omitempty"`
	MeasurementDigest string                 `json:"measurementDigest,omitempty"`
	RequestedBy       InvestigationActor     `json:"requestedBy"`
	RollbackStatus    string                 `json:"rollbackStatus"`
	CompilerVersion   string                 `json:"compilerVersion"`
	Summary           string                 `json:"summary"`
	Explanation       string                 `json:"explanation,omitempty"`
	Basis             []string               `json:"basis"`
	Limitations       []string               `json:"limitations"`
	RunID             string                 `json:"runId,omitempty"`
	Mode              string                 `json:"mode"`
	ModelRoute        *ReviewModelRoute      `json:"modelRoute,omitempty"`
	DraftRoute        *ReviewModelRoute      `json:"draftRoute,omitempty"`
	Skill             *ReviewSkillRef        `json:"skill,omitempty"`
	Binding           BindingReference       `json:"binding"`
	SourceDigest      string                 `json:"sourceDigest"`
	EvidenceDigest    string                 `json:"evidenceDigest"`
	ObservedFrom      time.Time              `json:"observedFrom"`
	ObservedTo        time.Time              `json:"observedTo"`
	Fields            []IntentFieldChange    `json:"fields"`
	Backtest          *IntentBacktestSummary `json:"backtest,omitempty"`
}
type IntentFieldChange struct {
	Resource string          `json:"resource"`
	Field    string          `json:"field"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
	Impact   string          `json:"impact"`
}
type IntentBacktestSummary struct {
	From              time.Time `json:"from"`
	To                time.Time `json:"to"`
	ResolutionSeconds int64     `json:"resolutionSeconds"`
	Complete          int       `json:"complete"`
	Unknown           int       `json:"unknown"`
	NoTraffic         int       `json:"noTraffic"`
	TargetViolations  int       `json:"targetViolations"`
	BurnBreaches      int       `json:"burnBreaches"`
	Digest            string    `json:"digest"`
}
type IntentAuditEvent struct {
	EvidenceDigest  string             `json:"evidenceDigest,omitempty"`
	Sequence        int64              `json:"sequence"`
	At              time.Time          `json:"at"`
	Actor           InvestigationActor `json:"actor"`
	Action          string             `json:"action"`
	Outcome         string             `json:"outcome"`
	Reason          string             `json:"reason"`
	ChangeSetDigest string             `json:"changeSetDigest,omitempty"`
	Version         int64              `json:"version"`
	PreviousHash    string             `json:"previousHash,omitempty"`
	Hash            string             `json:"hash,omitempty"`
}
type IntentAuditLog struct {
	NextAfter    int64              `json:"nextAfter,omitempty"`
	Through      int64              `json:"through"`
	ChangeSets   []IntentChangeSet  `json:"changeSets"`
	APIVersion   string             `json:"apiVersion"`
	Tenant       string             `json:"tenant"`
	Conversation string             `json:"conversation"`
	Integrity    string             `json:"integrity"`
	Events       []IntentAuditEvent `json:"events"`
}
type IntentRollback struct {
	ApprovalDigest string                   `json:"approvalDigest,omitempty"`
	SafetyCheck    *IntentSafetyCheck       `json:"safetyCheck,omitempty"`
	Outcome        *IntentOutcome           `json:"outcome,omitempty"`
	ID             string                   `json:"id"`
	Digest         string                   `json:"digest"`
	OriginalDigest string                   `json:"originalDigest"`
	State          string                   `json:"state"`
	Reason         string                   `json:"reason"`
	CreatedAt      time.Time                `json:"createdAt"`
	CreatedBy      InvestigationActor       `json:"createdBy"`
	Changes        []IntentDefinitionChange `json:"changes"`
	Blockers       []string                 `json:"blockers,omitempty"`
	Expected       []IntentAppliedResource  `json:"expected"`
	ApprovedBy     *InvestigationActor      `json:"approvedBy,omitempty"`
	ApprovedAt     *time.Time               `json:"approvedAt,omitempty"`
	ApprovalReason string                   `json:"approvalReason,omitempty"`
	AppliedBy      *InvestigationActor      `json:"appliedBy,omitempty"`
	AppliedAt      *time.Time               `json:"appliedAt,omitempty"`
	Applied        []IntentAppliedResource  `json:"applied,omitempty"`
}

// IntentOperationEvidence is committed with the resource transaction receipt.
type IntentOperationEvidence struct {
	SafetyCheck          *IntentSafetyCheck     `json:"safetyCheck,omitempty"`
	SafetyException      *IntentSafetyException `json:"safetyException,omitempty"`
	Operation            string                 `json:"operation"`
	ChangeSetDigest      string                 `json:"changeSetDigest"`
	ApprovedBy           InvestigationActor     `json:"approvedBy"`
	ApprovedAt           time.Time              `json:"approvedAt"`
	Reason               string                 `json:"reason"`
	RecommendationDigest string                 `json:"recommendationDigest,omitempty"`
}
