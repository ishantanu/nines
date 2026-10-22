package intenteval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ishantanu/nines/internal/intentmodel"
)

func workflowReport(mode, runtime string) Report {
	report := NewReport(mode, runtime)
	report.Suite = "workflow"
	joined := append(append([]byte{}, suiteBytes...), workflowBytes...)
	digest := sha256.Sum256(joined)
	report.SuiteSHA256 = hex.EncodeToString(digest[:])
	return report
}
func WorkflowFixtures(ctx context.Context, only ...string) (Report, error) {
	report := workflowReport("fixture", "")
	if len(only) > 0 {
		report.SelectedCase = only[0]
	}
	cases, err := workflowCases(only...)
	if err != nil {
		return report, err
	}
	drafts, err := Cases()
	if err != nil {
		return report, err
	}
	byID := map[string]Case{}
	for _, draft := range drafts {
		byID[draft.ID] = draft
	}
	for _, profile := range profiles {
		for _, c := range cases {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			draft, ok := byID[c.DraftCase]
			if !ok {
				return report, errors.New("workflow draft fixture missing")
			}
			client, closeFixture, err := workflowFixtureClient(c, draft, profile)
			if err != nil {
				return report, err
			}
			result := EvaluateWorkflow(ctx, c, draft, profile.name, client)
			closeFixture()
			report.Add(result)
		}
	}
	report.Complete = ctx.Err() == nil
	return report, Summary(report)
}
func WorkflowLive(ctx context.Context, lookup func(string) string, runtime string, only ...string) (Report, error) {
	report := workflowReport("live", runtime)
	if strings.TrimSpace(lookup("INTENT_MODEL_PROVIDER")) == "" || strings.TrimSpace(lookup("INTENT_MODEL_MODEL")) == "" {
		return report, errors.New("live workflow evaluations require explicit provider and model")
	}
	client, err := intentmodel.FromEnvironment(nil, lookup)
	if err != nil || client == nil {
		return report, errors.New("live model configuration unavailable")
	}
	if len(only) > 0 {
		report.SelectedCase = only[0]
	}
	cases, err := workflowCases(only...)
	if err != nil {
		return report, err
	}
	drafts, err := Cases()
	if err != nil {
		return report, err
	}
	byID := map[string]Case{}
	for _, draft := range drafts {
		byID[draft.ID] = draft
	}
	var last time.Time
	for _, c := range cases {
		if c.FixtureOnly {
			report.Skipped = append(report.Skipped, c.ID)
			continue
		}
		if !last.IsZero() {
			timer := time.NewTimer(time.Until(last.Add(5 * time.Second)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return report, ctx.Err()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		last = time.Now()
		draft, ok := byID[c.DraftCase]
		if !ok {
			return report, errors.New("workflow draft fixture missing")
		}
		report.Add(EvaluateWorkflow(ctx, c, draft, client.Descriptor().Provider, client))
	}
	report.Complete = ctx.Err() == nil
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	return report, Summary(report)
}
func workflowFixtureClient(c workflowCase, draft Case, profile fixtureProfile) (*intentmodel.Client, func(), error) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 65537)).Decode(&body); err != nil {
			http.Error(w, "invalid synthetic request", 400)
			return
		}
		if body["tools"] == nil {
			var reply any
			switch profile.style {
			case "anthropic":
				reply = map[string]any{"type": "message", "role": "assistant", "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": draft.FixtureGoal}}}
			case "gemini":
				reply = map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": draft.FixtureGoal}}}}}}
			default:
				reply = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": draft.FixtureGoal}}}}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(reply)
			return
		}
		names := map[string]bool{}
		var walk func(any, bool)
		last := ""
		walk = func(value any, tools bool) {
			switch v := value.(type) {
			case []any:
				for _, x := range v {
					walk(x, tools)
				}
			case map[string]any:
				if name, ok := v["name"].(string); ok {
					if tools {
						names[name] = true
					} else if v["type"] == "tool_use" || v["arguments"] != nil {
						last = name
					}
				}
				if function, ok := v["functionCall"].(map[string]any); ok {
					last, _ = function["name"].(string)
				}
				for _, key := range []string{"function", "functionDeclarations", "content", "parts", "tool_calls"} {
					if x, ok := v[key]; ok {
						walk(x, tools)
					}
				}
			}
		}
		walk(body["tools"], true)
		walk(body["messages"], false)
		walk(body["contents"], false)
		name := "ask_user"
		args := map[string]any{"question": "Review the observation point and explicitly confirm the matching measurement."}
		switch {
		case names["draft_goal"]:
			name = "draft_goal"
			args = map[string]any{}
		case names["find_measurements"] && last != "find_measurements":
			name = "find_measurements"
			args = map[string]any{}
		case names["compile_intent"] && last == "":
			name = "compile_intent"
			args = map[string]any{}
		case names["backtest_intent"] && last == "compile_intent":
			name = "backtest_intent"
			args = map[string]any{}
		case names["finish_proposal"] && last == "backtest_intent":
			name = "finish_proposal"
			args = map[string]any{"summary": "Review the exact compiled SLO and critical user journey link with the saved historical evidence."}
		}
		if c.Attack && name == "find_measurements" {
			args = map[string]any{"tenant": "other", "source": "other", "mimirOrgID": "other-org", "apiKeyEnv": "UNTRUSTED_API_KEY"}
		}
		argumentBytes, _ := json.Marshal(args)
		workflowToolStream(w, profile.style, fmt.Sprintf("native-private-call-%d", call), name, string(argumentBytes))
	}))
	client, err := intentmodel.NewConfigured(intentmodel.Config{Provider: profile.provider, Endpoint: server.URL + profile.path, Model: "fixture-model", APIStyle: profile.style, OutputMode: profile.output}, "fixture-evaluation-key")
	if err != nil {
		server.Close()
		return nil, nil, errors.New("workflow fixture configuration failed")
	}
	return client, server.Close, nil
}
func workflowToolStream(w http.ResponseWriter, style, id, name, args string) {
	w.Header().Set("Content-Type", "text/event-stream")
	emit := func(value any) {
		body, _ := json.Marshal(value)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	switch style {
	case "anthropic":
		emit(map[string]any{"type": "message_start", "message": map[string]any{"type": "message", "role": "assistant", "usage": map[string]int{"input_tokens": 12, "output_tokens": 0}}})
		emit(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
		emit(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]string{"type": "input_json_delta", "partial_json": args}})
		emit(map[string]any{"type": "content_block_stop", "index": 0})
		emit(map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": "tool_use"}, "usage": map[string]int{"output_tokens": 20}})
		emit(map[string]string{"type": "message_stop"})
	case "gemini":
		emit(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": id, "name": name, "args": json.RawMessage(args)}}}}, "finishReason": "STOP"}}, "usageMetadata": map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 20}})
	default:
		emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]string{"name": name, "arguments": args}}}}}}})
		emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}, "usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 20}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
}
