package model

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProviderQueryYAMLRoundTrip(t *testing.T) {
	s := SLO{ID: "api", Name: "API", Service: "api", Project: "prod", Target: .99, WindowSeconds: 3600, StepSeconds: 60, QueryLanguage: "datadog", GoodQuery: "sum:api.good{env:prod}.as_count().rollup(sum,60)", TotalQuery: "sum:api.total{env:prod}.as_count().rollup(sum,60)"}
	d := DefinitionFromSLO(s)
	b, err := yaml.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSLODefinition(b)
	if err != nil || parsed.SLO().QueryLanguage != "datadog" {
		t.Fatal(parsed, err)
	}
	for _, q := range []string{"avg:api.good{*}.as_count().rollup(sum,60)", "sum:api.good{*}.as_count().rollup(sum,300)", "sum:api.good{*}.as_rate().rollup(sum,60)", "sum:api.good{env:*}.as_count().rollup(sum,60)", "sum:api.good{*}.as_count().rollup(sum,60)+1"} {
		bad := s
		bad.GoodQuery = q
		if err = bad.Validate(); err == nil {
			t.Fatal("accepted", q)
		}
	}
	s.QueryLanguage = ""
	if err = s.Validate(); err == nil {
		t.Fatal("Datadog expression accepted as PromQL")
	}
}
