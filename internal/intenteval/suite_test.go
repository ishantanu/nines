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
