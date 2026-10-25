package v1alpha1

import "time"

// MeasurementQualityPolicy controls evidence requirements, not SLI arithmetic.
type MeasurementQualityPolicy struct {
	MinEvents           float64  `json:"minEvents,omitempty" yaml:"minEvents,omitempty"`
	MaxSampleAgeSeconds int64    `json:"maxSampleAgeSeconds,omitempty" yaml:"maxSampleAgeSeconds,omitempty"`
	CohortLabels        []string `json:"cohortLabels,omitempty" yaml:"cohortLabels,omitempty"`
}
type QualityCheck struct {
	Code    string `json:"code"`
	State   string `json:"state"`
	Message string `json:"message"`
}
type MeasurementQuality struct {
	CollectedSourceDigest string         `json:"collectedSourceDigest,omitempty"`
	SourceFrom            *time.Time     `json:"sourceFrom,omitempty"`
	SourceTo              *time.Time     `json:"sourceTo,omitempty"`
	APIVersion            string         `json:"apiVersion"`
	Tenant                string         `json:"tenant"`
	SLOID                 string         `json:"sloId"`
	Revision              int64          `json:"revision"`
	DefinitionDigest      string         `json:"definitionDigest"`
	SourceDigest          string         `json:"sourceDigest"`
	ObservedAt            time.Time      `json:"observedAt"`
	EvaluatedAt           time.Time      `json:"evaluatedAt"`
	State                 string         `json:"state"`
	Freshness             Freshness      `json:"freshness"`
	Good                  float64        `json:"good"`
	Total                 float64        `json:"total"`
	Coverage              float64        `json:"coverage"`
	MinimumEvents         float64        `json:"minimumEvents"`
	ObservationPoint      string         `json:"observationPoint,omitempty"`
	Checks                []QualityCheck `json:"checks"`
	Limitations           []string       `json:"limitations"`
	Cohorts               *CohortReport  `json:"cohorts,omitempty"`
}

// CohortEvidence is a provider-independent boundary. Missing good counts remain
// absent; adapters must never fabricate counts for absent series.
type CohortEvidence struct {
	At            time.Time     `json:"at"`
	WindowSeconds int64         `json:"windowSeconds"`
	Counts        []CohortCount `json:"counts"`
}
type CohortCount struct {
	Labels map[string]string `json:"labels"`
	Good   *float64          `json:"good"`
	Total  float64           `json:"total"`
}
type CohortResult struct {
	Labels      map[string]string `json:"labels"`
	Good        *float64          `json:"good"`
	Total       float64           `json:"total"`
	Reliability *float64          `json:"reliability"`
	BurnRate    *float64          `json:"burnRate"`
	State       string            `json:"state"`
}
type CohortReport struct {
	State            string         `json:"state"`
	Message          string         `json:"message"`
	At               time.Time      `json:"at"`
	WindowSeconds    int64          `json:"windowSeconds"`
	Labels           []string       `json:"labels"`
	Items            []CohortResult `json:"items"`
	MatchesAggregate bool           `json:"matchesAggregate"`
}
