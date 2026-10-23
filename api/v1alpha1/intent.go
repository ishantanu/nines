package v1alpha1

import "time"

const IntentKind = "ServiceLevelObjectiveIntent"

// SLOIntent is an authoring document. Plan it, then review/apply the generated SLO.
// It contains no provider query language and is never directly evaluated.
type SLOIntent struct {
	APIVersion string     `json:"apiVersion" yaml:"apiVersion"`
	Kind       string     `json:"kind" yaml:"kind"`
	Metadata   Metadata   `json:"metadata" yaml:"metadata"`
	Spec       IntentSpec `json:"spec" yaml:"spec"`
}
type IntentSpec struct {
	Measurement      *MeasurementSpec  `json:"measurement,omitempty" yaml:"measurement,omitempty"`
	StepSeconds      int64             `json:"stepSeconds,omitempty" yaml:"stepSeconds,omitempty"`
	Description      string            `json:"description" yaml:"description"`
	Service          string            `json:"service" yaml:"service"`
	Source           string            `json:"source" yaml:"source"`
	Outcome          string            `json:"outcome" yaml:"outcome"`
	Population       map[string]string `json:"population" yaml:"population"`
	ThresholdSeconds *float64          `json:"thresholdSeconds,omitempty" yaml:"thresholdSeconds,omitempty"`
	Target           float64           `json:"target" yaml:"target"`
	WindowSeconds    int64             `json:"windowSeconds" yaml:"windowSeconds"`
	NoDataPolicy     string            `json:"noDataPolicy" yaml:"noDataPolicy"`
	Policies         []BurnRatePolicy  `json:"policies,omitempty" yaml:"policies,omitempty"`
	Policy           *BurnRatePolicy   `json:"policy,omitempty" yaml:"policy,omitempty"`
	Journey          *IntentJourney    `json:"journey,omitempty" yaml:"journey,omitempty"`
}
type IntentJourney struct {
	Name             string `json:"name" yaml:"name"`
	Step             string `json:"step" yaml:"step"`
	ExpectedRevision int64  `json:"expectedRevision" yaml:"expectedRevision"`
	Required         bool   `json:"required" yaml:"required"`
}
type IntentSelection struct {
	Name             string `json:"name" yaml:"name"`
	ExpectedRevision int64  `json:"expectedRevision" yaml:"expectedRevision"`
}
type PlanIntentInput struct {
	Intent  SLOIntent        `json:"intent" yaml:"intent"`
	Binding *IntentSelection `json:"binding,omitempty" yaml:"binding,omitempty"`
}
type IntentCandidate struct {
	Binding    BindingReference   `json:"binding"`
	Definition MeasurementBinding `json:"definition"`
}
type IntentJourneyPlan struct {
	ExpectedRevision int64               `json:"expectedRevision"`
	Manifest         CriticalUserJourney `json:"manifest"`
	YAML             string              `json:"yaml"`
	Digest           string              `json:"digest"`
}
type IntentPlan struct {
	SourceDigest     string             `json:"sourceDigest"`
	ExpectedRevision int64              `json:"expectedRevision"`
	APIVersion       string             `json:"apiVersion"`
	Tenant           string             `json:"tenant"`
	Status           string             `json:"status"`
	Intent           SLOIntent          `json:"intent"`
	IntentDigest     string             `json:"intentDigest"`
	Candidates       []IntentCandidate  `json:"candidates"`
	Truncated        bool               `json:"truncated"`
	Clarifications   []string           `json:"clarifications"`
	Compiled         *CompiledBinding   `json:"compiled,omitempty"`
	Journey          *IntentJourneyPlan `json:"journey,omitempty"`
	Warnings         []string           `json:"warnings"`
}

type IntentBacktestInput struct {
	Plan              PlanIntentInput `json:"plan"`
	From              time.Time       `json:"from"`
	To                time.Time       `json:"to"`
	ResolutionSeconds int64           `json:"resolutionSeconds"`
}
type IntentBacktestPoint struct {
	PolicyBreaches        []string  `json:"policyBreaches,omitempty"`
	ShortCoverage         float64   `json:"shortCoverage"`
	LongCoverage          float64   `json:"longCoverage"`
	At                    time.Time `json:"at"`
	Reliability           *float64  `json:"reliability"`
	Coverage              float64   `json:"coverage"`
	DataStatus            string    `json:"dataStatus"`
	TargetViolated        bool      `json:"targetViolated"`
	ShortBurn             *float64  `json:"shortBurn"`
	LongBurn              *float64  `json:"longBurn"`
	BurnThresholdExceeded bool      `json:"burnThresholdExceeded"`
}
type IntentBacktest struct {
	SourceDigest           string                `json:"sourceDigest"`
	APIVersion             string                `json:"apiVersion"`
	Tenant                 string                `json:"tenant"`
	IntentDigest           string                `json:"intentDigest"`
	ManifestDigest         string                `json:"manifestDigest"`
	Binding                BindingReference      `json:"binding"`
	From                   time.Time             `json:"from"`
	To                     time.Time             `json:"to"`
	ResolutionSeconds      int64                 `json:"resolutionSeconds"`
	Points                 []IntentBacktestPoint `json:"points"`
	CompleteCutoffs        int                   `json:"completeCutoffs"`
	UnknownCutoffs         int                   `json:"unknownCutoffs"`
	NoTrafficCutoffs       int                   `json:"noTrafficCutoffs"`
	TargetViolationCutoffs int                   `json:"targetViolationCutoffs"`
	BurnThresholdCutoffs   int                   `json:"burnThresholdCutoffs"`
	Warnings               []string              `json:"warnings"`
}

// DraftIntentInput fixes measurement scope outside model control. Only Text is sent.
type DraftIntentInput struct {
	Text         string            `json:"text" yaml:"text"`
	Metadata     Metadata          `json:"metadata" yaml:"metadata"`
	Service      string            `json:"service" yaml:"service"`
	Source       string            `json:"source" yaml:"source"`
	Population   map[string]string `json:"population" yaml:"population"`
	NoDataPolicy string            `json:"noDataPolicy" yaml:"noDataPolicy"`
}
type ModelGoal struct {
	Outcome        string   `json:"outcome"`
	TargetText     *string  `json:"targetText"`
	WindowText     *string  `json:"windowText"`
	ThresholdText  *string  `json:"thresholdText"`
	Clarifications []string `json:"clarifications"`
}
type IntentDraft struct {
	APIVersion     string     `json:"apiVersion"`
	Tenant         string     `json:"tenant"`
	Status         string     `json:"status"`
	Model          string     `json:"model"`
	Intent         *SLOIntent `json:"intent,omitempty"`
	Clarifications []string   `json:"clarifications"`
	Warnings       []string   `json:"warnings"`
}
