package v1alpha1

import "time"

// IntentContextInput pins discovery scope; it never selects a credential or endpoint.
type IntentContextInput struct {
	Service         string `json:"service"`
	Source          string `json:"source"`
	Project         string `json:"project"`
	Journey         string `json:"journey,omitempty"`
	JourneyRevision int64  `json:"journeyRevision,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	Cursor          string `json:"cursor,omitempty"`
}

type IntentSourceContext struct {
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	Configured          bool   `json:"configured"`
	MetricNameDiscovery bool   `json:"metricNameDiscovery"`
}

type IntentJourneyContext struct {
	Resource   ResourceSummary `json:"resource"`
	StepCount  int             `json:"stepCount"`
	LinkedSLOs int             `json:"linkedSLOs"`
}

// IntentContext is a read of saved evidence, not telemetry verification or approval.
type IntentContext struct {
	APIVersion  string                `json:"apiVersion"`
	Tenant      string                `json:"tenant"`
	ObservedAt  time.Time             `json:"observedAt"`
	TextTrust   string                `json:"textTrust"`
	Scope       IntentContextInput    `json:"scope"`
	Source      IntentSourceContext   `json:"source"`
	Journey     *IntentJourneyContext `json:"journey,omitempty"`
	Objectives  []SLOContext          `json:"objectives"`
	NextCursor  string                `json:"nextCursor,omitempty"`
	Warnings    []string              `json:"warnings"`
	NextActions []string              `json:"nextActions"`
}
