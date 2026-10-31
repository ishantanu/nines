// Package openslo exports a bounded OpenSLO v1 interoperability subset.
// Nines resources remain authoritative; this is not a lossless replacement.
package openslo

import (
	"fmt"
	"regexp"

	"github.com/prometheus/prometheus/promql/parser"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/promqlvalidate"
)

const APIVersion = "openslo/v1"

type Metadata struct {
	Name        string            `json:"name" yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
type Document struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       Spec     `json:"spec" yaml:"spec"`
}
type Spec struct {
	Service         string      `json:"service" yaml:"service"`
	BudgetingMethod string      `json:"budgetingMethod" yaml:"budgetingMethod"`
	Indicator       Indicator   `json:"indicator" yaml:"indicator"`
	TimeWindow      []Window    `json:"timeWindow" yaml:"timeWindow"`
	Objectives      []Objective `json:"objectives" yaml:"objectives"`
}
type Indicator struct {
	Metadata Metadata      `json:"metadata" yaml:"metadata"`
	Spec     IndicatorSpec `json:"spec" yaml:"spec"`
}
type IndicatorSpec struct {
	RatioMetric Ratio `json:"ratioMetric" yaml:"ratioMetric"`
}
type Ratio struct {
	Counter bool   `json:"counter" yaml:"counter"`
	Good    Metric `json:"good" yaml:"good"`
	Total   Metric `json:"total" yaml:"total"`
}
type Metric struct {
	MetricSource MetricSource `json:"metricSource" yaml:"metricSource"`
}
type MetricSource struct {
	Type string `json:"type" yaml:"type"`
	Spec Query  `json:"spec" yaml:"spec"`
}
type Query struct {
	Query string `json:"query" yaml:"query"`
}
type Window struct {
	Duration  string `json:"duration" yaml:"duration"`
	IsRolling bool   `json:"isRolling" yaml:"isRolling"`
}
type Objective struct {
	DisplayName string  `json:"displayName" yaml:"displayName"`
	Target      float64 `json:"target" yaml:"target"`
}
type Result struct {
	Document Document `json:"document" yaml:"document"`
	Warnings []string `json:"warnings" yaml:"warnings"`
}

func Export(def v1.ServiceLevelObjective) (Result, error) {
	s := model.SLODefinition(def).SLO()
	if err := model.SLODefinition(def).Validate(); err != nil {
		return Result{}, err
	}
	if s.QueryLanguage != "" && s.QueryLanguage != "promql" {
		return Result{}, model.Invalid("OpenSLO export requires PromQL counter selectors")
	}
	if !regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`).MatchString(s.ID) || len(s.ID) > 63 {
		return Result{}, model.Invalid("OpenSLO export requires an RFC1123 name up to 63 characters")
	}
	if s.ProviderWindow == nil || s.MeasurementBinding != nil {
		return Result{}, model.Invalid("OpenSLO export requires inline measurement or providerWindow counter selectors; explicit interval queries and binding references cannot be translated losslessly")
	}
	metric := func(q string) (Metric, error) {
		expr, err := promqlvalidate.InstantVector(q)
		if err != nil {
			return Metric{}, model.Invalid("invalid OpenSLO counter selector")
		}
		v, ok := expr.(*parser.VectorSelector)
		if !ok || v.Name == "" || v.OriginalOffset != 0 || v.Timestamp != nil || v.StartOrEnd != 0 {
			return Metric{}, model.Invalid("OpenSLO export requires bare metric selectors without offsets or timestamp modifiers")
		}
		return Metric{MetricSource: MetricSource{Type: "Prometheus", Spec: Query{Query: q}}}, nil
	}
	good, err := metric(s.ProviderWindow.GoodSelector)
	if err != nil {
		return Result{}, err
	}
	total, err := metric(s.ProviderWindow.TotalSelector)
	if err != nil {
		return Result{}, err
	}
	if s.WindowSeconds%60 != 0 {
		return Result{}, model.Invalid("OpenSLO v1 export requires a window representable in whole minutes")
	}
	duration := fmt.Sprintf("%dm", s.WindowSeconds/60)
	if s.WindowSeconds%3600 == 0 {
		duration = fmt.Sprintf("%dh", s.WindowSeconds/3600)
	}
	if s.WindowSeconds%86400 == 0 {
		duration = fmt.Sprintf("%dd", s.WindowSeconds/86400)
	}
	warnings := []string{"Nines source, project, scrape interval, step and measurement-quality semantics are not represented by this OpenSLO subset. Configure the receiving provider explicitly."}
	if len(s.AlertPolicies()) > 0 {
		warnings = append(warnings, "Burn policies are not exported as OpenSLO alertPolicies; publish alerts separately.")
	}
	if s.Intent != nil || s.ErrorBudgetPolicy != nil || s.Quality != nil || s.Lifecycle != "" {
		warnings = append(warnings, "Nines intent, lifecycle, quality and error-budget governance are retained only in Nines.")
	}
	doc := Document{APIVersion: APIVersion, Kind: "SLO", Metadata: Metadata{Name: s.ID}, Spec: Spec{Service: s.Service, BudgetingMethod: "Occurrences", Indicator: Indicator{Metadata: Metadata{Name: s.ID}, Spec: IndicatorSpec{RatioMetric: Ratio{Counter: true, Good: good, Total: total}}}, TimeWindow: []Window{{Duration: duration, IsRolling: true}}, Objectives: []Objective{{DisplayName: s.Name, Target: s.Target}}}}
	return Result{Document: doc, Warnings: warnings}, nil
}
