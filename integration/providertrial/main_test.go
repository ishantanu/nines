package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSyntheticDatasetAndDefinitions(t *testing.T) {
	s := newState(time.Unix(1800000000, 0))
	s.Providers = []string{"datadog", "newrelic"}
	if s.To.Sub(s.From) != 6*time.Minute || s.BadTo.Sub(s.From) != 3*time.Minute {
		t.Fatal("unexpected time bounds")
	}
	events := newrelicPayload(s)
	if len(events) != 600 {
		t.Fatal("unbounded event fixture")
	}
	good := [2]int{}
	for i, event := range events {
		if event.(map[string]any)["success"].(bool) {
			good[i/300]++
		}
	}
	if good != [2]int{270, 300} {
		t.Fatal("wrong expected phases", good)
	}
	series := datadogPayload(s).(map[string]any)["series"].([]any)
	if len(series) != 2 {
		t.Fatal("custom metric series budget exceeded")
	}
	for _, entry := range series {
		if len(entry.(map[string]any)["points"].([]any)) != 6 {
			t.Fatal("point budget exceeded")
		}
	}
	for _, p := range s.Providers {
		b, e := yaml.Marshal(slo(s, p))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = model.ParseSLODefinition(b); e != nil {
			t.Fatal(p, e)
		}
	}
	j := model.Journey(journey(s))
	if e := j.Validate(); e != nil {
		t.Fatal(e)
	}
	if len(j.Spec.SLOs) != 2 || len(j.Spec.Steps) != 2 {
		t.Fatal("missing provider links")
	}
}

func TestVendorReadBudgetsAndRouteIsolation(t *testing.T) {
	calls := 0
	transport := &budgetTransport{base: fixtureTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}), calls: map[string]int{}}
	for _, tc := range []struct {
		method, url string
		limit       int
	}{
		{"GET", "https://api.uk1.datadoghq.com/api/v1/query", 24},
		{"POST", "https://api.eu.newrelic.com/graphql", 8},
	} {
		for i := 0; i < tc.limit; i++ {
			req, e := http.NewRequest(tc.method, tc.url, nil)
			if e != nil {
				t.Fatal(e)
			}
			res, e := transport.RoundTrip(req)
			if e != nil {
				t.Fatal(e)
			}
			_ = res.Body.Close()
		}
		req, _ := http.NewRequest(tc.method, tc.url, nil)
		if _, e := transport.RoundTrip(req); e == nil {
			t.Fatal("request budget exceeded")
		}
	}
	for _, url := range []string{"https://api.datadoghq.com/api/v1/query", "http://api.uk1.datadoghq.com/api/v1/query", "https://example.com"} {
		req, _ := http.NewRequest("GET", url, nil)
		if _, e := transport.RoundTrip(req); e == nil {
			t.Fatal("route escaped fixture")
		}
	}
	if calls != 32 {
		t.Fatal("unexpected upstream request count", calls)
	}
}

func TestPlaygroundConfigPreservesPrimaryAndAddsManualTrials(t *testing.T) {
	t.Setenv("NINES_DD_NATIVE_PUBLISH", "0")
	t.Setenv("NINES_NR_NATIVE_PUBLISH", "0")
	dir := t.TempDir()
	input := filepath.Join(dir, "base.yaml")
	output := filepath.Join(dir, "private", "tenants.yaml")
	base := `tenants:
- id: mimir
  name: Existing Mimir
  access: [{tokenEnv: API_TOKEN, role: owner}]
  source: {kind: mimir, url: "http://mimir:9009/prometheus", mimirOrgID: playground}
  systemOne: {provider: typesafe, model: jev-1.14.0, apiKeyEnv: JEV_API_KEY}
`
	if e := os.WriteFile(input, []byte(base), 0o600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("NINES_TRIAL_BASE_TENANTS", input)
	t.Setenv("NINES_PROVIDER_TENANTS_FILE", output)
	t.Setenv("NINES_TRIAL_TENANT", "mimir")
	t.Setenv("NEWRELIC_ACCOUNT_ID", "123456")
	if e := playgroundConfig(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(output)
	if e != nil {
		t.Fatal(e)
	}
	text := string(b)
	for _, expected := range []string{"kind: mimir", "jev-1.14.0", "name: datadog-trial", "name: newrelic-trial", "collectionMode: manual", "DD_API_KEY", "NEWRELIC_API_KEY"} {
		if !strings.Contains(text, expected) {
			t.Fatal("lost setting", expected)
		}
	}
	t.Setenv("NINES_DD_NATIVE_PUBLISH", "1")
	if e := playgroundConfig(); e != nil {
		t.Fatal(e)
	}
	native, e := os.ReadFile(output)
	if e != nil || !strings.Contains(string(native), "mode: native") || !strings.Contains(string(native), "namespace: provider-trial") {
		t.Fatal("native publication opt-in missing", e)
	}
	t.Setenv("NINES_NR_NATIVE_PUBLISH", "1")
	t.Setenv("NINES_NR_ENTITY_GUID", "synthetic-entity")
	if e := playgroundConfig(); e != nil {
		t.Fatal(e)
	}
	native, e = os.ReadFile(output)
	if e != nil || !strings.Contains(string(native), "entityGUID: synthetic-entity") || !strings.Contains(string(native), "newrelic-entity:") {
		t.Fatal("New Relic native opt-in missing", e)
	}
	t.Setenv("NINES_TRIAL_TENANT", "missing")
	if e := playgroundConfig(); e == nil {
		t.Fatal("unknown tenant accepted")
	}
}
