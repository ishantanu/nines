package v1alpha1

import "time"

// ResourceSummary is discovery metadata, not a measurement or an instruction.
type ResourceSummary struct {
	Archived    bool   `json:"archived,omitempty"`
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Project     string `json:"project"`
	DisplayName string `json:"displayName"`
	Service     string `json:"service,omitempty"`
	Revision    int64  `json:"revision"`
}
type SearchOptions struct {
	LinkedSLO string `json:"linkedSLO,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Project   string `json:"project,omitempty"`
	Query     string `json:"query,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}
type ResourceSearch struct {
	APIVersion string            `json:"apiVersion"`
	Tenant     string            `json:"tenant"`
	ObservedAt time.Time         `json:"observedAt"`
	TextTrust  string            `json:"textTrust"`
	Items      []ResourceSummary `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}
type ContextOptions struct {
	ExpectedRevision int64     `json:"expectedRevision,omitempty"`
	At               time.Time `json:"at,omitempty"`
	Limit            int       `json:"limit,omitempty"`
	Cursor           string    `json:"cursor,omitempty"`
}
type EvidenceReference struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Href     string `json:"href"`
}
type CollectionHealth struct {
	SourceDigest  string         `json:"sourceDigest,omitempty"`
	SourceFrom    time.Time      `json:"sourceFrom,omitempty"`
	SourceTo      time.Time      `json:"sourceTo,omitempty"`
	Measurement   *BindingHealth `json:"measurement,omitempty"`
	State         string         `json:"state"`
	AttemptedAt   time.Time      `json:"attemptedAt,omitempty"`
	CompletedAt   time.Time      `json:"completedAt,omitempty"`
	LastSuccessAt time.Time      `json:"lastSuccessAt,omitempty"`
	From          time.Time      `json:"from,omitempty"`
	To            time.Time      `json:"to,omitempty"`
	Source        string         `json:"source,omitempty"`
	SLORevision   int64          `json:"sloRevision,omitempty"`
	Points        int            `json:"points"`
	ErrorCode     string         `json:"errorCode,omitempty"`
}
type Freshness struct {
	State                   string     `json:"state"`
	LastSampleAt            *time.Time `json:"lastSampleAt,omitempty"`
	AgeAtObservationSeconds *int64     `json:"ageAtObservationSeconds,omitempty"`
	StaleAfterSeconds       int64      `json:"staleAfterSeconds"`
}
type Measurement struct {
	AlertSuppressedReason   string     `json:"alertSuppressedReason,omitempty"`
	ShortBurnRate           *float64   `json:"shortBurnRate"`
	LongBurnRate            *float64   `json:"longBurnRate"`
	ShortCoverage           *float64   `json:"shortCoverage"`
	LongCoverage            *float64   `json:"longCoverage"`
	EvaluatedAt             time.Time  `json:"evaluatedAt"`
	WindowStart             time.Time  `json:"windowStart"`
	Target                  float64    `json:"target"`
	Reliability             *float64   `json:"reliability"`
	Good                    float64    `json:"good"`
	Total                   float64    `json:"total"`
	Coverage                float64    `json:"coverage"`
	DataStatus              string     `json:"dataStatus"`
	BudgetRemainingFraction *float64   `json:"budgetRemainingFraction"`
	BurnRate                *float64   `json:"burnRate"`
	AlertPhase              string     `json:"alertPhase"`
	LastAlertEvaluation     *time.Time `json:"lastAlertEvaluation,omitempty"`
}
type SLOParameters struct {
	Lifecycle          SLOLifecycle      `json:"lifecycle,omitempty"`
	Intent             *IntentSpec       `json:"intent,omitempty"`
	MeasurementMode    string            `json:"measurementMode"`
	CoverageBasis      string            `json:"coverageBasis"`
	MeasurementBinding *BindingReference `json:"measurementBinding,omitempty"`
	Source             string            `json:"source,omitempty"`
	WindowSeconds      int64             `json:"windowSeconds"`
	StepSeconds        int64             `json:"stepSeconds"`
	Policies           []BurnRatePolicy  `json:"policies,omitempty" yaml:"policies,omitempty"`
	Policy             *BurnRatePolicy   `json:"policy,omitempty"`
}
type SLOContext struct {
	Retirement        *SLORetirement       `json:"retirement,omitempty"`
	BudgetDecision    *ErrorBudgetDecision `json:"budgetDecision,omitempty"`
	Quality           *MeasurementQuality  `json:"quality,omitempty"`
	MeasurementHealth *BindingHealth       `json:"measurementHealth,omitempty"`
	Definition        SLOParameters        `json:"definition"`
	APIVersion        string               `json:"apiVersion"`
	Tenant            string               `json:"tenant"`
	ObservedAt        time.Time            `json:"observedAt"`
	TextTrust         string               `json:"textTrust"`
	Resource          ResourceSummary      `json:"resource"`
	Health            string               `json:"health"`
	Measurement       Measurement          `json:"measurement"`
	Freshness         Freshness            `json:"freshness"`
	Collection        CollectionHealth     `json:"collection"`
	Evidence          []EvidenceReference  `json:"evidence"`
	Warnings          []string             `json:"warnings"`
}
type LinkedSLOContext struct {
	Link    JourneySLO    `json:"link"`
	Context *SLOContext   `json:"context,omitempty"`
	Error   *ContextError `json:"error,omitempty"`
}
type ContextError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Recovery  string `json:"recovery"`
}
type JourneyContext struct {
	APIVersion  string              `json:"apiVersion"`
	Tenant      string              `json:"tenant"`
	ObservedAt  time.Time           `json:"observedAt"`
	RequestedAt time.Time           `json:"requestedAt"`
	TextTrust   string              `json:"textTrust"`
	Resource    ResourceSummary     `json:"resource"`
	StepCount   int                 `json:"stepCount"`
	TotalLinks  int                 `json:"totalLinks"`
	Health      string              `json:"health"`
	HealthScope string              `json:"healthScope"`
	Links       []LinkedSLOContext  `json:"links"`
	NextCursor  string              `json:"nextCursor,omitempty"`
	Evidence    []EvidenceReference `json:"evidence"`
	Warnings    []string            `json:"warnings"`
}
