package v1alpha1

import "time"

// AuditRecord is an independently retained decision, scoped to a tenant and conversation.
type AuditRecord struct {
	RecoveredFromConversation bool                    `json:"recoveredFromConversation,omitempty"`
	APIVersion                string                  `json:"apiVersion"`
	ID                        string                  `json:"id"`
	Tenant                    string                  `json:"tenant"`
	Conversation              string                  `json:"conversation"`
	Event                     IntentAuditEvent        `json:"event"`
	Resources                 []IntentAppliedResource `json:"resources"`
	Proposal                  *IntentChangeSet        `json:"proposal,omitempty"`
}
type AuditQuery struct {
	Conversation string    `json:"conversation,omitempty"`
	Actor        string    `json:"actor,omitempty"`
	Action       string    `json:"action,omitempty"`
	Outcome      string    `json:"outcome,omitempty"`
	Resource     string    `json:"resource,omitempty"`
	From         time.Time `json:"from,omitempty"`
	To           time.Time `json:"to,omitempty"`
	Cursor       string    `json:"cursor,omitempty"`
	Limit        int       `json:"limit,omitempty"`
}
type AuditPage struct {
	APIVersion string        `json:"apiVersion"`
	Tenant     string        `json:"tenant"`
	Items      []AuditRecord `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
	From       time.Time     `json:"from"`
	To         time.Time     `json:"to"`
}
