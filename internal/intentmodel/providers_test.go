package intentmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validGoal = `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}
func TestProviderKeyDetection(t *testing.T) {
	for _, p := range presets {
		for _, name := range p.keys {
			t.Run(name, func(t *testing.T) {
				c, err := FromEnvironment(nil, env(map[string]string{name: "fixture-secret"}))
				if err != nil || c == nil {
					t.Fatal(err)
				}
				if c.provider != p.provider || c.key != "fixture-secret" || c.model == "" || c.apiStyle != p.style {
					t.Fatal("incorrect provider selection")
				}
				if p.provider != "openai" && strings.Contains(c.endpoint, "api.openai.com") {
					t.Fatal("routed provider to OpenAI")
				}
			})
		}
	}
	for _, values := range []map[string]string{
		{"ANTHROPIC_API_KEY": "first-secret", "GEMINI_API_KEY": "second-secret"},
		{"ANTHROPIC_API_KEY": "first-secret", "ATHROPIC_API_KEY": "different-secret"},
		{"GEMINI_API_KEY": "first-secret", "GOOGLE_API_KEY": "different-secret"},
	} {
		if _, err := FromEnvironment(nil, env(values)); err == nil || strings.Contains(err.Error(), "first-secret") || strings.Contains(err.Error(), "second-secret") || strings.Contains(err.Error(), "different-secret") {
			t.Fatal("ambiguous routing or leaked credential", err)
		}
	}
	c, err := FromEnvironment(nil, env(nil))
	if err != nil || c != nil {
		t.Fatal(c, err)
	}
	c, err = FromEnvironment(nil, env(map[string]string{"ANTHROPIC_API_KEY": "same", "ATHROPIC_API_KEY": "same"}))
	if err != nil || c.provider != "anthropic" {
		t.Fatal(err)
	}
	c, err = FromEnvironment(nil, env(map[string]string{"INTENT_MODEL_PROVIDER": "anthropic", "ANTHROPIC_API_KEY": "selected", "OPENAI_API_KEY": "unrelated"}))
	if err != nil || c.provider != "anthropic" || c.key != "selected" {
		t.Fatal(err)
	}
}
func TestExplicitTenantOverridesGlobalProvider(t *testing.T) {
	lookup := env(map[string]string{"INTENT_MODEL_PROVIDER": "anthropic", "ANTHROPIC_API_KEY": "global-secret", "TENANT_MODEL_KEY": "tenant-secret"})
	c, err := FromEnvironment(&Config{Provider: "gemini", APIKeyEnv: "TENANT_MODEL_KEY", Model: "tenant-gemini"}, lookup)
	if err != nil || c.provider != "gemini" || c.model != "tenant-gemini" || c.key != "tenant-secret" {
		t.Fatal(err)
	}
	c, err = FromEnvironment(&Config{Provider: "none"}, lookup)
	if err != nil || c != nil {
		t.Fatal("tenant disable ignored", err)
	}
	c, err = FromEnvironment(&Config{Endpoint: "http://127.0.0.1:11434/v1/chat/completions", Model: "qwen3:8b"}, lookup)
	if err != nil || c.key != "" || c.apiStyle != "chat-completions" {
		t.Fatal("local endpoint inherited credentials", err)
	}
	c, err = FromEnvironment(&Config{APIKeyEnv: "ANTHROPIC_API_KEY"}, lookup)
	if err != nil || c.provider != "anthropic" {
		t.Fatal("key reference did not select its native provider", err)
	}
	for _, cfg := range []Config{{Provider: "unknown"}, {Provider: "anthropic", APIKeyEnv: "MISSING"}, {Provider: "custom", Endpoint: "http://localhost:8000/v1/chat/completions"}, {Provider: "gemini", APIStyle: "openai-compatible"}, {Provider: "custom", Model: "m", Endpoint: "https://models.example/v1/messages", APIStyle: "unknown"}} {
		if _, err := FromEnvironment(&cfg, lookup); err == nil {
			t.Fatalf("accepted invalid config: %+v", cfg)
		}
	}
	_, err = FromEnvironment(&Config{Provider: "anthropic"}, env(map[string]string{"ANTHROPIC_API_KEY": "secret\r\nInjected: yes"}))
	if err == nil || strings.Contains(err.Error(), "Injected") {
		t.Fatal("invalid credential accepted or leaked", err)
	}
}
func TestNativeProviderRequestsAndResponses(t *testing.T) {
	for _, provider := range []string{"anthropic", "gemini"} {
		for _, mode := range []string{"valid", "thinking", "refused", "truncated", "tool", "missing", "extra", "multi", "upstream", "redirect"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
						t.Error("wrong method or credentials")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["response_format"] != nil || body["store"] != nil || body["max_completion_tokens"] != nil {
						t.Error("OpenAI-only fields reached native provider")
					}
					if mode == "upstream" {
						w.WriteHeader(429)
						_, _ = fmt.Fprint(w, "fixture-secret")
						return
					}
					if mode == "redirect" {
						http.Redirect(w, r, "/not-followed", http.StatusTemporaryRedirect)
						return
					}
					goal := validGoal
					if mode == "extra" {
						goal = strings.TrimSuffix(goal, "}") + `,"query":"vector(1)"}`
					}
					if mode == "missing" {
						goal = `{"outcome":"availability"}`
					}
					if provider == "anthropic" {
						if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "fixture-secret" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("x-goog-api-key") != "" {
							t.Error("wrong Anthropic authentication")
						}
						if body["system"] != prompt || body["model"] != "model-fixture" || body["output_config"] == nil || body["max_tokens"] != float64(2048) || len(body) != 5 {
							t.Error("wrong Anthropic payload")
						}
						messages, _ := body["messages"].([]any)
						if len(messages) != 1 || messages[0].(map[string]any)["content"] != "99.9% success over 28 days" {
							t.Error("scope text changed")
						}
						reason := "end_turn"
						content := []any{map[string]any{"type": "text", "text": goal}}
						switch mode {
						case "thinking":
							content = append([]any{map[string]any{"type": "thinking", "thinking": "internal"}}, content...)
						case "refused":
							reason = "refusal"
						case "truncated":
							reason = "max_tokens"
						case "tool":
							content = []any{map[string]any{"type": "tool_use", "name": "deploy", "input": map[string]any{}}}
						case "multi":
							content = append(content, content[0])
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"type": "message", "role": "assistant", "stop_reason": reason, "content": content})
					} else {
						if r.URL.Path != "/v1beta/models/model-fixture:generateContent" || r.Header.Get("x-goog-api-key") != "fixture-secret" || r.Header.Get("x-api-key") != "" {
							t.Error("wrong Gemini authentication")
						}
						generation, _ := body["generationConfig"].(map[string]any)
						if generation["responseJsonSchema"] == nil || generation["responseMimeType"] != "application/json" || len(body) != 3 {
							t.Error("wrong Gemini payload")
						}
						finish := "STOP"
						parts := []any{map[string]any{"text": goal}}
						response := map[string]any{}
						switch mode {
						case "thinking":
							parts = append([]any{map[string]any{"thought": true, "text": "internal"}}, parts...)
						case "refused":
							response["promptFeedback"] = map[string]any{"blockReason": "SAFETY"}
						case "truncated":
							finish = "MAX_TOKENS"
						case "tool":
							parts = []any{map[string]any{"functionCall": map[string]any{"name": "deploy"}}}
						case "multi":
							parts = append(parts, parts[0])
						}
						response["candidates"] = []any{map[string]any{"finishReason": finish, "content": map[string]any{"role": "model", "parts": parts}}}
						_ = json.NewEncoder(w).Encode(response)
					}
				}))
				defer upstream.Close()
				path := "/v1/messages"
				if provider == "gemini" {
					path = "/v1beta/models/{model}:generateContent"
				}
				c, err := NewConfigured(Config{Provider: provider, Endpoint: upstream.URL + path, Model: "model-fixture"}, "fixture-secret")
				if err != nil {
					t.Fatal(err)
				}
				goal, err := c.Draft(context.Background(), "99.9% success over 28 days")
				if mode == "valid" || mode == "thinking" {
					if err != nil || goal.TargetText == nil || *goal.TargetText != "99.9%" {
						t.Fatal(goal, err)
					}
				} else if err == nil || strings.Contains(err.Error(), "fixture-secret") {
					t.Fatal("accepted bad response or leaked credential", err)
				}
				if calls != 1 {
					t.Fatal("retried or followed redirect", calls)
				}
			})
		}
	}
}
func TestPortableJSONModeRetainsStrictValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		format := body["response_format"].(map[string]any)
		messages := body["messages"].([]any)
		if format["type"] != "json_object" || !strings.Contains(messages[0].(map[string]any)["content"].(string), `"targetText"`) || strings.Contains(messages[0].(map[string]any)["content"].(string), "State a percentage and rolling duration.") || r.Header.Get("Authorization") != "Bearer custom-key" {
			t.Error("JSON mode is not grounded in the shared schema")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": validGoal}}}})
	}))
	defer server.Close()
	c, err := NewConfigured(Config{Provider: "custom", Endpoint: server.URL + "/v1/chat/completions", Model: "any-model", OutputMode: "json"}, "custom-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Draft(context.Background(), "99.9% success over 28 days"); err != nil {
		t.Fatal(err)
	}
}
