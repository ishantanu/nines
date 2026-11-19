package model

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func customerSLO() SLO {
	m := v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 15, ObservationPoint: "Completed API requests."}
	return SLO{ID: "api", Project: "p", Service: "api", Name: "api", Target: .999, WindowSeconds: 3600, StepSeconds: 60, Source: "primary", Measurement: &m, ProviderWindow: MeasurementWindow(m),
		Customers: &v1.CustomerPolicy{Label: "account", Customers: []v1.CustomerRef{{Value: "acme-01", Name: "Acme"}, {Value: "globex-7"}}}}
}

func TestCustomerPolicyIsBoundedAndNeverOverlapsThePopulation(t *testing.T) {
	if err := customerSLO().Validate(); err != nil {
		t.Fatalf("a valid policy was rejected: %v", err)
	}
	tooMany := customerSLO()
	for i := 0; i < 21; i++ {
		tooMany.Customers.Customers = append(tooMany.Customers.Customers, v1.CustomerRef{Value: "c" + strings.Repeat("x", i+1)})
	}
	target := 1.0
	for name, mutate := range map[string]func(*SLO){
		"too many":            func(s *SLO) { *s = tooMany },
		"none":                func(s *SLO) { s.Customers.Customers = nil },
		"duplicate":           func(s *SLO) { s.Customers.Customers[1].Value = "acme-01" },
		"pinned label":        func(s *SLO) { s.Customers.Label = "service" },
		"success label":       func(s *SLO) { s.Customers.Label = "status" },
		"le label":            func(s *SLO) { s.Customers.Label = "le" },
		"bad label":           func(s *SLO) { s.Customers.Label = "9lives" },
		"quote in value":      func(s *SLO) { s.Customers.Customers[0].Value = `a"b` },
		"regex in value":      func(s *SLO) { s.Customers.Customers[0].Value = "a|b" },
		"leading punctuation": func(s *SLO) { s.Customers.Customers[0].Value = "-a" },
		"target of one":       func(s *SLO) { s.Customers.Customers[0].Target = &target },
		"no inline":           func(s *SLO) { s.Measurement, s.ProviderWindow = nil, nil },
	} {
		s := customerSLO()
		mutate(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCustomerAliasIsStableScopedAndHidesTheValue(t *testing.T) {
	a := CustomerAlias("alpha", "api", "acme-01")
	if a != CustomerAlias("alpha", "api", "acme-01") || strings.Contains(a, "acme") {
		t.Fatalf("alias must be stable and opaque: %s", a)
	}
	if a == CustomerAlias("beta", "api", "acme-01") || a == CustomerAlias("alpha", "other", "acme-01") {
		t.Fatal("aliases must differ across tenants and SLOs")
	}
}
