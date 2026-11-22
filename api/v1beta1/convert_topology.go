package v1beta1

import (
	"fmt"
	"math"
	"time"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

type MeasurementBinding struct {
	APIVersion string      `json:"apiVersion" yaml:"apiVersion"`
	Kind       string      `json:"kind" yaml:"kind"`
	Metadata   Metadata    `json:"metadata" yaml:"metadata"`
	Spec       BindingSpec `json:"spec" yaml:"spec"`
}

// BindingSpec is a reusable measurement. Type matches MeasurementSpec.Type.
type BindingSpec struct {
	DisplayName    string            `json:"displayName" yaml:"displayName"`
	Service        string            `json:"service" yaml:"service"`
	Connection     string            `json:"connection,omitempty" yaml:"connection,omitempty"`
	Type           string            `json:"type" yaml:"type"`
	Metric         string            `json:"metric" yaml:"metric"`
	EventMetric    string            `json:"eventMetric,omitempty" yaml:"eventMetric,omitempty"`
	Labels         map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	SuccessLabels  map[string]string `json:"successLabels,omitempty" yaml:"successLabels,omitempty"`
	CountFrom      string            `json:"countFrom,omitempty" yaml:"countFrom,omitempty"`
	Threshold      *Duration         `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	ScrapeInterval *Duration         `json:"scrapeInterval,omitempty" yaml:"scrapeInterval,omitempty"`
	// Step is the interval of SLOs compiled from this binding; it defaults to
	// 1m, the same default as an SLO's step.
	Step             *Duration `json:"step,omitempty" yaml:"step,omitempty"`
	ObservationPoint string    `json:"observationPoint" yaml:"observationPoint"`
}

type ServiceDependencyMap struct {
	APIVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   Metadata          `json:"metadata" yaml:"metadata"`
	Spec       DependencyMapSpec `json:"spec" yaml:"spec"`
}

type DependencyMapSpec struct {
	DisplayName  string               `json:"displayName" yaml:"displayName"`
	Services     []MapService         `json:"services" yaml:"services"`
	Dependencies []DeclaredDependency `json:"dependencies" yaml:"dependencies"`
	Observed     *GraphObservation    `json:"observed,omitempty" yaml:"observed,omitempty"`
}

type GraphObservation struct {
	Connection     string            `json:"connection,omitempty" yaml:"connection,omitempty"`
	Labels         map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Window         Duration          `json:"window" yaml:"window"`
	ScrapeInterval *Duration         `json:"scrapeInterval,omitempty" yaml:"scrapeInterval,omitempty"`
	MaxSampleAge   *Duration         `json:"maxSampleAge,omitempty" yaml:"maxSampleAge,omitempty"`
}

func optionalSeconds(d *Duration, field string) (int64, error) {
	if d == nil {
		return 0, nil
	}
	return d.WholeSeconds(field)
}

func optionalDur(seconds int64) *Duration {
	if seconds <= 0 {
		return nil
	}
	return dur(seconds)
}

// ToHub converts a binding to the stored v1alpha1 form.
func (b MeasurementBinding) ToHub() (v1alpha1.MeasurementBinding, error) {
	if b.APIVersion != APIVersion || b.Kind != BindingKind {
		return v1alpha1.MeasurementBinding{}, fmt.Errorf("expected apiVersion %s and kind %s", APIVersion, BindingKind)
	}
	in := b.Spec
	step, err := seconds(in.Step, DefaultStep, "step")
	if err != nil {
		return v1alpha1.MeasurementBinding{}, err
	}
	scrape, err := optionalSeconds(in.ScrapeInterval, "scrapeInterval")
	if err != nil {
		return v1alpha1.MeasurementBinding{}, err
	}
	labels := in.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	connection := in.Connection
	if connection == "" {
		connection = "primary"
	}
	out := v1alpha1.BindingSpec{ScrapeIntervalSeconds: scrape, CountFrom: in.CountFrom, DisplayName: in.DisplayName, Service: in.Service, Source: connection, Measurement: in.Type, Metric: in.Metric, EventMetric: in.EventMetric, Labels: labels, SuccessLabels: in.SuccessLabels, StepSeconds: step, ObservationPoint: in.ObservationPoint}
	if in.Threshold != nil {
		t := in.Threshold.Seconds()
		out.ThresholdSeconds = &t
	}
	return v1alpha1.MeasurementBinding{APIVersion: v1alpha1.APIVersion, Kind: BindingKind, Metadata: b.Metadata, Spec: out}, nil
}

// BindingFromHub converts a stored binding for export.
func BindingFromHub(h v1alpha1.MeasurementBinding) MeasurementBinding {
	in := h.Spec
	out := BindingSpec{DisplayName: in.DisplayName, Service: in.Service, Connection: exportConnection(in.Source), Type: in.Measurement, Metric: in.Metric, EventMetric: in.EventMetric, Labels: in.Labels, SuccessLabels: in.SuccessLabels, CountFrom: in.CountFrom, ScrapeInterval: optionalDur(in.ScrapeIntervalSeconds), ObservationPoint: in.ObservationPoint}
	if in.StepSeconds != int64(DefaultStep/time.Second) {
		out.Step = optionalDur(in.StepSeconds)
	}
	if in.ThresholdSeconds != nil {
		t := D(time.Duration(math.Round(*in.ThresholdSeconds * float64(time.Second))))
		out.Threshold = &t
	}
	return MeasurementBinding{APIVersion: APIVersion, Kind: BindingKind, Metadata: h.Metadata, Spec: out}
}

// ToHub converts a dependency map to the stored v1alpha1 form.
func (m ServiceDependencyMap) ToHub() (v1alpha1.ServiceDependencyMap, error) {
	if m.APIVersion != APIVersion || m.Kind != DependencyMapKind {
		return v1alpha1.ServiceDependencyMap{}, fmt.Errorf("expected apiVersion %s and kind %s", APIVersion, DependencyMapKind)
	}
	out := v1alpha1.DependencyMapSpec{DisplayName: m.Spec.DisplayName, Services: m.Spec.Services, Dependencies: m.Spec.Dependencies}
	if o := m.Spec.Observed; o != nil {
		window, err := o.Window.WholeSeconds("observed.window")
		if err != nil {
			return v1alpha1.ServiceDependencyMap{}, err
		}
		scrape, err := optionalSeconds(o.ScrapeInterval, "observed.scrapeInterval")
		if err != nil {
			return v1alpha1.ServiceDependencyMap{}, err
		}
		age, err := optionalSeconds(o.MaxSampleAge, "observed.maxSampleAge")
		if err != nil {
			return v1alpha1.ServiceDependencyMap{}, err
		}
		labels := o.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		connection := o.Connection
		if connection == "" {
			connection = "primary"
		}
		out.Observed = &v1alpha1.GraphObservation{Source: connection, Labels: labels, WindowSeconds: window, ScrapeIntervalSeconds: scrape, MaxSampleAgeSeconds: age}
	}
	return v1alpha1.ServiceDependencyMap{APIVersion: v1alpha1.APIVersion, Kind: DependencyMapKind, Metadata: m.Metadata, Spec: out}, nil
}

// DependencyMapFromHub converts a stored dependency map for export.
func DependencyMapFromHub(h v1alpha1.ServiceDependencyMap) ServiceDependencyMap {
	out := DependencyMapSpec{DisplayName: h.Spec.DisplayName, Services: h.Spec.Services, Dependencies: h.Spec.Dependencies}
	if o := h.Spec.Observed; o != nil {
		out.Observed = &GraphObservation{Connection: exportConnection(o.Source), Labels: o.Labels, Window: D(time.Duration(o.WindowSeconds) * time.Second), ScrapeInterval: optionalDur(o.ScrapeIntervalSeconds), MaxSampleAge: optionalDur(o.MaxSampleAgeSeconds)}
	}
	return ServiceDependencyMap{APIVersion: APIVersion, Kind: DependencyMapKind, Metadata: h.Metadata, Spec: out}
}

// exportConnection leaves the default connection unwritten.
func exportConnection(source string) string {
	if source == "primary" {
		return ""
	}
	return source
}
