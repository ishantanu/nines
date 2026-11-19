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
	// Concentration summarizes how failures spread across the cohorts.
	Concentration *CohortConcentration `json:"concentration,omitempty"`
	// Outliers lists cohorts that are significantly less reliable than their
	// peers. It is evidence for review, not an alert or a target breach.
	Outliers *CohortOutliers `json:"outliers,omitempty"`
}

// CohortOutliers compares each cohort with all other cohorts combined.
type CohortOutliers struct {
	State       string          `json:"state"` // available, incomplete or insufficient
	Threshold   float64         `json:"threshold"`
	Items       []CohortOutlier `json:"items"`
	Limitations []string        `json:"limitations"`
}

// CohortOutlier is one cohort reliably worse than its peers. ZScore is the
// two-proportion statistic; more negative means a larger shortfall.
type CohortOutlier struct {
	Labels          map[string]string `json:"labels"`
	Total           float64           `json:"total"`
	Reliability     float64           `json:"reliability"`
	PeerReliability float64           `json:"peerReliability"`
	ZScore          float64           `json:"zScore"`
}

// CohortContribution compares one cohort's share of failures with its share of
// traffic. Representation above 1 means it fails more than its traffic share.
type CohortContribution struct {
	Labels         map[string]string `json:"labels"`
	Failures       float64           `json:"failures"`
	Events         float64           `json:"events"`
	FailureShare   float64           `json:"failureShare"`
	EventShare     float64           `json:"eventShare"`
	Representation *float64          `json:"representation,omitempty"`
}

// CohortConcentration shows how concentrated failures are across the cohort
// labels, next to the one-number SLO. It is computed only from complete cohort
// evidence at one cutoff. States: available, no_failures, insufficient_failures
// (too few failures to read a pattern) and incomplete (a numerator is missing).
// Verdict is "concentrated" or "spread" only when State is available.
type CohortConcentration struct {
	State            string               `json:"state"`
	Verdict          string               `json:"verdict,omitempty"`
	Failures         float64              `json:"failures"`
	Events           float64              `json:"events"`
	TopShare         float64              `json:"topShare"`
	EffectiveCohorts float64              `json:"effectiveCohorts"`
	Entropy          float64              `json:"entropy"`
	Contributors     []CohortContribution `json:"contributors"`
	Limitations      []string             `json:"limitations"`
}
