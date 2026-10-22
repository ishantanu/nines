package intentmodel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func TestModelCredentialEchoNeverBecomesDraftOrToolArguments(t *testing.T) {
	const key = "fixture-provider-key-0123456789abcdef"
	for _, mode := range []string{"draft", "tool"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Deliberately JSON-escape the credential inside the nested arguments string.
				escaped := strings.ReplaceAll(key, "f", `\u0066`)
				content := `{"outcome":"availability","targetText":null,"windowText":null,"thresholdText":null,"clarifications":["` + escaped + `"]}`
				message := map[string]any{"role": "assistant", "content": content}
				finish := "stop"
				if mode == "tool" {
					message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call-one", "type": "function", "function": map[string]string{"name": "ask_user", "arguments": `{"question":"` + escaped + `"}`}}}}
					finish = "tool_calls"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": message}}})
			}))
			defer server.Close()
			client, err := NewConfigured(Config{Provider: "custom", Endpoint: server.URL + "/chat/completions", Model: "fixture-model"}, key)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "draft" {
				if _, err := client.Draft(context.Background(), "99.9% over 28 days"); !errors.Is(err, ErrUnavailable) {
					t.Fatal("credential echo accepted", err)
				}
			} else {
				_, err := client.Turn(context.Background(), rm.Request{MaxOutputTokens: 1024, Messages: []rm.Message{{Role: "user", Text: "clarify"}}, Tools: []rm.Tool{{Name: "ask_user", Schema: json.RawMessage(`{"type":"object"}`)}}}, nil)
				var failure *rm.Error
				if !errors.As(err, &failure) || failure.Code != "model_invalid_response" {
					t.Fatal("credential echo accepted", err)
				}
			}
		})
	}
}
