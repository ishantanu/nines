package service

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestHistogramEvidenceCanonicalBoundaryAndAmbiguity(t *testing.T) {
	threshold := 2.0
	b := model.MeasurementBinding{Spec: v1.BindingSpec{Measurement: "latency", Metric: "duration_seconds", ThresholdSeconds: &threshold}}
	evidence := func(boundaries ...string) v1.TelemetryDiscovery {
		d := v1.TelemetryDiscovery{Metadata: []v1.TelemetryMetadata{{Type: "histogram", Unit: "seconds"}}, Series: []map[string]string{{"__name__": "duration_seconds_count", "service": "api"}, {"__name__": "duration_seconds_bucket", "service": "api", "le": "+Inf"}}}
		for _, boundary := range boundaries {
			d.Series = append(d.Series, map[string]string{"__name__": "duration_seconds_bucket", "service": "api", "le": boundary})
		}
		return d
	}
	for _, boundary := range []string{"2", "2.0"} {
		if e := verifyBindingEvidence(b, evidence(boundary)); e != nil {
			t.Fatal(boundary, e)
		}
	}
	for _, boundaries := range [][]string{{"1.99"}, {"2.01"}, {"2", "2.0"}, {"2e0"}} {
		if verifyBindingEvidence(b, evidence(boundaries...)) == nil {
			t.Fatal("inexact or duplicated boundary admitted", boundaries)
		}
	}
}
