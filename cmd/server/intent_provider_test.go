package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/httpapi"
)

func clearIntentProviderEnvironment(t *testing.T) {
	t.Helper()
	for _, k := range []string{"INTENT_MODEL_PROVIDER", "INTENT_MODEL_MODEL", "INTENT_MODEL_ENDPOINT", "INTENT_MODEL_API_KEY_ENV", "INTENT_MODEL_API_STYLE", "INTENT_MODEL_OUTPUT_MODE", "ANTHROPIC_API_KEY", "ATHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "MISTRAL_API_KEY", "GROQ_API_KEY", "DEEPSEEK_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY", "XAI_API_KEY", "MIMIR_URL", "PROMETHEUS_URL", "ALERT_WEBHOOK_URL"} {
		t.Setenv(k, "")
	}
}
func TestNativeAnthropicEnvironmentBootstrapThroughHTTP(t *testing.T) {
	clearIntentProviderEnvironment(t)
	calls := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "local-test-anthropic-key" || r.Header.Get("Authorization") != "" {
			t.Error("wrong provider route or secret")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "message", "role": "assistant", "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`}}})
	}))
	defer modelServer.Close()
	t.Setenv("ANTHROPIC_API_KEY", "local-test-anthropic-key")
	// Only the endpoint is overridden for a local fixture. The provider is inferred
	// from ANTHROPIC_API_KEY when no provider selector is set.
	t.Setenv("INTENT_MODEL_ENDPOINT", modelServer.URL+"/v1/messages")
	t.Setenv("PROMETHEUS_URL", modelServer.URL)
	runtimes, err := initTenants(context.Background(), "", false, true, "local-owner-token-more-than-24-characters")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range runtimes {
			r.Close()
		}
	}()
	if calls != 0 {
		t.Fatal("bootstrap called the provider")
	}
	h, err := httpapi.NewMulti([]httpapi.Workspace{runtimes[0].Workspace}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/intents/draft", strings.NewReader(`{"text":"99.9% success over 28 days","metadata":{"name":"api-success","project":"test"},"service":"api","source":"primary","population":{"service":"api"},"noDataPolicy":"unknown"}`))
	request.Header.Set("Authorization", "Bearer local-owner-token-more-than-24-characters")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Nines-Tenant", "default")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var draft v1.IntentDraft
	if err = json.Unmarshal(response.Body.Bytes(), &draft); err != nil || draft.Status != "draft" || draft.Intent == nil || draft.Intent.Spec.Target != .999 || draft.Intent.Spec.WindowSeconds != 28*86400 || calls != 1 {
		t.Fatal(draft, err, calls)
	}
	rows, err := runtimes[0].Workspace.Service.Store.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("draft wrote an SLO", err)
	}
}
func TestDemoIgnoresProviderEnvironment(t *testing.T) {
	clearIntentProviderEnvironment(t)
	t.Setenv("ANTHROPIC_API_KEY", "demo-must-not-transmit")
	t.Setenv("GEMINI_API_KEY", "ambiguous-must-not-be-evaluated")
	runtimes, err := initTenants(context.Background(), "", true, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range runtimes {
			r.Close()
		}
	}()
	for _, r := range runtimes {
		if r.Workspace.Service.IntentDrafter != nil {
			t.Fatal("demo enabled an external model")
		}
	}
}
