package httpapi

import (
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// Request and response bodies of stable routes that are not public API
// types. They are named so the OpenAPI contract is generated from, and
// checked against, the exact shapes the handlers encode.

// IngestSamples is the body of POST /api/slos/{id}/samples.
type IngestSamples struct {
	Samples []model.Point `json:"samples"`
}

// IngestResult reports how many samples were accepted.
type IngestResult struct {
	Accepted int `json:"accepted"`
}

// CollectRequest asks for a bounded collection: recent, or from and to.
type CollectRequest struct {
	Recent bool      `json:"recent"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

// CollectResult reports how many intervals were collected.
type CollectResult struct {
	Collected int `json:"collected"`
}

// CreatedSLO names the SLO a create made.
type CreatedSLO struct {
	ID string `json:"id"`
}

// RuleChange applies or withdraws rules at the digest that was reviewed.
type RuleChange struct {
	ExpectedDigest string `json:"expectedDigest"`
}

// RuleAutomationChange pauses or resumes automatic rule deployment.
type RuleAutomationChange struct {
	ExpectedRevision int64 `json:"expectedRevision"`
	Paused           *bool `json:"paused"`
}

// SLORevisionList is a page of an SLO's archived definition revisions.
type SLORevisionList struct {
	APIVersion         string           `json:"apiVersion"`
	SLOID              string           `json:"sloId"`
	Revisions          []v1.SLORevision `json:"revisions"`
	NextBeforeRevision int64            `json:"nextBeforeRevision"`
}

// TopologyResource is a measurement binding or dependency map revision.
type TopologyResource struct {
	APIVersion string                 `json:"apiVersion"`
	Tenant     string                 `json:"tenant"`
	Resource   model.TopologyRevision `json:"resource"`
}
