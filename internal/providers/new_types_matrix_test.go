package providers

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/telemetryprovider"
)

// This test pins which SLO types each provider can host, so the documented
// compatibility table cannot drift from the validators and capability flags.

func open(t *testing.T, kind string, cfg telemetryprovider.Configuration) telemetryprovider.Collector {
	t.Helper()
	c, err := Builtins().Open(kind, cfg)
	if err != nil {
		t.Fatal(kind, err)
	}
	return c
}

func configs() map[string]telemetryprovider.Configuration {
	return map[string]telemetryprovider.Configuration{
		"prometheus": {Endpoint: "https://metrics.example"},
		"dash0":      {Endpoint: "https://api.eu-west-1.aws.dash0.com", Options: map[string]string{"dataset": "production"}, Credentials: map[string]string{"bearerToken": "synthetic"}},
		"datadog":    {Endpoint: "https://api.datadoghq.eu", Credentials: map[string]string{"apiKey": "synthetic", "applicationKey": "synthetic"}},
		"newrelic":   {Endpoint: "https://api.eu.newrelic.com/graphql", Options: map[string]string{"accountID": "123456"}, Credentials: map[string]string{"userKey": "synthetic-key"}},
	}
}

func sessionsSLO() model.SLO {
	return model.SLO{ID: "ios", Name: "iOS", Project: "p", Service: "ios", Target: .995, WindowSeconds: 3600, StepSeconds: 60,
		Measurement: &v1.MeasurementSpec{Type: "sessions", Metric: "app_sessions_total", EventMetric: "app_sessions_crashed_total", Labels: map[string]string{"app": "ios"}, ScrapeIntervalSeconds: 15, ObservationPoint: "One increment per session."}}
}

func goodMinuteSLO(language string) model.SLO {
	return model.SLO{ID: "gm", Name: "gm", Project: "p", Service: "svc", Target: .999, WindowSeconds: 86400, StepSeconds: 60, QueryLanguage: language,
		GoodQuery: "sum(increase(ok_total[1m]))", TotalQuery: "sum(increase(all_total[1m]))", BadInterval: &v1.BadIntervalPolicy{ErrorRate: .06, MinEvents: 20}}
}

func TestSessionSLOsNeedWholeWindowCountsOnAPromQLSource(t *testing.T) {
	for kind, want := range map[string]bool{"prometheus": true, "dash0": true, "datadog": false, "newrelic": false} {
		err := open(t, kind, configs()[kind]).ValidateSLO(sessionsSLO())
		if (err == nil) != want {
			t.Errorf("%s: sessions accepted=%v, want %v (%v)", kind, err == nil, want, err)
		}
	}
	// The capability flag that decides it.
	for kind, want := range map[string]bool{"prometheus": true, "mimir": true, "dash0": true, "victoriametrics": false, "datadog": false, "newrelic": false, "googlecloud": false} {
		d, _ := Builtins().Describe(kind)
		if d.Capabilities.WindowCounts != want {
			t.Errorf("%s windowCounts=%v, want %v", kind, d.Capabilities.WindowCounts, want)
		}
	}
}

func TestGoodMinuteSLOsNeedIntervalCountsInPromQL(t *testing.T) {
	for _, kind := range []string{"prometheus", "dash0"} {
		s := goodMinuteSLO("")
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := open(t, kind, configs()[kind]).ValidateSLO(s); err != nil {
			t.Errorf("%s: good-minute refused: %v", kind, err)
		}
	}
	// The other query languages are refused by the model before any provider is asked.
	for _, language := range []string{"datadog", "nrql", "googlecloud", "metricsql"} {
		if err := goodMinuteSLO(language).Validate(); err == nil {
			t.Errorf("good-minute accepted for query language %s", language)
		}
	}
}

func TestCohortDependentReportsAreAPrometheusFamilyCapability(t *testing.T) {
	// Per-customer reports, failure concentration and peer comparison all read
	// grouped counts through the cohort capability.
	for kind, want := range map[string]bool{"prometheus": true, "mimir": true, "dash0": false, "victoriametrics": false, "datadog": false, "newrelic": false, "googlecloud": false} {
		d, _ := Builtins().Describe(kind)
		if d.Capabilities.Cohorts != want {
			t.Errorf("%s cohorts=%v, want %v", kind, d.Capabilities.Cohorts, want)
		}
	}
}

func throughputSLO(language string) model.SLO {
	s := goodMinuteSLO(language)
	s.BadInterval = &v1.BadIntervalPolicy{MinGoodEvents: 500}
	return s
}

func TestThroughputSLOsNeedIntervalCountsInPromQL(t *testing.T) {
	for _, kind := range []string{"prometheus", "dash0"} {
		s := throughputSLO("")
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := open(t, kind, configs()[kind]).ValidateSLO(s); err != nil {
			t.Errorf("%s: throughput refused: %v", kind, err)
		}
	}
	for _, language := range []string{"datadog", "nrql", "googlecloud", "metricsql"} {
		if err := throughputSLO(language).Validate(); err == nil {
			t.Errorf("throughput accepted for query language %s", language)
		}
	}
}

// Correctness, durability and coverage compile exactly like availability, and
// freshness exactly like latency, so every provider must host them exactly as
// it hosts their counterpart. The documented table relies on this.
func TestNewMeasurementTypesHaveTheirCounterpartsSupport(t *testing.T) {
	threshold := 300.0
	inline := func(kind string) model.SLO {
		m := v1.MeasurementSpec{Type: kind, Metric: "work_total", CountFrom: "counter", Labels: map[string]string{"job": "etl"}, SuccessLabels: map[string]string{"outcome": "good"}, ScrapeIntervalSeconds: 15, ObservationPoint: "Every unit of work."}
		if model.ThresholdMeasurement(kind) {
			m.Metric, m.CountFrom, m.SuccessLabels, m.ThresholdSeconds = "work_age_seconds", "", nil, &threshold
		}
		s := model.SLO{ID: "w", Name: "w", Project: "p", Service: "svc", Source: "primary", Target: .99, WindowSeconds: 86400, StepSeconds: 60, Measurement: &m, ProviderWindow: model.MeasurementWindow(m)}
		if err := s.Validate(); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		return s
	}
	for kind, counterpart := range map[string]string{"correctness": "availability", "durability": "availability", "coverage": "availability", "freshness": "latency"} {
		for provider, cfg := range configs() {
			c := open(t, provider, cfg)
			got, want := c.ValidateSLO(inline(kind)), c.ValidateSLO(inline(counterpart))
			if (got == nil) != (want == nil) {
				t.Errorf("%s on %s: accepted=%v, but %s accepted=%v", kind, provider, got == nil, counterpart, want == nil)
			}
		}
	}
}
