package v1alpha1

import "time"

// IntentReviewPacket freezes authoring context alongside the recommendation.
// It describes what was reviewed, not current provider configuration or health.
type IntentReviewPacket struct {
	Exclusions         *IntentReviewExclusions       `json:"exclusions,omitempty"`
	CandidateDiscovery *IntentReviewCandidateReceipt `json:"candidateDiscovery,omitempty"`
	Goal               SLOIntent                     `json:"goal"`
	Confirmation       IntentBindingConfirmation     `json:"confirmation"`
	LegacyMeasurement  *MeasurementBinding           `json:"legacyMeasurement,omitempty"`
	Checks             []IntentPreparationCheck      `json:"checks"`
	Destination        IntentReviewDestination       `json:"destination"`
	Resources          []IntentReviewResource        `json:"resources"`
	Limitations        []string                      `json:"limitations"`
}

// IntentReviewDestination identifies the logical source only. Endpoint URLs,
// credentials, upstream tenant headers and activation claims are never included.
type IntentReviewDestination struct {
	Tenant       string `json:"tenant"`
	Project      string `json:"project"`
	Service      string `json:"service"`
	Source       string `json:"source"`
	SourceDigest string `json:"sourceDigest"`
	State        string `json:"state"`
}

type IntentReviewResource struct {
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Digest           string `json:"digest"`
}

// IntentReviewCandidateReceipt references the newest saved discovery, without
// inferring that a suggested candidate became the confirmed measurement.
// Invalid or stale-scope receipts omit their report rather than exporting it.
type IntentReviewCandidateReceipt struct {
	State          string                    `json:"state"`
	RunID          string                    `json:"runId"`
	RunInputDigest string                    `json:"runInputDigest"`
	StepDigest     string                    `json:"stepDigest"`
	IntentDigest   string                    `json:"intentDigest,omitempty"`
	At             time.Time                 `json:"at"`
	Report         *IntentCandidateDiscovery `json:"report,omitempty"`
}

// IntentReviewExclusions ties observation-boundary notes to the same exact
// human confirmation as the measurement. No provider filter is inferred.
type IntentReviewExclusions struct {
	State        string                    `json:"state"`
	Definition   IntentExclusions          `json:"definition"`
	IntentDigest string                    `json:"intentDigest"`
	Confirmation IntentBindingConfirmation `json:"confirmation"`
}
