package v1alpha1

import "time"

// IntentEvidenceInput pins a complete measurement; digests are consistency
// checks, never proof of human approval or permission to apply a definition.
type IntentEvidenceInput struct {
	Plan              PlanIntentInput             `json:"plan" yaml:"plan"`
	IntentDigest      string                      `json:"intentDigest,omitempty" yaml:"intentDigest,omitempty"`
	MeasurementDigest string                      `json:"measurementDigest,omitempty" yaml:"measurementDigest,omitempty"`
	SourceDigest      string                      `json:"sourceDigest,omitempty" yaml:"sourceDigest,omitempty"`
	CatalogDigest     string                      `json:"catalogDigest,omitempty" yaml:"catalogDigest,omitempty"`
	ExpectedRevision  int64                       `json:"expectedRevision" yaml:"expectedRevision"`
	History           *IntentEvidenceRange        `json:"history,omitempty" yaml:"history,omitempty"`
	Alternatives      []IntentEvidenceAlternative `json:"alternatives,omitempty" yaml:"alternatives,omitempty"`
}
type IntentEvidenceRange struct {
	From              time.Time `json:"from" yaml:"from"`
	To                time.Time `json:"to" yaml:"to"`
	ResolutionSeconds int64     `json:"resolutionSeconds" yaml:"resolutionSeconds"`
}
type IntentEvidenceAlternative struct {
	Name     string            `json:"name" yaml:"name"`
	Target   *float64          `json:"target,omitempty" yaml:"target,omitempty"`
	Window   string            `json:"window,omitempty" yaml:"window,omitempty"`
	Policies *[]BurnRatePolicy `json:"policies,omitempty" yaml:"policies,omitempty"`
}
type IntentEvidenceCurrent struct {
	State         string         `json:"state"`
	At            time.Time      `json:"at"`
	From          time.Time      `json:"from"`
	Good          float64        `json:"good"`
	Total         float64        `json:"total"`
	Coverage      float64        `json:"coverage"`
	Reliability   *float64       `json:"reliability"`
	BurnRate      *float64       `json:"burnRate"`
	DataStatus    string         `json:"dataStatus"`
	Freshness     Freshness      `json:"freshness"`
	MinimumEvents float64        `json:"minimumEvents"`
	Checks        []QualityCheck `json:"checks"`
}
type IntentEvidenceHistory struct {
	From                   time.Time `json:"from"`
	To                     time.Time `json:"to"`
	ResolutionSeconds      int64     `json:"resolutionSeconds"`
	CompleteCutoffs        int       `json:"completeCutoffs"`
	UnknownCutoffs         int       `json:"unknownCutoffs"`
	NoTrafficCutoffs       int       `json:"noTrafficCutoffs"`
	TargetViolationCutoffs int       `json:"targetViolationCutoffs"`
	BurnThresholdCutoffs   int       `json:"burnThresholdCutoffs"`
}
type IntentPolicyTradeoff struct {
	MinCoverage              float64 `json:"minCoverage"`
	MinEvents                float64 `json:"minEvents"`
	Confidence               float64 `json:"confidence,omitempty"`
	Name                     string  `json:"name"`
	Severity                 string  `json:"severity"`
	ShortWindowSeconds       int64   `json:"shortWindowSeconds"`
	LongWindowSeconds        int64   `json:"longWindowSeconds"`
	ForSeconds               int64   `json:"forSeconds"`
	BurnRate                 float64 `json:"burnRate"`
	LongWindowBudgetFraction float64 `json:"longWindowBudgetFraction"`
}
type IntentEvidenceComparison struct {
	LifecycleAllowsAlerts      bool                   `json:"lifecycleAllowsAlerts"`
	Name                       string                 `json:"name"`
	State                      string                 `json:"state"`
	Target                     float64                `json:"target"`
	WindowSeconds              int64                  `json:"windowSeconds"`
	AllowedBadEventsPerMillion float64                `json:"allowedBadEventsPerMillion"`
	Policies                   []IntentPolicyTradeoff `json:"policies"`
	Current                    *IntentEvidenceCurrent `json:"current,omitempty"`
	History                    *IntentEvidenceHistory `json:"history,omitempty"`
	HistoryState               string                 `json:"historyState"`
	Gaps                       []string               `json:"gaps"`
}
type IntentEvidence struct {
	APIVersion                string                     `json:"apiVersion"`
	Tenant                    string                     `json:"tenant"`
	ObservedAt                time.Time                  `json:"observedAt"`
	IntentDigest              string                     `json:"intentDigest"`
	MeasurementDigest         string                     `json:"measurementDigest"`
	SourceDigest              string                     `json:"sourceDigest"`
	CatalogDigest             string                     `json:"catalogDigest"`
	ExpectedRevision          int64                      `json:"expectedRevision"`
	State                     string                     `json:"state"`
	AdapterCalls              int                        `json:"adapterCalls"`
	MaxAdapterCalls           int                        `json:"maxAdapterCalls"`
	Comparisons               []IntentEvidenceComparison `json:"comparisons"`
	Warnings                  []string                   `json:"warnings"`
	HumanConfirmationRequired bool                       `json:"humanConfirmationRequired"`
	Digest                    string                     `json:"digest"`
}
