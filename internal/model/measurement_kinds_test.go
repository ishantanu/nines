package model

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// The SRE book's SLI kinds name what is counted; they reuse the proven
// availability and latency compilation, so their selectors must be identical.
func TestNewMeasurementKindsCompileLikeTheirCounterparts(t *testing.T) {
	threshold := 300.0
	ratio := func(kind string) v1.MeasurementSpec {
		return v1.MeasurementSpec{Type: kind, Metric: "records_total", Labels: map[string]string{"pipeline": "billing"}, SuccessLabels: map[string]string{"verdict": "pass"}, ScrapeIntervalSeconds: 15, ObservationPoint: "x"}
	}
	for _, kind := range []string{"correctness", "durability", "coverage"} {
		if got, want := *MeasurementWindow(ratio(kind)), *MeasurementWindow(ratio("availability")); got != want {
			t.Errorf("%s compiled to %+v, availability to %+v", kind, got, want)
		}
		if !RatioMeasurement(kind) || ThresholdMeasurement(kind) || MeasurementFamily(kind) != "availability" {
			t.Errorf("%s is misclassified", kind)
		}
	}
	age := v1.MeasurementSpec{Type: "freshness", Metric: "record_age_seconds", Labels: map[string]string{"pipeline": "index"}, ThresholdSeconds: &threshold, ScrapeIntervalSeconds: 15, ObservationPoint: "x"}
	lat := age
	lat.Type = "latency"
	if got, want := *MeasurementWindow(age), *MeasurementWindow(lat); got != want {
		t.Errorf("freshness compiled to %+v, latency to %+v", got, want)
	}
	if !strings.Contains(MeasurementWindow(age).GoodSelector, `record_age_seconds_bucket`) || !strings.Contains(MeasurementWindow(age).GoodSelector, `^(300|300\\.0)$`) {
		t.Errorf("freshness must read the age histogram bucket at the threshold: %s", MeasurementWindow(age).GoodSelector)
	}
}

func TestNewMeasurementKindsValidateTheirOwnShape(t *testing.T) {
	threshold := 300.0
	binding := func(kind string, success map[string]string, th *float64) MeasurementBinding {
		return MeasurementBinding{APIVersion: APIVersion, Kind: v1.BindingKind, Metadata: v1.Metadata{Name: "b", Project: "p"}, Spec: v1.BindingSpec{DisplayName: "B", Service: "svc", Source: "primary", Measurement: kind, Metric: "work_total", Labels: map[string]string{"job": "etl"}, SuccessLabels: success, ThresholdSeconds: th, StepSeconds: 60, ObservationPoint: "Every unit of work."}}
	}
	for _, kind := range []string{"correctness", "durability", "coverage"} {
		if err := binding(kind, map[string]string{"outcome": "good"}, nil).Validate(); err != nil {
			t.Errorf("%s with success labels rejected: %v", kind, err)
		}
		err := binding(kind, nil, nil).Validate()
		if err == nil || !strings.Contains(err.Error(), kind+" requires successLabels") {
			t.Errorf("%s without success labels: %v", kind, err)
		}
		if binding(kind, map[string]string{"outcome": "good"}, &threshold).Validate() == nil {
			t.Errorf("%s accepted a threshold", kind)
		}
	}
	if err := binding("freshness", nil, &threshold).Validate(); err != nil {
		t.Errorf("freshness with a threshold rejected: %v", err)
	}
	err := binding("freshness", nil, nil).Validate()
	if err == nil || !strings.Contains(err.Error(), "oldest acceptable data age") {
		t.Errorf("freshness without a threshold: %v", err)
	}
	if binding("throughput", nil, nil).Validate() == nil {
		t.Error("throughput is an interval rule, not a measurement type")
	}
}
