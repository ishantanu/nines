package service

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func sessionBinding() model.MeasurementBinding {
	return model.MeasurementBinding{Spec: v1.BindingSpec{Measurement: "sessions", Metric: "app_sessions_total", EventMetric: "app_sessions_crashed_total", Labels: map[string]string{"app": "ios"}}}
}
func sessionEvidence() v1.TelemetryDiscovery {
	return v1.TelemetryDiscovery{
		Metadata: []v1.TelemetryMetadata{{Type: "counter"}, {Type: "counter"}},
		Series:   []map[string]string{{"__name__": "app_sessions_total", "app": "ios"}, {"__name__": "app_sessions_crashed_total", "app": "ios"}},
	}
}

func TestSessionEvidenceRequiresBothCountersAndNeverGuessesAMissingOne(t *testing.T) {
	b := sessionBinding()
	if err := verifyBindingEvidence(b, sessionEvidence()); err != nil {
		t.Fatalf("complete evidence was rejected: %v", err)
	}
	for name, mutate := range map[string]func(*v1.TelemetryDiscovery){
		"no event series":     func(d *v1.TelemetryDiscovery) { d.Series = d.Series[:1] },
		"no session series":   func(d *v1.TelemetryDiscovery) { d.Series = d.Series[1:] },
		"gauge event":         func(d *v1.TelemetryDiscovery) { d.Metadata[1].Type = "gauge" },
		"histogram session":   func(d *v1.TelemetryDiscovery) { d.Metadata[0].Type = "histogram" },
		"single metadata":     func(d *v1.TelemetryDiscovery) { d.Metadata = d.Metadata[:1] },
		"truncated discovery": func(d *v1.TelemetryDiscovery) { d.Truncated = true },
		"no series at all":    func(d *v1.TelemetryDiscovery) { d.Series = nil },
	} {
		d := sessionEvidence()
		d.Metadata = append([]v1.TelemetryMetadata{}, d.Metadata...)
		d.Series = append([]map[string]string{}, d.Series...)
		mutate(&d)
		if err := verifyBindingEvidence(b, d); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSessionBindingQueriesSubtractTheEventCounter(t *testing.T) {
	b := sessionBinding()
	b.Spec.StepSeconds = 60
	good, total := BindingQueries(b)
	if good != `(sum(increase(app_sessions_total{app="ios"}[60s])) - sum(increase(app_sessions_crashed_total{app="ios"}[60s])))` || total != `sum(increase(app_sessions_total{app="ios"}[60s]))` {
		t.Fatalf("%s / %s", good, total)
	}
	if metadataExpected("sessions") != 2 || metadataExpected("availability") != 1 || metadataExpected("latency") != 1 {
		t.Fatal("metadata expectations")
	}
}
