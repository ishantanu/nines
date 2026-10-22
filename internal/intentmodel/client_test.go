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

func TestStructuredAdapterBoundsScopeAndRedaction(t *testing.T) {
	var seen string
	mode := "valid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("wrong endpoint/auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		seen = fmt.Sprint(body)
		if mode == "failure" {
			w.WriteHeader(500)
			_, _ = fmt.Fprint(w, "test-secret")
			return
		}
		content := `{"outcome":"availability","targetText":"99.9%","windowText":"28 days","thresholdText":null,"clarifications":[]}`
		if mode == "unknown" {
			content = strings.TrimSuffix(content, "}") + `,"query":"vector(1)"}`
		}
		if mode == "missing" {
			content = `{"outcome":"availability"}`
		}
		finish := "stop"
		if mode == "length" {
			finish = "length"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	for _, m := range []string{"valid", "failure", "unknown", "missing", "length"} {
		mode = m
		c, err := New(server.URL+"/v1/chat/completions", "test-model", "test-secret")
		if err != nil {
			t.Fatal(err)
		}
		result, err := c.Draft(context.Background(), "99.9% success over 28 days")
		if mode == "valid" {
			if err != nil || result.TargetText == nil || !strings.Contains(seen, "json_schema") {
				t.Fatal(result, err, seen)
			}
		} else if err == nil || strings.Contains(err.Error(), "test-secret") {
			t.Fatal(m, result, err)
		}
	}
}
func TestModelConfigRejectsCredentialURLsAndRemoteHTTP(t *testing.T) {
	for _, url := range []string{"http://example.com/v1/chat/completions", "https://user:pass@example.com/v1/chat/completions", "https://example.com/v1/chat/completions?key=secret", "https://example.com/other"} {
		if _, err := New(url, "model", ""); err == nil {
			t.Fatal(url)
		}
	}
}

func TestRuntimeDialectsUseSupportedTokenFields(t *testing.T) {
	for _, style := range []string{"", "openai-compatible", "openai"} {
		t.Run("style="+style, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("local model received unexpected credentials")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if style == "openai" {
					if body["store"] != false || body["max_completion_tokens"] != float64(2048) || body["max_tokens"] != nil {
						t.Errorf("wrong OpenAI payload: %v", body)
					}
				} else if body["max_tokens"] != float64(2048) || body["max_completion_tokens"] != nil || body["store"] != nil {
					t.Errorf("wrong compatible payload: %v", body)
				}
				if body["model"] != "qwen-local" || body["response_format"] == nil || len(body) != map[bool]int{true: 5, false: 4}[style == "openai"] {
					t.Errorf("unexpected model scope: %v", body)
				}
				_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"outcome\":\"availability\",\"targetText\":\"99.9%\",\"windowText\":\"28 days\",\"thresholdText\":null,\"clarifications\":[]}"}}]}`)
			}))
			defer server.Close()
			c, err := NewWithStyle(server.URL+"/v1/chat/completions", "qwen-local", "", style)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.Draft(context.Background(), "99.9% success over 28 days"); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := NewWithStyle("http://localhost:11434/v1/chat/completions", "qwen", "", "unknown"); err == nil {
		t.Fatal("accepted unsupported API style")
	}
}
