package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestDefinitionsUseSharedTemplatesAndExplicitEvaluationCoverage(t *testing.T) {
	defs := definitions()
	if len(defs) != 10 {
		t.Fatal("expected nine templates and separate evaluation coverage")
	}
	seen := map[string]bool{}
	for _, d := range defs {
		if e := model.SLODefinition(d.SLO).Validate(); e != nil {
			t.Fatal(d.SLO.Metadata.Name, e)
		}
		if seen[d.SLO.Metadata.Name] || d.SLO.Spec.Lifecycle != v1.LifecycleExperimental || len(d.SLO.Spec.Policies) != 0 || d.SLO.Spec.Measurement.ScrapeIntervalSeconds != 5 {
			t.Fatal("invalid fixture contract")
		}
		seen[d.SLO.Metadata.Name] = true
	}
	j := journey()
	body, e := yaml.Marshal(j)
	if e != nil {
		t.Fatal(e)
	}
	var parsed model.Journey
	if yaml.Unmarshal(body, &parsed) != nil || parsed.Validate() != nil {
		t.Fatal("invalid journey")
	}
	for _, step := range j.Spec.Steps {
		for _, id := range step.SLORefs {
			if !seen[id] {
				t.Fatal("dangling SLO link", id)
			}
		}
	}
	var out bytes.Buffer
	if e = run(context.Background(), []string{"render"}, &out); e != nil {
		t.Fatal(e)
	}
	decoder := yaml.NewDecoder(&out)
	docs := 0
	for {
		var doc yaml.Node
		e = decoder.Decode(&doc)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		docs++
	}
	if docs != 11 {
		t.Fatal("missing rendered resources", docs)
	}
}

func ratio(w *workload, d objective) float64 {
	m := d.SLO.Spec.Measurement
	if m.Type == "latency" {
		h := w.durations
		if d.SLO.Metadata.Name == "ai-inference-ttft" {
			h = w.firstToken
		}
		for i, b := range boundaries {
			if b == *m.ThresholdSeconds {
				return float64(h.buckets[i]) / float64(h.count)
			}
		}
		return math.NaN()
	}
	var total uint64
	for key, value := range w.counters {
		if strings.HasPrefix(key, m.Metric+"{") {
			total += value
		}
	}
	var good uint64
	for key, value := range m.SuccessLabels {
		good = w.counters[series(m.Metric, m.Labels, key, value)]
	}
	return float64(good) / float64(total)
}
func TestSyntheticMeasurementSemantics(t *testing.T) {
	for _, scenario := range []string{"healthy", "degraded", "inference-failure"} {
		t.Run(scenario, func(t *testing.T) {
			w := newWorkload()
			w.scenario = scenario
			w.tick()
			for _, d := range definitions() {
				expected := 1.0
				if scenario == "degraded" {
					expected = d.Degraded
				}
				if scenario == "inference-failure" && (d.SLO.Metadata.Name == "ai-inference-availability" || d.SLO.Metadata.Name == "ai-inference-latency" || d.SLO.Metadata.Name == "ai-llm-responses") {
					expected = .8
				}
				if observed := ratio(w, d); math.Abs(observed-expected) > 1e-9 {
					t.Fatalf("%s: %g, want %g", d.SLO.Metadata.Name, observed, expected)
				}
			}
			if scenario == "inference-failure" && w.firstToken.count != 80 {
				t.Fatal("TTFT must exclude streams never reaching a first token")
			}
		})
	}
	w := newWorkload()
	w.scenario = "no-traffic"
	w.tick()
	for _, d := range definitions() {
		if !math.IsNaN(ratio(w, d)) {
			t.Fatal("zero traffic appeared healthy")
		}
	}
	w.scenario = "degraded"
	w.tick()
	var before, after bytes.Buffer
	w.metrics(&before)
	w.scenario = "healthy"
	w.tick()
	w.metrics(&after)
	for key, previous := range w.counters {
		if previous == 0 {
			continue
		}
		if !strings.Contains(after.String(), key+" ") {
			t.Fatal("scenario change removed a series")
		}
	}
	if strings.Contains(after.String(), "NaN") || strings.Contains(after.String(), "prompt=") || strings.Contains(after.String(), "task_id=") {
		t.Fatal("invalid or unbounded instrumentation")
	}
}
func TestScenarioControlAndConcurrentScrapes(t *testing.T) {
	w := newWorkload()
	server := httptest.NewServer(w.handler())
	defer server.Close()
	for _, name := range scenarios {
		response, e := http.Post(server.URL+"/scenario?name="+name, "text/plain", nil)
		if e != nil {
			t.Fatal(e)
		}
		_ = response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal(name, response.StatusCode)
		}
	}
	for _, path := range []string{"/scenario?name=unsupported", "/scenario?name=healthy&token=secret", "/scenario?name="} {
		response, e := http.Post(server.URL+path, "text/plain", nil)
		if e != nil {
			t.Fatal(e)
		}
		_ = response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("invalid scenario accepted")
		}
	}
	w.scenario = "healthy"
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Go(func() {
			for j := 0; j < 20; j++ {
				w.tick()
				var out bytes.Buffer
				w.metrics(&out)
			}
		})
	}
	workers.Wait()
}
func TestFixtureEndpointRestrictions(t *testing.T) {
	for _, raw := range []string{"http://metadata.invalid", "http://169.254.169.254", "http://localhost@attacker.invalid", "http://localhost?token=secret", "http://localhost/production", "https://localhost", "http://localhost#private"} {
		if fixtureURL(raw, "localhost", "") == nil {
			t.Fatal("unsafe fixture origin accepted", raw)
		}
	}
	for _, raw := range []string{"http://127.0.0.1:18080", "http://[::1]:18080", "http://localhost:18080"} {
		if e := fixtureURL(raw, "localhost", ""); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSetupPreservesExistingResourcesAndCancellationRestoresScenario(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	mutations := 0
	changes := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/tenants":
			_, _ = io.WriteString(w, `[{"id":"prometheus"}]`)
		case r.URL.Path == "/scenario" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"scenario":"task-failure"}`)
		case r.URL.Path == "/scenario" && r.Method == http.MethodPost:
			mu.Lock()
			changes = append(changes, r.URL.Query().Get("name"))
			mu.Unlock()
			_, _ = io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/integration"):
			cancel()
			http.Error(w, "not ready", 404)
		case r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"prometheus"}`)
		default:
			mu.Lock()
			mutations++
			mu.Unlock()
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	config := settings{nines: server.URL, sample: server.URL}
	if e := config.setup(context.Background(), io.Discard); e != nil {
		t.Fatal(e)
	}
	if len(config.tenants) != 1 || config.tenants[0] != "prometheus" || mutations != 0 {
		t.Fatal("setup changed an existing resource")
	}
	if e := config.verify(ctx, io.Discard); e == nil {
		t.Fatal("incomplete verification passed")
	}
	mu.Lock()
	defer mu.Unlock()
	data, _ := json.Marshal(changes)
	if string(data) != `["healthy","task-failure"]` {
		t.Fatal("previous scenario not restored on cancellation", string(data))
	}
}
