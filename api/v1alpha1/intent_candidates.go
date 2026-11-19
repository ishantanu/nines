package v1alpha1

import "time"

// IntentCandidateInput requests read-only discovery; it is not an SLO definition.
// Population is an explicit equality filter, never inferred from the service name.
type IntentCandidateInput struct {
	Service    string            `json:"service" yaml:"service"`
	Source     string            `json:"source" yaml:"source"`
	Project    string            `json:"project" yaml:"project"`
	Population map[string]string `json:"population" yaml:"population"`
	Prefix     string            `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Metrics    []string          `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	Category   string            `json:"category,omitempty" yaml:"category,omitempty"`
	Outcome    string            `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	Threshold  string            `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	Limit      int               `json:"limit,omitempty" yaml:"limit,omitempty"`
}
type IntentRecipeReference struct {
	ID        string              `json:"id"`
	Revision  int                 `json:"revision"`
	Digest    string              `json:"digest"`
	Category  string              `json:"category"`
	SLI       string              `json:"sli"`
	Match     string              `json:"match"`
	Extension *ExtensionReference `json:"extension,omitempty"`
}
type IntentMeasurementCandidate struct {
	ID                       string                  `json:"id"`
	RequestedMetric          string                  `json:"requestedMetric"`
	Metric                   string                  `json:"metric"`
	State                    string                  `json:"state"`
	MetricType               string                  `json:"metricType"`
	Unit                     string                  `json:"unit"`
	From                     time.Time               `json:"from,omitempty"`
	To                       time.Time               `json:"to,omitempty"`
	ObservedSeries           int                     `json:"observedSeries"`
	LabelNames               []string                `json:"labelNames"`
	Truncated                bool                    `json:"truncated"`
	ThresholdSeconds         *float64                `json:"thresholdSeconds,omitempty"`
	MatchingBoundaryObserved bool                    `json:"matchingBoundaryObserved"`
	Recipes                  []IntentRecipeReference `json:"recipes"`
	Questions                []string                `json:"questions"`
	Warnings                 []string                `json:"warnings"`
	EvidenceDigest           string                  `json:"evidenceDigest"`
}
type IntentCandidateDiscovery struct {
	APIVersion                string                       `json:"apiVersion"`
	Tenant                    string                       `json:"tenant"`
	ObservedAt                time.Time                    `json:"observedAt"`
	TextTrust                 string                       `json:"textTrust"`
	Scope                     IntentCandidateInput         `json:"scope"`
	SourceDigest              string                       `json:"sourceDigest"`
	CatalogDigest             string                       `json:"catalogDigest"`
	State                     string                       `json:"state"`
	MetricNameDiscovery       bool                         `json:"metricNameDiscovery"`
	SeriesDiscovery           bool                         `json:"seriesDiscovery"`
	AdapterCalls              int                          `json:"adapterCalls"`
	MaxAdapterCalls           int                          `json:"maxAdapterCalls"`
	Names                     []string                     `json:"names"`
	Truncated                 bool                         `json:"truncated"`
	Candidates                []IntentMeasurementCandidate `json:"candidates"`
	Questions                 []string                     `json:"questions"`
	Warnings                  []string                     `json:"warnings"`
	HumanConfirmationRequired bool                         `json:"humanConfirmationRequired"`
}
