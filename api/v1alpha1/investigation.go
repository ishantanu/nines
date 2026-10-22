package v1alpha1

import (
	"encoding/json"
	"time"
)

// Investigation is operational state, independent of declarative SLO/CUJ schemas.
// The scope is immutable; evidence captures never revise resource definitions.
type InvestigationScope struct {
	Kind             string    `json:"kind"`
	ResourceID       string    `json:"resourceId"`
	ExpectedRevision int64     `json:"expectedRevision"`
	At               time.Time `json:"at"`
}
type InvestigationActor struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
}
type InvestigationEvidence struct {
	CapturedAt time.Time       `json:"capturedAt"`
	Digest     string          `json:"digest"`
	Context    json.RawMessage `json:"context"`
}
type InvestigationEntry struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	CreatedAt time.Time              `json:"createdAt"`
	Actor     InvestigationActor     `json:"actor"`
	Text      string                 `json:"text,omitempty"`
	Evidence  *InvestigationEvidence `json:"evidence,omitempty"`
}
type InvestigationSummary struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Version   int64              `json:"version"`
	Scope     InvestigationScope `json:"scope"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
	CreatedBy InvestigationActor `json:"createdBy"`
}
type Investigation struct {
	APIVersion string `json:"apiVersion"`
	Tenant     string `json:"tenant"`
	InvestigationSummary
	Entries []InvestigationEntry `json:"entries"`
}
type CreateInvestigation struct {
	ID    string             `json:"id"`
	Title string             `json:"title"`
	Scope InvestigationScope `json:"scope"`
}
type AppendInvestigationMessage struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Text            string `json:"text"`
}
type CaptureInvestigationEvidence struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
}
type InvestigationList struct {
	APIVersion string                 `json:"apiVersion"`
	Tenant     string                 `json:"tenant"`
	Items      []InvestigationSummary `json:"items"`
	NextCursor string                 `json:"nextCursor,omitempty"`
}
