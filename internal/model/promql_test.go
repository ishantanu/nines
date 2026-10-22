package model

import (
	"errors"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestSLOPromQLSyntaxAndResultType(t *testing.T) {
	base := SLO{ID: "api", Project: "test", Service: "api", Name: "API", Target: .999, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	for _, field := range []string{"goodQuery", "totalQuery"} {
		for _, query := range []string{"sum(", "not_a_prometheus_function(up)", "sum(rate(up))", "up[5m]", "1", `"text"`, " ", `up{service="\q"}`} {
			s := base
			if field == "goodQuery" {
				s.GoodQuery = query
			} else {
				s.TotalQuery = query
			}
			err := s.Validate()
			var invalid *ValidationError
			if !errors.As(err, &invalid) || !strings.Contains(err.Error(), field) {
				t.Fatalf("%s %q accepted or missing field diagnostic: %v", field, query, err)
			}
		}
	}
	for _, query := range []string{"vector(1)", "sum(increase(requests_total{status!~\"5..\"}[1m]))", "sum by (service) (increase(requests_total[1m]))", "up # trailing comment", "missing_metric"} {
		s := base
		s.GoodQuery = query
		if err := s.Validate(); err != nil {
			t.Fatalf("valid vector %q rejected: %v", query, err)
		}
	}
	base.GoodQuery = ""
	base.TotalQuery = ""
	if err := base.Validate(); err != nil {
		t.Fatal("manual ingestion must remain valid without queries", err)
	}
}

func TestProviderWindowSelectorsUsePromQLParser(t *testing.T) {
	s := SLO{ID: "api", Project: "test", Service: "api", Name: "API", Target: .999, WindowSeconds: 3600, StepSeconds: 60, ProviderWindow: &v1.ProviderWindow{GoodSelector: `requests_total{status="200"}`, TotalSelector: "requests_total", ScrapeIntervalSeconds: 10}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, good := range []bool{true, false} {
		copy := s
		p := *s.ProviderWindow
		copy.ProviderWindow = &p
		field := "providerWindow.totalSelector"
		if good {
			p.GoodSelector = `requests_total{status="\q"}`
			field = "providerWindow.goodSelector"
		} else {
			p.TotalSelector = `requests_total{status="\q"}`
		}
		if err := copy.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Fatalf("invalid PromQL escape accepted or missing field: %v", err)
		}
	}
}
