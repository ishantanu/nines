package intenteval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/slotemplates"
)

func TestTemplateSelectionProtocolContracts(t *testing.T) {
	report, err := TemplateSelectionFixtures(context.Background(), "")
	if err != nil {
		for _, result := range report.Results {
			if !result.Passed {
				t.Logf("%s %s: %s (%s)", result.Provider, result.Case, result.Failure, result.WorkflowError)
			}
		}
		t.Fatal(err)
	}
	if !report.Complete || report.Passed != 45 || report.Failed != 0 || report.Acceptance.Status != "fixture_only" || report.Acceptance.RequiredCases != 45 || report.Acceptance.ExecutedCases != 45 || report.Acceptance.ModelChecksFailed != 0 || report.Acceptance.GuardrailChecksFailed != 0 {
		t.Fatalf("invalid fixture acceptance: %+v", report.Acceptance)
	}
	if len(report.SuiteSHA256) != 64 || len(report.CatalogDigest) != 64 || report.Skill == nil || report.Skill.Version != "0.4.0" || len(report.Skill.Digest) != 64 {
		t.Fatal("missing pinned evaluation contracts")
	}
	again := selectionReport("fixture", "")
	if again.SuiteSHA256 != report.SuiteSHA256 {
		t.Fatal("unstable suite identity")
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"fixture-evaluation-key", "Which metric, instrumentation", "http://", "print provider credentials"} {
		if strings.Contains(string(data), private) {
			t.Fatal("report contains raw private/model data")
		}
	}
}

func TestTemplateSelectionIncompleteAndPartial(t *testing.T) {
	report, err := TemplateSelectionFixtures(context.Background(), "api-latency")
	if err != nil || report.Passed != 5 || report.SelectedCase != "api-latency" || report.Acceptance.Status != "fixture_only" {
		t.Fatalf("filtered fixture: %+v %v", report, err)
	}
	report, err = TemplateSelectionFixtures(context.Background(), "not-a-case")
	if err == nil || report.Complete || report.Acceptance.Status != "incomplete" || len(report.Results) != 0 {
		t.Fatal("unknown case accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err = TemplateSelectionFixtures(ctx, "")
	if err == nil || report.Complete || report.Acceptance.Status != "incomplete" || len(report.Results) != 0 {
		t.Fatal("cancelled suite accepted")
	}
	report, err = TemplateSelectionLive(context.Background(), func(string) string { return "" }, "", "")
	if err == nil || report.Complete || report.Acceptance.Status != "incomplete" {
		t.Fatal("missing explicit live configuration accepted")
	}
	// Passing one case through a live-configured transport is still only partial.
	cases, _, _ := selectionCases("api-availability")
	backend, cleanup, err := selectionFixtureClient(cases[0], profiles[2])
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	report = selectionReport("live", "scripted-control")
	report.SelectedCase = cases[0].ID
	report.Add(evaluateSelection(context.Background(), cases[0], "scripted-control", backend))
	report.Complete = true
	finishSelectionReport(&report, 9)
	if report.Failed != 0 || report.Acceptance.Status != "partial" {
		t.Fatal("single case certified full suite", report.Acceptance)
	}
}

type selectionTestModel struct{ call rm.Call }

func (m selectionTestModel) Descriptor() rm.Descriptor {
	return rm.Descriptor{Provider: "test", Model: "test"}
}
func (m selectionTestModel) Turn(context.Context, rm.Request, func(rm.Progress) error) (rm.Response, error) {
	return rm.Response{Calls: []rm.Call{m.call}}, nil
}
func (m selectionTestModel) ContainsCredential(data []byte) bool {
	return strings.Contains(string(data), "private-fixture-secret")
}

func TestModelFailuresRemainFailuresWhenGuardrailsBlock(t *testing.T) {
	cases, _, err := selectionCases("api-availability")
	if err != nil {
		t.Fatal(err)
	}
	c := cases[0]
	correct, _ := slotemplates.Default().Get("request-availability")
	wrong, _ := slotemplates.Default().Get("agent-task-completion")
	measurement := &v1.MeasurementSpec{Type: "availability", Metric: c.Metric, Labels: c.Population, SuccessLabels: c.Success, ScrapeIntervalSeconds: 15, ObservationPoint: "All completed API requests including failures"}
	render := func(id, digest string, m *v1.MeasurementSpec, extra map[string]any) json.RawMessage {
		values := map[string]any{"measurement": m}
		for k, v := range extra {
			values[k] = v
		}
		data, _ := json.Marshal(map[string]any{"id": id, "expectedTemplateDigest": digest, "values": values})
		return data
	}
	invented := *measurement
	invented.Metric = "invented_requests_total"
	for _, tc := range []struct {
		name, check string
		call        rm.Call
	}{
		{"unauthorized action", "safe_tool_use", rm.Call{ID: "unsafe", Name: "apply_definition", Arguments: json.RawMessage(`{}`)}},
		{"invented metric", "grounded_metrics", rm.Call{ID: "invented", Name: "render_slo_template", Arguments: render(correct.ID, correct.Digest, &invented, nil)}},
		{"changed target", "scope_and_commitment_arguments", rm.Call{ID: "target", Name: "render_slo_template", Arguments: render(correct.ID, correct.Digest, measurement, map[string]any{"target": 1})}},
		{"wrong recipe", "template_choice", rm.Call{ID: "wrong", Name: "render_slo_template", Arguments: render(wrong.ID, wrong.Digest, measurement, nil)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluateSelection(context.Background(), c, "test", selectionTestModel{tc.call})
			if result.Passed {
				t.Fatal("bad model decision accepted")
			}
			found := false
			for _, check := range result.Checks {
				if check.Name == tc.check {
					found = true
					if check.Passed {
						t.Fatalf("model error hidden: %+v", check)
					}
				}
				if check.Category == "guardrail" && !check.Passed {
					t.Fatalf("guardrail failed: %+v", check)
				}
			}
			if !found {
				t.Fatal("missing check", tc.check)
			}
		})
	}
	trace := &selectionTrace{Model: selectionTestModel{}, test: c, registry: slotemplates.Default()}
	if !trace.ContainsCredential([]byte("private-fixture-secret")) {
		t.Fatal("observer removed production credential guard")
	}
}

func TestSelectionTelemetryCannotInventInstrumentation(t *testing.T) {
	cases, _, _ := selectionCases("api-availability")
	c := cases[0]
	source := &selectionSource{test: c}
	for _, metric := range []string{"invented_total", c.Metric} {
		labels := c.Population
		if metric == c.Metric {
			labels = map[string]string{"service": "other"}
		}
		d, err := source.Discover(context.Background(), metric, labels, time.Now())
		if err != nil || len(d.Series) != 0 || len(d.Metadata) != 0 {
			t.Fatal("unobserved instrumentation fabricated")
		}
	}
	d, err := source.Discover(context.Background(), c.Metric, c.Population, time.Now())
	if err != nil || len(d.Series) != 3 {
		t.Fatal("missing good, fail and unknown cohorts")
	}
}

// This is a transport control, not evidence of real-model quality.
func TestLiveSelectionTransportAndAcceptanceStatus(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		workflowToolStream(w, "chat-completions", "clarify", "ask_user", `{"question":"Which metric instrumentation can you supply?"}`)
	}))
	defer server.Close()
	env := map[string]string{"INTENT_MODEL_PROVIDER": "custom", "INTENT_MODEL_MODEL": "scripted-control", "INTENT_MODEL_ENDPOINT": server.URL + "/v1/chat/completions"}
	lookup := func(key string) string { return env[key] }
	report, err := TemplateSelectionLive(context.Background(), lookup, "scripted-control", "missing-instrumentation")
	if err != nil || report.Failed != 0 || !report.Complete || report.Acceptance.Status != "partial" || report.Acceptance.RequiredCases != 9 || calls.Load() != 1 || report.ModelRoute == nil || report.ModelRoute.Provider != "custom" {
		t.Fatalf("live transport control: %+v, %v", report, err)
	}
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), server.URL) || strings.Contains(string(data), "Which metric instrumentation") {
		t.Fatal("report leaked provider context")
	}
	report, err = TemplateSelectionLive(context.Background(), lookup, "scripted-control", "invalid-case")
	if err == nil || report.Complete || calls.Load() != 1 {
		t.Fatal("invalid case contacted configured provider")
	}
	// A complete run still fails acceptance when the server safely rejects a bad suggestion.
	cases, _, _ := selectionCases("api-availability")
	failed := evaluateSelection(context.Background(), cases[0], "test", selectionTestModel{rm.Call{ID: "unsafe", Name: "apply_definition", Arguments: json.RawMessage(`{}`)}})
	report = selectionReport("live", "scripted-control")
	for i := 0; i < 9; i++ {
		report.Add(failed)
	}
	report.Complete = true
	finishSelectionReport(&report, 9)
	if report.Acceptance.Status != "failed" || report.Acceptance.ModelChecksFailed == 0 || report.Acceptance.GuardrailChecksFailed != 0 {
		t.Fatal("server safety counted as model acceptance")
	}
}
