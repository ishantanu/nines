package intentmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func emitSSE(w http.ResponseWriter, value any) {
	b, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
func toolStream(w http.ResponseWriter, style, id, name, args string) {
	w.Header().Set("Content-Type", "text/event-stream")
	switch style {
	case "anthropic":
		emitSSE(w, map[string]any{"type": "message_start", "message": map[string]any{"type": "message", "role": "assistant", "usage": map[string]int{"input_tokens": 12, "output_tokens": 0}}})
		emitSSE(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
		split := len(args) / 2
		for _, part := range []string{args[:split], args[split:]} {
			emitSSE(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]string{"type": "input_json_delta", "partial_json": part}})
		}
		emitSSE(w, map[string]any{"type": "content_block_stop", "index": 0})
		emitSSE(w, map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": "tool_use"}, "usage": map[string]int{"output_tokens": 20}})
		emitSSE(w, map[string]string{"type": "message_stop"})
	case "gemini":
		emitSSE(w, map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": id, "name": name, "args": json.RawMessage(args)}, "thoughtSignature": "opaque-signature"}}}, "finishReason": "STOP"}}, "usageMetadata": map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 20}})
	default:
		split := len(args) / 2
		emitSSE(w, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]string{"name": name, "arguments": args[:split]}}}}}}})
		emitSSE(w, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]string{"arguments": args[split:]}}}}, "finish_reason": "tool_calls"}}, "usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 20}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
}
func TestNativeReviewToolRoundTripPreservesCorrelation(t *testing.T) {
	for _, style := range []string{"anthropic", "gemini", "chat-completions", "openai"} {
		t.Run(style, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("invalid payload")
				}
				if style == "anthropic" && r.Header.Get("x-api-key") != "secret" || style == "gemini" && r.Header.Get("x-goog-api-key") != "secret" || (style == "openai" || style == "chat-completions") && r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("wrong native credentials")
				}
				if style == "gemini" {
					declaration := body["tools"].([]any)[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
					if declaration["parametersJsonSchema"] == nil || declaration["parameters"] != nil {
						t.Error("JSON Schema must use native parametersJsonSchema")
					}
				}
				if calls == 2 {
					b, _ := json.Marshal(body)
					if !strings.Contains(string(b), "first-call") {
						t.Error("lost tool correlation", string(b))
					}
					if style == "gemini" && !strings.Contains(string(b), "opaque-signature") {
						t.Error("lost Gemini signature")
					}
					switch style {
					case "anthropic":
						messages := body["messages"].([]any)
						if messages[1].(map[string]any)["role"] != "assistant" {
							t.Error("incorrect native role")
						}
					case "gemini":
						messages := body["contents"].([]any)
						if messages[1].(map[string]any)["role"] != "model" {
							t.Error("incorrect native role")
						}
					default:
						messages := body["messages"].([]any)
						call := messages[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
						if call["id"] != "first-call" || call["function"] == nil {
							t.Error("incorrect native function fields", call)
						}
					}
				}
				toolStream(w, style, fmt.Sprintf("%s-call", map[bool]string{true: "first", false: "second"}[calls == 1]), "read_assessment", `{}`)
			}))
			defer server.Close()
			suffix := "/chat/completions"
			if style == "anthropic" {
				suffix = "/messages"
			}
			if style == "gemini" {
				suffix = "/models/fixture:generateContent"
			}
			c, err := NewWithStyle(server.URL+suffix, "fixture-model", "secret", style)
			if err != nil {
				t.Fatal(err)
			}
			request := rm.Request{System: "Read only.", Messages: []rm.Message{{Role: "user", Text: "Review availability"}}, Tools: []rm.Tool{{Name: "read_assessment", Schema: json.RawMessage(`{"type":"object","properties":{}}`)}}, MaxOutputTokens: 1024}
			events := 0
			first, err := c.Turn(context.Background(), request, func(p rm.Progress) error { events++; return nil })
			if err != nil || len(first.Calls) != 1 || first.Calls[0].ID != "first-call" || !first.UsageKnown || first.OutputTokens != 20 || events == 0 {
				t.Fatal(first, events, err)
			}
			request.Messages = append(request.Messages, rm.Message{Role: "assistant", Native: first.Native}, rm.Message{Role: "tool", Name: first.Calls[0].Name, CallID: first.Calls[0].ID, ProviderID: first.Calls[0].ProviderID, Result: json.RawMessage(`{"state":"insufficient_evidence"}`)})
			if _, err = c.Turn(context.Background(), request, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestReviewRefusalTruncationAndIncompleteStreams(t *testing.T) {
	c, _ := NewWithStyle("http://127.0.0.1:1/chat/completions", "fixture", "", "chat-completions")
	for _, tc := range []struct{ body, code string }{{`{"choices":[{"finish_reason":"content_filter","message":{"role":"assistant","content":null}}]}`, "model_refused"}, {`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial"}}]}`, "model_truncated"}, {`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"I cannot call tools"}}]}`, "model_capability_unavailable"}} {
		_, err := c.parseReview([]byte(tc.body), 1024)
		var typed *rm.Error
		if !errors.As(err, &typed) || typed.Code != tc.code {
			t.Fatal(err, tc.code)
		}
	}
	if _, err := collectReviewStream(strings.NewReader("data: {\"choices\":[]}\n\n"), "chat-completions", nil); err == nil {
		t.Fatal("incomplete stream accepted")
	}
}
