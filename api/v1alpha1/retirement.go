package v1alpha1

import "time"

// SLORetirement is server-owned state, never part of an authored SLO manifest.
// Credentials and provider response bodies must not be persisted here.
type SLORetirement struct {
	ResourceUID      string            `json:"resourceUID,omitempty"`
	EvaluationAt     time.Time         `json:"evaluationAt"`
	ArchivedAt       time.Time         `json:"archivedAt"`
	OriginalRevision int64             `json:"originalRevision"`
	RouteDigest      string            `json:"routeDigest,omitempty"`
	RuleDigest       string            `json:"ruleDigest,omitempty"`
	Group            string            `json:"group,omitempty"`
	Cleanup          string            `json:"cleanup"`
	Attempts         int               `json:"attempts"`
	LastAttemptAt    *time.Time        `json:"lastAttemptAt,omitempty"`
	NextAttemptAt    *time.Time        `json:"nextAttemptAt,omitempty"`
	Message          string            `json:"message"`
	Audit            []RetirementEvent `json:"audit"`
}
type RetirementEvent struct {
	ID       string    `json:"id"`
	Action   string    `json:"action"`
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Revision int64     `json:"revision"`
	Digest   string    `json:"digest"`
}
type RetirementReference struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Revision int64  `json:"revision"`
	Required bool   `json:"required"`
}
type RetirementPlan struct {
	ResourceUID      string                `json:"resourceUID,omitempty"`
	APIVersion       string                `json:"apiVersion"`
	Tenant           string                `json:"tenant"`
	SLO              string                `json:"slo"`
	Action           string                `json:"action"`
	ExpectedRevision int64                 `json:"expectedRevision"`
	Digest           string                `json:"digest"`
	References       []RetirementReference `json:"references"`
	Blockers         []string              `json:"blockers"`
	RouteDigest      string                `json:"routeDigest,omitempty"`
	RuleDigest       string                `json:"ruleDigest,omitempty"`
	Group            string                `json:"group,omitempty"`
	Retirement       *SLORetirement        `json:"retirement,omitempty"`
}
type RetirementInput struct {
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Digest           string `json:"digest"`
	// ConfirmName is required for irreversible deletion.
	ConfirmName string `json:"confirmName,omitempty"`
}
type RetirementReceipt struct {
	APIVersion string        `json:"apiVersion"`
	Tenant     string        `json:"tenant"`
	SLO        string        `json:"slo"`
	Revision   int64         `json:"revision"`
	DeletedAt  *time.Time    `json:"deletedAt,omitempty"`
	Retirement SLORetirement `json:"retirement"`
}
