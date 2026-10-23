package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// InlineBinding adapts the shared measurement validation/compiler internally.
// It never reads or creates a MeasurementBinding resource.
func InlineBinding(s SLO) MeasurementBinding {
	m := s.Measurement
	if m == nil {
		return MeasurementBinding{}
	}
	source := s.Source
	if source == "" {
		source = "primary"
	}
	return MeasurementBinding{APIVersion: APIVersion, Kind: v1.BindingKind, Metadata: v1.Metadata{Name: s.ID, Project: s.Project}, Spec: v1.BindingSpec{DisplayName: s.Name, Service: s.Service, Source: source, Measurement: m.Type, Metric: m.Metric, Labels: m.Labels, SuccessLabels: m.SuccessLabels, CountFrom: m.CountFrom, ThresholdSeconds: m.ThresholdSeconds, ScrapeIntervalSeconds: m.ScrapeIntervalSeconds, StepSeconds: s.StepSeconds, ObservationPoint: m.ObservationPoint}}
}
func ValidateInlineSLO(s SLO) error {
	if s.Measurement == nil {
		return nil
	}
	if s.MeasurementBinding != nil || s.GoodQuery != "" || s.TotalQuery != "" {
		return Invalid("inline measurement cannot be combined with a binding reference or custom queries")
	}
	b := InlineBinding(s)
	if err := b.Validate(); err != nil {
		return err
	}
	if s.Measurement.ScrapeIntervalSeconds < 1 {
		return Invalid("measurement.scrapeIntervalSeconds must be positive")
	}
	expected := MeasurementWindow(*s.Measurement)
	if DefinitionDigest(expected) != DefinitionDigest(s.ProviderWindow) {
		return Invalid("generated provider selectors do not match spec.measurement")
	}
	return nil
}
func MeasurementWindow(m v1.MeasurementSpec) *v1.ProviderWindow {
	labels := map[string]string{}
	for k, v := range m.Labels {
		labels[k] = v
	}
	metric := m.Metric
	if m.Type == "latency" || m.CountFrom == "histogram" {
		metric += "_count"
	}
	total := measurementSelector(metric, labels)
	if m.Type == "latency" {
		if m.ThresholdSeconds != nil {
			labels["le"] = strconv.FormatFloat(*m.ThresholdSeconds, 'g', -1, 64)
		}
		metric = m.Metric + "_bucket"
	} else {
		for k, v := range m.SuccessLabels {
			labels[k] = v
		}
	}
	return &v1.ProviderWindow{GoodSelector: measurementSelector(metric, labels), TotalSelector: total, ScrapeIntervalSeconds: m.ScrapeIntervalSeconds}
}
func measurementSelector(metric string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	if len(parts) == 0 {
		return metric
	}
	return metric + "{" + strings.Join(parts, ",") + "}"
}
