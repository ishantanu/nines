package model

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func sessionSLO() SLO {
	m := v1.MeasurementSpec{Type: "sessions", Metric: "app_sessions_total", EventMetric: "app_sessions_crashed_total", Labels: map[string]string{"app": "ios"}, ScrapeIntervalSeconds: 15, ObservationPoint: "One increment per app session; the event counter increments once per crashed session."}
	return SLO{ID: "ios-crash-free", Project: "p", Service: "ios", Name: "iOS crash-free sessions", Target: .995, WindowSeconds: 3600, StepSeconds: 60, Source: "primary", Measurement: &m, ProviderWindow: MeasurementWindow(m)}
}

func TestSessionMeasurementDerivesGoodAsTotalMinusEvents(t *testing.T) {
	s := sessionSLO()
	w := s.ProviderWindow
	if w.GoodSelector != "" || w.TotalSelector != `app_sessions_total{app="ios"}` || w.BadSelector != `app_sessions_crashed_total{app="ios"}` {
		t.Fatalf("%+v", w)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("a valid session SLO was rejected: %v", err)
	}
	good, total, coverage := WindowQueries(s, 3600)
	if good != `(sum(increase(app_sessions_total{app="ios"}[3600s])) - sum(increase(app_sessions_crashed_total{app="ios"}[3600s])))` {
		t.Fatalf("good must be total minus the event counter: %s", good)
	}
	if total != `sum(increase(app_sessions_total{app="ios"}[3600s]))` {
		t.Fatalf("total: %s", total)
	}
	// Both counters must be present for coverage: the event counter stands in for the numerator.
	if !strings.Contains(coverage, "app_sessions_crashed_total") || !strings.Contains(coverage, "app_sessions_total") {
		t.Fatalf("coverage must require both counters: %s", coverage)
	}
}

func TestSessionMeasurementRejectsAmbiguousDefinitions(t *testing.T) {
	for name, mutate := range map[string]func(*v1.MeasurementSpec){
		"missing event metric": func(m *v1.MeasurementSpec) { m.EventMetric = "" },
		"same metric":          func(m *v1.MeasurementSpec) { m.EventMetric = m.Metric },
		"success labels":       func(m *v1.MeasurementSpec) { m.SuccessLabels = map[string]string{"ok": "true"} },
		"threshold":            func(m *v1.MeasurementSpec) { x := .3; m.ThresholdSeconds = &x },
		"count from":           func(m *v1.MeasurementSpec) { m.CountFrom = "counter" },
		"bad metric name":      func(m *v1.MeasurementSpec) { m.EventMetric = "a b" },
	} {
		s := sessionSLO()
		m := *s.Measurement
		mutate(&m)
		s.Measurement, s.ProviderWindow = &m, MeasurementWindow(m)
		if err := s.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// An event metric belongs to sessions only.
	avail := SLO{ID: "a", Project: "p", Service: "s", Name: "a", Target: .99, WindowSeconds: 3600, StepSeconds: 60, Source: "primary"}
	m := v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", EventMetric: "x_total", Labels: map[string]string{"service": "s"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 15, ObservationPoint: "Completed requests."}
	avail.Measurement, avail.ProviderWindow = &m, MeasurementWindow(m)
	if err := avail.Validate(); err == nil {
		t.Error("an availability measurement accepted an eventMetric")
	}
	// Both selectors at once would be ambiguous.
	both := sessionSLO()
	both.ProviderWindow.GoodSelector = `app_sessions_ok_total{app="ios"}`
	if err := both.Validate(); err == nil {
		t.Error("goodSelector together with badSelector accepted")
	}
	// Features that read a good counter per group refuse sessions explicitly.
	cust := sessionSLO()
	cust.Customers = &v1.CustomerPolicy{Label: "account", Customers: []v1.CustomerRef{{Value: "acme"}}}
	if err := cust.Validate(); err == nil {
		t.Error("per-customer reporting accepted a session measurement")
	}
}
