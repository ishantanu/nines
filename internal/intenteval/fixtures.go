package intenteval

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	"github.com/ishantanu/nines/internal/intentmodel"
)

type fixtureProfile struct{ name, provider, style, output, path string }

var profiles = []fixtureProfile{
	{"anthropic-contract", "anthropic", "anthropic", "schema", "/v1/messages"},
	{"gemini-contract", "gemini", "gemini", "schema", "/models/fixture-model:generateContent"},
	{"openai-compatible-contract", "custom", "chat-completions", "schema", "/v1/chat/completions"},
	{"ollama-contract", "ollama", "chat-completions", "schema", "/v1/chat/completions"},
	{"vllm-json-contract", "vllm", "chat-completions", "json", "/v1/chat/completions"},
}

// Fixtures exercises actual HTTP adapters and deterministic draft validation.
// Scripted responses are contract tests, not evidence of real model quality.
func Fixtures(ctx context.Context) (Report, error) {
	report := NewReport("fixture", "")
	cases, err := Cases()
	if err != nil {
		return report, err
	}
	for _, profile := range profiles {
		for _, c := range cases {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			result, err := fixtureCase(ctx, c, profile)
			if err != nil {
				return report, err
			}
			report.Add(result)
		}
	}
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	report.Complete = true
	return report, Summary(report)
}
func fixtureCase(ctx context.Context, c Case, profile fixtureProfile) (Result, error) {
	var requests atomic.Int32
	var invalid atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		data, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		var body map[string]any
		if err != nil || len(data) > 65536 || json.Unmarshal(data, &body) != nil || r.Method != "POST" || r.URL.Path != profile.path {
			invalid.Store(true)
		}
		// Scope, catalogs, telemetry and identity must never enter the model input.
		for _, private := range []string{"evaluation-draft", "\"population\"", "fixture-provider-key"} {
			if strings.Contains(string(data), private) {
				invalid.Store(true)
			}
		}
		var text string
		switch profile.style {
		case "anthropic":
			messages, _ := body["messages"].([]any)
			if len(messages) == 1 {
				message, _ := messages[0].(map[string]any)
				text, _ = message["content"].(string)
			}
		case "gemini":
			contents, _ := body["contents"].([]any)
			if len(contents) == 1 {
				content, _ := contents[0].(map[string]any)
				parts, _ := content["parts"].([]any)
				if len(parts) == 1 {
					part, _ := parts[0].(map[string]any)
					text, _ = part["text"].(string)
				}
			}
		default:
			messages, _ := body["messages"].([]any)
			if len(messages) == 2 {
				message, _ := messages[1].(map[string]any)
				text, _ = message["content"].(string)
			}
		}
		if text != c.Text {
			invalid.Store(true)
		}
		var response any
		switch profile.style {
		case "anthropic":
			response = map[string]any{"type": "message", "role": "assistant", "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": c.FixtureGoal}}}
		case "gemini":
			response = map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": c.FixtureGoal}}}}}}
		default:
			response = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": c.FixtureGoal}}}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			invalid.Store(true)
		}
	}))
	defer server.Close()
	client, err := intentmodel.NewConfigured(intentmodel.Config{Provider: profile.provider, Endpoint: server.URL + profile.path, Model: "fixture-model", APIStyle: profile.style, OutputMode: profile.output}, "fixture-provider-key")
	if err != nil {
		return Result{}, errors.New("fixture model configuration failed")
	}
	result := Evaluate(ctx, c, profile.name, client)
	if requests.Load() != 1 || invalid.Load() {
		result.Passed = false
		result.Failure = "model_request_contract_failed"
	}
	return result, nil
}
