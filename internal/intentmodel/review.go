package intentmodel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

var callIdentifier = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func (c *Client) Descriptor() rm.Descriptor {
	u, _ := url.Parse(c.endpoint)
	ip := net.ParseIP(u.Hostname())
	digest := sha256.Sum256([]byte(c.provider + "\x00" + c.model + "\x00" + c.apiStyle + "\x00" + c.endpoint))
	return rm.Descriptor{Provider: c.provider, Model: c.model, Protocol: c.apiStyle, RouteDigest: hex.EncodeToString(digest[:]), Local: u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()}
}
func (c *Client) Turn(ctx context.Context, in rm.Request, progress func(rm.Progress) error) (rm.Response, error) {
	var out rm.Response
	if in.MaxOutputTokens < 1 || in.MaxOutputTokens > 1024 || len(in.Messages) > 20 || len(in.Tools) > 4 {
		return out, rm.Failure("model_budget_exceeded")
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	default:
		return out, rm.Failure("model_unavailable")
	}
	payload, err := c.reviewPayload(in)
	if err != nil {
		return out, err
	}
	body, err := json.Marshal(payload)
	if err != nil || len(body) > 64<<10 {
		return out, rm.Failure("model_budget_exceeded")
	}
	endpoint := c.endpoint
	if c.apiStyle == "gemini" {
		endpoint = strings.TrimSuffix(endpoint, ":generateContent") + ":streamGenerateContent?alt=sse"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return out, rm.Failure("model_unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.authenticate(req)
	h := *c.http
	h.Timeout = 65 * time.Second
	res, err := h.Do(req)
	if err != nil {
		return out, rm.Failure("model_unavailable")
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != 200 {
		if res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != 429 {
			return out, rm.Failure("model_capability_unavailable")
		}
		return out, rm.Failure("model_unavailable")
	}
	typ, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	var raw []byte
	if typ == "text/event-stream" {
		raw, err = collectReviewStream(res.Body, c.apiStyle, progress)
	} else {
		raw, err = io.ReadAll(io.LimitReader(res.Body, (256<<10)+1))
		if len(raw) > 256<<10 {
			err = rm.Failure("model_budget_exceeded")
		}
		if progress != nil && err == nil {
			err = progress(rm.Progress{Bytes: len(raw), Final: true})
		}
	}
	if err != nil {
		return out, err
	}
	return c.parseReview(raw, in.MaxOutputTokens)
}
func (c *Client) reviewPayload(in rm.Request) (map[string]any, error) {
	tools := []any{}
	messages := []any{}
	for _, t := range in.Tools {
		var schema any
		if json.Unmarshal(t.Schema, &schema) != nil {
			return nil, rm.Failure("model_invalid_response")
		}
		switch c.apiStyle {
		case "anthropic":
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "input_schema": schema})
		case "gemini":
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "parametersJsonSchema": schema})
		default:
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": schema}})
		}
	}
	for i := 0; i < len(in.Messages); i++ {
		m := in.Messages[i]
		if m.Role == "assistant" {
			var native any
			if json.Unmarshal(m.Native, &native) != nil {
				return nil, rm.Failure("model_invalid_response")
			}
			messages = append(messages, native)
			continue
		}
		switch c.apiStyle {
		case "anthropic":
			if m.Role == "tool" {
				parts := []any{}
				for {
					parts = append(parts, map[string]any{"type": "tool_result", "tool_use_id": m.ProviderID, "content": string(m.Result)})
					if i+1 >= len(in.Messages) || in.Messages[i+1].Role != "tool" {
						break
					}
					i++
					m = in.Messages[i]
				}
				messages = append(messages, map[string]any{"role": "user", "content": parts})
			} else {
				messages = append(messages, map[string]any{"role": "user", "content": m.Text})
			}
		case "gemini":
			if m.Role == "tool" {
				parts := []any{}
				for {
					var result any
					if json.Unmarshal(m.Result, &result) != nil {
						return nil, rm.Failure("model_invalid_response")
					}
					response := map[string]any{"name": m.Name, "response": map[string]any{"result": result}}
					if m.ProviderID != "" {
						response["id"] = m.ProviderID
					}
					parts = append(parts, map[string]any{"functionResponse": response})
					if i+1 >= len(in.Messages) || in.Messages[i+1].Role != "tool" {
						break
					}
					i++
					m = in.Messages[i]
				}
				messages = append(messages, map[string]any{"role": "user", "parts": parts})
			} else {
				messages = append(messages, map[string]any{"role": "user", "parts": []any{map[string]any{"text": m.Text}}})
			}
		default:
			if m.Role == "tool" {
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": m.ProviderID, "content": string(m.Result)})
			} else {
				messages = append(messages, map[string]any{"role": "user", "content": m.Text})
			}
		}
	}
	switch c.apiStyle {
	case "anthropic":
		return map[string]any{"model": c.model, "system": in.System, "messages": messages, "tools": tools, "max_tokens": in.MaxOutputTokens, "stream": true}, nil
	case "gemini":
		return map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": in.System}}}, "contents": messages, "tools": []any{map[string]any{"functionDeclarations": tools}}, "generationConfig": map[string]any{"maxOutputTokens": in.MaxOutputTokens}}, nil
	default:
		messages = append([]any{map[string]string{"role": "system", "content": in.System}}, messages...)
		p := map[string]any{"model": c.model, "messages": messages, "tools": tools, "stream": true, "stream_options": map[string]any{"include_usage": true}}
		if c.apiStyle == "openai" {
			p["max_completion_tokens"] = in.MaxOutputTokens
			p["store"] = false
		} else {
			p["max_tokens"] = in.MaxOutputTokens
		}
		return p, nil
	}
}
func (c *Client) parseReview(b []byte, maxTokens int) (rm.Response, error) {
	out := rm.Response{}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(b, &envelope) != nil {
		return out, rm.Failure("model_invalid_response")
	}
	stop := ""
	var native any
	switch c.apiStyle {
	case "anthropic":
		var e struct {
			Type, Role string
			Stop       string `json:"stop_reason"`
			Content    []map[string]json.RawMessage
			Usage      struct {
				Input  int `json:"input_tokens"`
				Output int `json:"output_tokens"`
			}
		}
		if json.Unmarshal(b, &e) != nil || e.Type != "message" || e.Role != "assistant" || len(e.Content) > 16 {
			return out, rm.Failure("model_invalid_response")
		}
		stop = e.Stop
		out.InputTokens = e.Usage.Input
		out.OutputTokens = e.Usage.Output
		_, out.UsageKnown = envelope["usage"]
		for _, p := range e.Content {
			var typ string
			_ = json.Unmarshal(p["type"], &typ)
			switch typ {
			case "tool_use":
				var id, name string
				_ = json.Unmarshal(p["id"], &id)
				_ = json.Unmarshal(p["name"], &name)
				out.Calls = append(out.Calls, rm.Call{ID: id, ProviderID: id, Name: name, Arguments: p["input"]})
			case "text":
			case "thinking", "redacted_thinking":
				return out, rm.Failure("model_capability_unavailable")
			default:
				return out, rm.Failure("model_invalid_response")
			}
		}
		native = map[string]any{"role": "assistant", "content": e.Content}
		if stop == "refusal" {
			return out, rm.Failure("model_refused")
		}
		if stop == "max_tokens" {
			return out, rm.Failure("model_truncated")
		}
		if stop != "tool_use" && stop != "end_turn" {
			return out, rm.Failure("model_incomplete")
		}
	case "gemini":
		var e struct {
			PromptFeedback struct{ BlockReason string }
			Candidates     []struct {
				Content struct {
					Role  string
					Parts []map[string]json.RawMessage
				}
				FinishReason  string
				SafetyRatings []struct{ Blocked bool }
			}
			UsageMetadata *struct{ PromptTokenCount, CandidatesTokenCount, ThoughtsTokenCount int }
		}
		if json.Unmarshal(b, &e) != nil {
			return out, rm.Failure("model_invalid_response")
		}
		if e.PromptFeedback.BlockReason != "" {
			return out, rm.Failure("model_refused")
		}
		if len(e.Candidates) != 1 || e.Candidates[0].Content.Role != "model" || len(e.Candidates[0].Content.Parts) > 256 {
			return out, rm.Failure("model_invalid_response")
		}
		choice := e.Candidates[0]
		for _, s := range choice.SafetyRatings {
			if s.Blocked {
				return out, rm.Failure("model_refused")
			}
		}
		stop = choice.FinishReason
		for i, p := range choice.Content.Parts {
			if fn, ok := p["functionCall"]; ok {
				var call struct {
					ID, Name string
					Args     json.RawMessage
				}
				if json.Unmarshal(fn, &call) != nil {
					return out, rm.Failure("model_invalid_response")
				}
				id := call.ID
				if id == "" {
					id = fmtCallID(i)
				}
				out.Calls = append(out.Calls, rm.Call{ID: id, ProviderID: call.ID, Name: call.Name, Arguments: call.Args})
			}
		}
		native = map[string]any{"role": choice.Content.Role, "parts": choice.Content.Parts}
		if e.UsageMetadata != nil {
			out.UsageKnown = true
			out.InputTokens = e.UsageMetadata.PromptTokenCount
			out.OutputTokens = e.UsageMetadata.CandidatesTokenCount + e.UsageMetadata.ThoughtsTokenCount
		}
		if stop == "MAX_TOKENS" {
			return out, rm.Failure("model_truncated")
		}
		if stop != "STOP" {
			return out, rm.Failure("model_refused")
		}
	default:
		var e struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Role      string
					Content   json.RawMessage
					Refusal   *string
					ToolCalls []struct {
						ID, Type string
						Function struct{ Name, Arguments string }
					} `json:"tool_calls"`
				}
			}
			Usage *struct {
				Input  int `json:"prompt_tokens"`
				Output int `json:"completion_tokens"`
			}
		}
		if json.Unmarshal(b, &e) != nil || len(e.Choices) != 1 || e.Choices[0].Message.Role != "assistant" {
			return out, rm.Failure("model_invalid_response")
		}
		choice := e.Choices[0]
		if choice.Message.Refusal != nil || choice.FinishReason == "content_filter" {
			return out, rm.Failure("model_refused")
		}
		stop = choice.FinishReason
		for _, call := range choice.Message.ToolCalls {
			if call.Type != "function" {
				return out, rm.Failure("model_invalid_response")
			}
			out.Calls = append(out.Calls, rm.Call{ID: call.ID, ProviderID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
		}
		nativeCalls := []any{}
		for _, call := range choice.Message.ToolCalls {
			nativeCalls = append(nativeCalls, map[string]any{"id": call.ID, "type": call.Type, "function": map[string]string{"name": call.Function.Name, "arguments": call.Function.Arguments}})
		}
		native = map[string]any{"role": "assistant", "content": choice.Message.Content, "tool_calls": nativeCalls}
		if e.Usage != nil {
			out.UsageKnown = true
			out.InputTokens = e.Usage.Input
			out.OutputTokens = e.Usage.Output
		}
		if stop == "length" {
			return out, rm.Failure("model_truncated")
		}
		if stop != "tool_calls" && stop != "stop" {
			return out, rm.Failure("model_incomplete")
		}
	}
	if len(out.Calls) == 0 {
		return out, rm.Failure("model_capability_unavailable")
	}
	if len(out.Calls) > 3 || out.InputTokens < 0 || out.OutputTokens < 0 || out.OutputTokens > maxTokens {
		return out, rm.Failure("model_budget_exceeded")
	}
	seen := map[string]bool{}
	for _, call := range out.Calls {
		if !callIdentifier.MatchString(call.ID) || seen[call.ID] || len(call.Name) > 64 || len(call.Arguments) > 8192 || !json.Valid(call.Arguments) {
			return out, rm.Failure("model_invalid_response")
		}
		seen[call.ID] = true
	}
	out.Native, _ = json.Marshal(native)
	if len(out.Native) > 64<<10 {
		return out, rm.Failure("model_budget_exceeded")
	}
	return out, nil
}
