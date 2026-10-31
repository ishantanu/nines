package intentmodel

import (
	"encoding/json"
	"net/http"
)

func (c *Client) payload(text string) map[string]any {
	instruction := prompt
	if c.outputMode == "json" {
		b, _ := json.Marshal(schema)
		instruction += " Return one JSON object matching this JSON Schema: " + string(b)
	}
	switch c.apiStyle {
	case "anthropic":
		return map[string]any{"model": c.model, "max_tokens": 2048, "system": instruction, "messages": []map[string]string{{"role": "user", "content": text}}, "output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": schema}}}
	case "gemini":
		return map[string]any{"systemInstruction": map[string]any{"parts": []map[string]string{{"text": instruction}}}, "contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": text}}}}, "generationConfig": map[string]any{"maxOutputTokens": 2048, "responseMimeType": "application/json", "responseJsonSchema": schema}}
	default:
		format := map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "slo_goal", "strict": true, "schema": schema}}
		if c.outputMode == "json" {
			format = map[string]any{"type": "json_object"}
		}
		out := map[string]any{"model": c.model, "messages": []map[string]string{{"role": "system", "content": instruction}, {"role": "user", "content": text}}, "response_format": format}
		if c.apiStyle == "openai" {
			out["store"] = false
			out["max_completion_tokens"] = 2048
		} else {
			out["max_tokens"] = 2048
		}
		return out
	}
}
func (c *Client) authenticate(req *http.Request) {
	switch c.apiStyle {
	case "anthropic":
		req.Header.Set("anthropic-version", "2023-06-01")
		if c.key != "" {
			req.Header.Set("x-api-key", c.key)
		}
	case "gemini":
		if c.key != "" {
			req.Header.Set("x-goog-api-key", c.key)
		}
	default:
		if c.key != "" {
			req.Header.Set("Authorization", "Bearer "+c.key)
		}
	}
}
func (c *Client) responseText(b []byte) (string, error) {
	switch c.apiStyle {
	case "anthropic":
		var e struct {
			Type, Role string
			StopReason string `json:"stop_reason"`
			Content    []struct{ Type, Text string }
		}
		if json.Unmarshal(b, &e) != nil || e.Type != "message" || e.Role != "assistant" || e.StopReason != "end_turn" || len(e.Content) > 32 {
			return "", ErrUnavailable
		}
		content := ""
		count := 0
		for _, part := range e.Content {
			switch part.Type {
			case "text":
				content = part.Text
				count++
			case "thinking", "redacted_thinking":
			default:
				return "", ErrUnavailable
			}
		}
		if count != 1 {
			return "", ErrUnavailable
		}
		return content, nil
	case "gemini":
		var e struct {
			PromptFeedback struct{ BlockReason string }
			Candidates     []struct {
				FinishReason string
				Content      struct {
					Role  string
					Parts []map[string]json.RawMessage
				}
				SafetyRatings []struct{ Blocked bool }
			}
		}
		if json.Unmarshal(b, &e) != nil || e.PromptFeedback.BlockReason != "" || len(e.Candidates) != 1 {
			return "", ErrUnavailable
		}
		choice := e.Candidates[0]
		if choice.FinishReason != "STOP" || choice.Content.Role != "model" || len(choice.Content.Parts) > 32 {
			return "", ErrUnavailable
		}
		for _, rating := range choice.SafetyRatings {
			if rating.Blocked {
				return "", ErrUnavailable
			}
		}
		content := ""
		count := 0
		for _, part := range choice.Content.Parts {
			for k := range part {
				if k != "text" && k != "thought" && k != "thoughtSignature" {
					return "", ErrUnavailable
				}
			}
			var thought bool
			if b, ok := part["thought"]; ok && json.Unmarshal(b, &thought) != nil {
				return "", ErrUnavailable
			}
			if thought {
				continue
			}
			var text string
			if json.Unmarshal(part["text"], &text) != nil {
				return "", ErrUnavailable
			}
			content = text
			count++
		}
		if count != 1 {
			return "", ErrUnavailable
		}
		return content, nil
	default:
		var e struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content      string
					Refusal      *string
					ToolCalls    []json.RawMessage `json:"tool_calls"`
					FunctionCall json.RawMessage   `json:"function_call"`
				}
			}
		}
		if json.Unmarshal(b, &e) != nil || len(e.Choices) != 1 {
			return "", ErrUnavailable
		}
		choice := e.Choices[0]
		if choice.FinishReason != "stop" || choice.Message.Refusal != nil || len(choice.Message.ToolCalls) > 0 || len(choice.Message.FunctionCall) > 0 && string(choice.Message.FunctionCall) != "null" {
			return "", ErrUnavailable
		}
		return choice.Message.Content, nil
	}
}
