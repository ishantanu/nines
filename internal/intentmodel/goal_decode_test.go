package intentmodel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoalRejectsDuplicateAndEscapedDuplicateFields(t *testing.T) {
	for _, content := range []string{
		`{"outcome":"availability","targetText":"99.5%","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`,
		`{"outcome":"availability","targetText":"99.5%","\u0074argetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`,
		`{"outcome":"latency","outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`,
		`{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":["Clarify the measurement"],"clarifications":[]}`,
	} {
		if _, err := decodeGoal(content); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("accepted ambiguous structured goal: %v", err)
		}
	}
}

func TestGoalRejectsNullOrBlankClarificationItems(t *testing.T) {
	for _, items := range []string{`[null]`, `["Which target?",null]`, `[""]`, `["   "]`} {
		content := `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":` + items + `}`
		if _, err := decodeGoal(content); !errors.Is(err, ErrUnavailable) {
			t.Errorf("accepted invalid clarification array %s: %v", items, err)
		}
	}
}

func TestJSONFenceRequiresOneCompleteBlockAndStrictGoal(t *testing.T) {
	valid := `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`
	for _, input := range []string{valid, "```json\n" + valid + "\n```", " \n```\r\n" + valid + "\r\n``` \n"} {
		unwrapped, err := unwrapJSONFence(input)
		if err != nil {
			t.Fatalf("valid JSON wrapper rejected: %v", err)
		}
		if _, err = decodeGoal(unwrapped); err != nil {
			t.Fatalf("valid goal rejected: %v", err)
		}
	}
	for _, input := range []string{
		"```python\n" + valid + "\n```",
		"```json\n" + valid + "\n```\nextra",
		"```json\n" + valid + "\n```\n```json\n" + valid + "\n```",
		"```json\n" + valid,
		"prose\n```json\n" + valid + "\n```",
		"```json\n" + `{"outcome":"availability","outcome":"latency","targetText":"99%","windowText":"7 days","thresholdText":null,"clarifications":[]}` + "\n```",
	} {
		unwrapped, err := unwrapJSONFence(input)
		if err == nil {
			_, err = decodeGoal(unwrapped)
		}
		if !errors.Is(err, ErrUnavailable) {
			t.Fatal("accepted ambiguous model output", input)
		}
	}
}

// JSON mode alone accepts a single fence; schema mode retains its strict wire contract.
func TestFencedJSONIsLimitedToPortableJSONMode(t *testing.T) {
	content := "```json\n" + `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}` + "\n```"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	for _, tc := range []struct {
		mode     string
		succeeds bool
	}{{"json", true}, {"schema", false}} {
		client, err := NewConfigured(Config{Provider: "custom", Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", OutputMode: tc.mode}, "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Draft(context.Background(), "99.9% success over 28 days")
		if (err == nil) != tc.succeeds {
			t.Fatalf("mode %s: %v", tc.mode, err)
		}
	}
}
