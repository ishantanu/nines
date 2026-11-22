// Package v1beta1 is the nines.dev/v1beta1 definition schema users write.
//
// Compared with v1alpha1 it has one way to write each thing: durations are
// strings (window: 28d), an SLO is measured by exactly one of measurement,
// queries or binding, telemetry connections are called connection, and
// omitted settings take documented defaults that plan output shows. The
// server stores definitions in the v1alpha1 form; ToHub and FromHub convert.
package v1beta1

import (
	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

const APIVersion = "nines.dev/v1beta1"

// Kinds keep their v1alpha1 names.
const (
	SLOKind           = v1alpha1.SLOKind
	JourneyKind       = v1alpha1.JourneyKind
	BindingKind       = v1alpha1.BindingKind
	DependencyMapKind = v1alpha1.DependencyMapKind
)

type Metadata = v1alpha1.Metadata

// Unchanged nested shapes are shared with v1alpha1.
type (
	CustomerPolicy     = v1alpha1.CustomerPolicy
	CustomerRef        = v1alpha1.CustomerRef
	ErrorBudgetPolicy  = v1alpha1.ErrorBudgetPolicy
	BudgetBand         = v1alpha1.BudgetBand
	SLOLifecycle       = v1alpha1.SLOLifecycle
	BindingReference   = v1alpha1.BindingReference
	JourneyStep        = v1alpha1.JourneyStep
	MapService         = v1alpha1.MapService
	DeclaredDependency = v1alpha1.DeclaredDependency
)

type ServiceLevelObjective struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       SLOSpec  `json:"spec" yaml:"spec"`
}

type SLOSpec struct {
	DisplayName string    `json:"displayName" yaml:"displayName"`
	Service     string    `json:"service" yaml:"service"`
	Target      float64   `json:"target" yaml:"target"`
	Window      *Duration `json:"window,omitempty" yaml:"window,omitempty"`
	Step        *Duration `json:"step,omitempty" yaml:"step,omitempty"`
	// Connection is the telemetry connection to read; omitted means primary.
	Connection string            `json:"connection,omitempty" yaml:"connection,omitempty"`
	Labels     map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Lifecycle  SLOLifecycle      `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`

	// Exactly one of Measurement, Queries or Binding says what is counted.
	Measurement *MeasurementSpec  `json:"measurement,omitempty" yaml:"measurement,omitempty"`
	Queries     *Queries          `json:"queries,omitempty" yaml:"queries,omitempty"`
	Binding     *BindingReference `json:"binding,omitempty" yaml:"binding,omitempty"`

	// Intervals judges intervals instead of events: errorRate (with an
	// optional minEvents) for good-minute SLOs, or minGoodEvents for
	// throughput SLOs.
	Intervals *Intervals `json:"intervals,omitempty" yaml:"intervals,omitempty"`
	// Policies are the burn-rate alerts. Omitted means the standard
	// multiwindow alerts; an empty list turns alerting off.
	Policies *[]BurnRatePolicy `json:"policies,omitempty" yaml:"policies,omitempty"`

	ErrorBudgetPolicy *ErrorBudgetPolicy `json:"errorBudgetPolicy,omitempty" yaml:"errorBudgetPolicy,omitempty"`
	Quality           *QualityPolicy     `json:"quality,omitempty" yaml:"quality,omitempty"`
	Customers         *CustomerPolicy    `json:"customers,omitempty" yaml:"customers,omitempty"`
}

// MeasurementSpec is an inline measurement compiled from one metric family.
type MeasurementSpec struct {
	Type          string            `json:"type" yaml:"type"`
	Metric        string            `json:"metric" yaml:"metric"`
	EventMetric   string            `json:"eventMetric,omitempty" yaml:"eventMetric,omitempty"`
	Labels        map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	SuccessLabels map[string]string `json:"successLabels,omitempty" yaml:"successLabels,omitempty"`
	CountFrom     string            `json:"countFrom,omitempty" yaml:"countFrom,omitempty"`
	Threshold     *Duration         `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	// ScrapeInterval defaults to 15s.
	ScrapeInterval   *Duration `json:"scrapeInterval,omitempty" yaml:"scrapeInterval,omitempty"`
	ObservationPoint string    `json:"observationPoint" yaml:"observationPoint"`
}

// Queries measures with the connection's own query language.
type Queries struct {
	Good     string `json:"good,omitempty" yaml:"good,omitempty"`
	Total    string `json:"total,omitempty" yaml:"total,omitempty"`
	Language string `json:"language,omitempty" yaml:"language,omitempty"`
	// Window reads whole-window counter increases from selectors instead of
	// per-interval queries.
	Window *WindowSelectors `json:"window,omitempty" yaml:"window,omitempty"`
}

type WindowSelectors struct {
	Good           string    `json:"good,omitempty" yaml:"good,omitempty"`
	Total          string    `json:"total" yaml:"total"`
	Bad            string    `json:"bad,omitempty" yaml:"bad,omitempty"`
	ScrapeInterval *Duration `json:"scrapeInterval,omitempty" yaml:"scrapeInterval,omitempty"`
}

type Intervals struct {
	ErrorRate     float64 `json:"errorRate,omitempty" yaml:"errorRate,omitempty"`
	MinEvents     float64 `json:"minEvents,omitempty" yaml:"minEvents,omitempty"`
	MinGoodEvents float64 `json:"minGoodEvents,omitempty" yaml:"minGoodEvents,omitempty"`
}

type BurnRatePolicy struct {
	Name        string    `json:"name" yaml:"name"`
	Severity    string    `json:"severity" yaml:"severity"`
	ShortWindow Duration  `json:"shortWindow" yaml:"shortWindow"`
	LongWindow  Duration  `json:"longWindow" yaml:"longWindow"`
	BurnRate    float64   `json:"burnRate" yaml:"burnRate"`
	For         *Duration `json:"for,omitempty" yaml:"for,omitempty"`
	// MinCoverage defaults to 1, full coverage of both windows.
	MinCoverage *float64 `json:"minCoverage,omitempty" yaml:"minCoverage,omitempty"`
	MinEvents   float64  `json:"minEvents,omitempty" yaml:"minEvents,omitempty"`
	Confidence  float64  `json:"confidence,omitempty" yaml:"confidence,omitempty"`
}

type QualityPolicy struct {
	MinEvents    float64   `json:"minEvents,omitempty" yaml:"minEvents,omitempty"`
	MaxSampleAge *Duration `json:"maxSampleAge,omitempty" yaml:"maxSampleAge,omitempty"`
	CohortLabels []string  `json:"cohortLabels,omitempty" yaml:"cohortLabels,omitempty"`
}

type CriticalUserJourney struct {
	APIVersion string      `json:"apiVersion" yaml:"apiVersion"`
	Kind       string      `json:"kind" yaml:"kind"`
	Metadata   Metadata    `json:"metadata" yaml:"metadata"`
	Spec       JourneySpec `json:"spec" yaml:"spec"`
}

type JourneySpec struct {
	DisplayName  string           `json:"displayName" yaml:"displayName"`
	Description  string           `json:"description" yaml:"description"`
	Owner        string           `json:"owner" yaml:"owner"`
	Criticality  string           `json:"criticality" yaml:"criticality"`
	SLOs         []JourneySLO     `json:"slos" yaml:"slos"`
	Steps        []JourneyStep    `json:"steps" yaml:"steps"`
	Agentic      *AgenticContract `json:"agentic,omitempty" yaml:"agentic,omitempty"`
	RunObjective *RunObjective    `json:"runObjective,omitempty" yaml:"runObjective,omitempty"`
}

// JourneySLO links an SLO. Dimension defaults to the SLO's type; write it only
// for cost or safety.
type JourneySLO struct {
	ID        string `json:"id" yaml:"id"`
	Dimension string `json:"dimension,omitempty" yaml:"dimension,omitempty"`
	Required  bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

type AgenticContract struct {
	SuccessDefinition string    `json:"successDefinition" yaml:"successDefinition"`
	MaxDuration       *Duration `json:"maxDuration,omitempty" yaml:"maxDuration,omitempty"`
	MaxCostUSD        *float64  `json:"maxCostUSD,omitempty" yaml:"maxCostUSD,omitempty"`
	MaxSteps          int       `json:"maxSteps,omitempty" yaml:"maxSteps,omitempty"`
	ApprovalRequired  bool      `json:"approvalRequired,omitempty" yaml:"approvalRequired,omitempty"`
}

type RunObjective struct {
	Target  float64  `json:"target" yaml:"target"`
	Window  Duration `json:"window" yaml:"window"`
	MinRuns int      `json:"minRuns" yaml:"minRuns"`
}
