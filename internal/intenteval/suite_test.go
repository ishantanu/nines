package intenteval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentmodel"
)

func TestFixtureSuite(t *testing.T) {
	report, err := Fixtures(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.Passed != 105 || report.Failed != 0 || len(report.Results) != 105 || len(report.SuiteSHA256) != 64 {
		t.Fatalf("unexpected report: %+v", report)
	}
	counts := map[string]int{}
	for _, result := range report.Results {
		counts[result.Provider]++
		if !result.Passed || result.Model != "fixture-model" {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if len(counts) != 5 {
		t.Fatal(counts)
	}
	for provider, count := range counts {
		if count != 21 {
			t.Fatalf("%s: %d cases", provider, count)
		}
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"fixture-provider-key", "127.0.0.1", "fixtureGoal", "population", "Clarify success semantics"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("private fixture data in report: %s", private)
		}
	}
}

type scriptedDrafter struct {
	goal v1.ModelGoal
	err  error
}

func (d scriptedDrafter) Draft(context.Context, string) (v1.ModelGoal, error) { return d.goal, d.err }
func (scriptedDrafter) ModelID() string                                       { return "scripted-model" }

func TestEvaluationDetectsFailureAndRedactsProviderContent(t *testing.T) {
	cases, err := Cases()
	if err != nil {
		t.Fatal(err)
	}
	c := cases[0]
	var goal v1.ModelGoal
	if err := json.Unmarshal([]byte(c.FixtureGoal), &goal); err != nil {
		t.Fatal(err)
	}
	invented := goal
	inventedTarget := "99.99%"
	invented.TargetText = &inventedTarget
	for _, tc := range []struct {
		name    string
		drafter scriptedDrafter
		want    string
	}{
		{"provider error", scriptedDrafter{err: errors.New("private-provider-key-and-endpoint")}, "unexpected_status"},
		{"unrequested clarification", scriptedDrafter{goal: v1.ModelGoal{Outcome: "availability", Clarifications: []string{"private-model-response"}}}, "unexpected_status"},
		{"wrong quantity", scriptedDrafter{goal: invented}, "unexpected_status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Evaluate(context.Background(), c, "scripted", tc.drafter)
			if result.Passed || result.Failure != tc.want {
				t.Fatalf("unexpected result: %+v", result)
			}
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "private-") {
				t.Fatal("provider details leaked")
			}
		})
	}
	c.Expected.Target = .995
	result := Evaluate(context.Background(), c, "scripted", scriptedDrafter{goal: goal})
	if result.Passed || result.Failure != "unexpected_definition" {
		t.Fatalf("wrong typed value not detected: %+v", result)
	}
}

func TestCancelledSuiteCannotPass(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := Fixtures(ctx)
	if !errors.Is(err, context.Canceled) || report.Complete || len(report.Results) != 0 || Summary(report) == nil {
		t.Fatalf("cancelled suite passed: %+v %v", report, err)
	}
}

func TestLiveRequiresExplicitConfiguration(t *testing.T) {
	for _, env := range []map[string]string{
		{"ANTHROPIC_API_KEY": "private-key"},
		{"INTENT_MODEL_PROVIDER": "anthropic", "ANTHROPIC_API_KEY": "private-key"},
		{"INTENT_MODEL_MODEL": "fixture-model", "ANTHROPIC_API_KEY": "private-key"},
		{"INTENT_MODEL_PROVIDER": "custom", "INTENT_MODEL_MODEL": "fixture-model", "INTENT_MODEL_ENDPOINT": "https://private-key@example.invalid/v1/chat/completions"},
	} {
		report, err := Live(context.Background(), func(key string) string { return env[key] }, "")
		if err == nil || strings.Contains(err.Error(), "private-key") || report.Complete || len(report.Results) != 0 {
			t.Fatalf("invalid live configuration accepted or leaked: %+v %v", report, err)
		}
	}
}

func TestLiveCancellationAndFailureUseActualAdapter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cancel()
		http.Error(w, "private-provider-response", http.StatusUnauthorized)
	}))
	defer server.Close()
	env := map[string]string{"INTENT_MODEL_PROVIDER": "custom", "INTENT_MODEL_MODEL": "fixture-model", "INTENT_MODEL_ENDPOINT": server.URL + "/v1/chat/completions"}
	report, err := Live(ctx, func(key string) string { return env[key] }, "fixture-runtime")
	if !errors.Is(err, context.Canceled) || report.Complete || calls.Load() != 1 || report.Failed != 1 || report.Results[0].Passed {
		t.Fatalf("unexpected cancelled live result: %+v %v calls=%d", report, err, calls.Load())
	}
	data, encodeErr := json.Marshal(report)
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if strings.Contains(string(data), "private-provider-response") || strings.Contains(string(data), server.URL) {
		t.Fatal("provider content leaked")
	}
}

func TestLiveDraftCaseSelection(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		content := `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	env := map[string]string{"INTENT_MODEL_PROVIDER": "custom", "INTENT_MODEL_MODEL": "fixture", "INTENT_MODEL_ENDPOINT": server.URL + "/v1/chat/completions"}
	lookup := func(k string) string { return env[k] }
	for _, id := range []string{"unknown", "invented-quantity"} {
		report, err := Live(context.Background(), lookup, "fixture", id)
		if err == nil || report.Complete || calls.Load() != 0 {
			t.Fatal("invalid selection contacted provider", id, report, err)
		}
	}
	report, err := Live(context.Background(), lookup, "fixture", "availability")
	if err != nil || report.SelectedCase != "availability" || len(report.Results) != 1 || report.Passed != 1 || calls.Load() != 1 {
		t.Fatal(report, err)
	}
	if Summary(report) == nil {
		t.Fatal("filtered report must not count as full acceptance")
	}
}

// Replays synthetic local-model formatting through the real adapter and service.
// A joint goal can become a clarification; it must never become a draft.
func TestFencedSyntheticJointGoalRemainsReviewOnly(t *testing.T) {
	content := "```json\n" + `{"outcome":"compound","targetText":"99%","windowText":"28 days","thresholdText":"1 dollar","clarifications":["What percentage target should apply?"]}` + "\n```"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	c, err := intentmodel.NewConfigured(intentmodel.Config{Provider: "custom", Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", OutputMode: "json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	cases, err := Cases()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range cases {
		if item.ID == "agent-joint-conditions" {
			result := Evaluate(context.Background(), item, "custom", c)
			if !result.Passed || result.Status != "needs_clarification" {
				t.Fatal(result)
			}
			return
		}
	}
	t.Fatal("synthetic case missing")
}

// Reports must explain failed synthetic cases without recording model-produced
// text, extracted quantities, provider errors or credentials.
func TestDraftDiagnosticsAreBoundedAndRedacted(t *testing.T) {
	cases, err := Cases()
	if err != nil {
		t.Fatal(err)
	}
	c := cases[0]
	var original v1.ModelGoal
	if err = json.Unmarshal([]byte(c.FixtureGoal), &original); err != nil {
		t.Fatal(err)
	}
	secret := "private-model-response-and-secret"
	invented := "private-invented-quantity"
	modelClarification := original
	modelClarification.Clarifications = []string{secret}
	inventedGoal := original
	inventedGoal.TargetText = &invented
	for _, tc := range []struct {
		name               string
		drafter            scriptedDrafter
		state, evidence    string
		modelClarification bool
		reason             string
	}{
		{"unavailable", scriptedDrafter{err: errors.New(secret)}, "unavailable", "", false, ""},
		{"clarification", scriptedDrafter{goal: modelClarification}, "returned", "appears_in_input", true, ""},
		{"invented", scriptedDrafter{goal: inventedGoal}, "returned", "not_in_input", false, "target_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Evaluate(context.Background(), c, "fixture", tc.drafter)
			diagnostic := result.DraftDiagnostic
			if result.Passed || diagnostic == nil || diagnostic.ModelState != tc.state || diagnostic.TargetEvidence != tc.evidence || diagnostic.ModelClarification != tc.modelClarification {
				t.Fatal(result)
			}
			if tc.reason != "" && (len(diagnostic.ValidationReasons) != 1 || diagnostic.ValidationReasons[0] != tc.reason) {
				t.Fatal(diagnostic)
			}
			raw, e := json.Marshal(result)
			if e != nil {
				t.Fatal(e)
			}
			for _, forbidden := range []string{secret, invented, c.Text, "99.9%", "28 days"} {
				if strings.Contains(string(raw), forbidden) {
					t.Fatal("private model or input text appeared in report")
				}
			}
		})
	}
}
