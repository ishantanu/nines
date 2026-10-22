package intentmodel

import (
	"bufio"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func fmtCallID(i int) string { return "gemini-" + strconv.Itoa(i) }
func collectReviewStream(reader io.Reader, style string, progress func(rm.Progress) error) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: (1 << 20) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 256<<10)
	total := 0
	done := false
	data := []string{}
	message := map[string]any{}
	blocks := []map[string]any{}
	args := map[int]string{}
	stopped := map[int]bool{}
	chatCalls := map[int]map[string]any{}
	parts := []any{}
	finish := ""
	var usage any
	var feedback any
	var ratings any
	handle := func(raw string) error {
		if done {
			return rm.Failure("model_invalid_response")
		}
		if raw == "[DONE]" {
			if style == "anthropic" || style == "gemini" {
				return rm.Failure("model_invalid_response")
			}
			done = true
			return nil
		}
		var e map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &e) != nil {
			return rm.Failure("model_invalid_response")
		}
		if e["error"] != nil {
			return rm.Failure("model_unavailable")
		}
		if progress != nil {
			if err := progress(rm.Progress{Bytes: total}); err != nil {
				return err
			}
		}
		switch style {
		case "anthropic":
			var event struct {
				Type         string
				Index        int
				Message      map[string]any
				ContentBlock map[string]any `json:"content_block"`
				Delta        struct {
					Type, Text  string
					PartialJSON string `json:"partial_json"`
					StopReason  string `json:"stop_reason"`
				}
				Usage map[string]any
			}
			if json.Unmarshal([]byte(raw), &event) != nil {
				return rm.Failure("model_invalid_response")
			}
			switch event.Type {
			case "message_start":
				if message["type"] != nil {
					return rm.Failure("model_invalid_response")
				}
				message = event.Message
			case "content_block_start":
				if event.Index != len(blocks) || len(blocks) >= 16 {
					return rm.Failure("model_invalid_response")
				}
				if event.ContentBlock["type"] != "text" && event.ContentBlock["type"] != "tool_use" {
					return rm.Failure("model_capability_unavailable")
				}
				blocks = append(blocks, event.ContentBlock)
			case "content_block_delta":
				if stopped[event.Index] {
					return rm.Failure("model_invalid_response")
				}
				if event.Index < 0 || event.Index >= len(blocks) {
					return rm.Failure("model_invalid_response")
				}
				switch event.Delta.Type {
				case "input_json_delta":
					args[event.Index] += event.Delta.PartialJSON
					if len(args[event.Index]) > 8192 {
						return rm.Failure("model_budget_exceeded")
					}
				case "text_delta":
					text, _ := blocks[event.Index]["text"].(string)
					blocks[event.Index]["text"] = text + event.Delta.Text
				default:
					return rm.Failure("model_capability_unavailable")
				}
			case "content_block_stop":
				if stopped[event.Index] {
					return rm.Failure("model_invalid_response")
				}
				stopped[event.Index] = true
				if event.Index < 0 || event.Index >= len(blocks) {
					return rm.Failure("model_invalid_response")
				}
				if value, ok := args[event.Index]; ok {
					var input any
					if json.Unmarshal([]byte(value), &input) != nil {
						return rm.Failure("model_invalid_response")
					}
					blocks[event.Index]["input"] = input
				}
			case "message_delta":
				message["stop_reason"] = event.Delta.StopReason
				if event.Usage != nil {
					old, _ := message["usage"].(map[string]any)
					if old == nil {
						old = map[string]any{}
					}
					for k, v := range event.Usage {
						old[k] = v
					}
					message["usage"] = old
				}
			case "message_stop":
				if len(stopped) != len(blocks) {
					return rm.Failure("model_incomplete")
				}
				done = true
			case "ping":
			default:
				return rm.Failure("model_capability_unavailable")
			}
		case "gemini":
			var event struct {
				Candidates []struct {
					Content struct {
						Role  string
						Parts []json.RawMessage
					}
					FinishReason  string
					SafetyRatings json.RawMessage
				}
				UsageMetadata  json.RawMessage
				PromptFeedback json.RawMessage
			}
			if json.Unmarshal([]byte(raw), &event) != nil || len(event.Candidates) > 1 {
				return rm.Failure("model_invalid_response")
			}
			if len(event.PromptFeedback) > 0 {
				feedback = event.PromptFeedback
			}
			if len(event.UsageMetadata) > 0 {
				usage = event.UsageMetadata
			}
			if len(event.Candidates) == 1 {
				choice := event.Candidates[0]
				for _, p := range choice.Content.Parts {
					parts = append(parts, p)
					if len(parts) > 256 {
						return rm.Failure("model_budget_exceeded")
					}
				}
				if choice.FinishReason != "" {
					finish = choice.FinishReason
				}
				if len(choice.SafetyRatings) > 0 {
					ratings = choice.SafetyRatings
				}
			}
		default:
			var event struct {
				Choices []struct {
					Index        int
					FinishReason *string `json:"finish_reason"`
					Delta        struct {
						Content   string
						Refusal   *string
						ToolCalls []struct {
							Index    int
							ID, Type string
							Function struct{ Name, Arguments string }
						} `json:"tool_calls"`
					}
				}
				Usage json.RawMessage
			}
			if json.Unmarshal([]byte(raw), &event) != nil || len(event.Choices) > 1 {
				return rm.Failure("model_invalid_response")
			}
			if len(event.Usage) > 0 && string(event.Usage) != "null" {
				usage = event.Usage
			}
			if len(event.Choices) == 1 {
				choice := event.Choices[0]
				if choice.Index != 0 {
					return rm.Failure("model_invalid_response")
				}
				if choice.Delta.Refusal != nil {
					return rm.Failure("model_refused")
				}
				if choice.FinishReason != nil {
					finish = *choice.FinishReason
				}
				for _, delta := range choice.Delta.ToolCalls {
					if delta.Index < 0 || delta.Index > 2 {
						return rm.Failure("model_budget_exceeded")
					}
					call := chatCalls[delta.Index]
					if call == nil {
						call = map[string]any{"id": "", "type": "function", "function": map[string]any{"name": "", "arguments": ""}}
						chatCalls[delta.Index] = call
					}
					if delta.ID != "" {
						call["id"] = call["id"].(string) + delta.ID
					}
					if delta.Type != "" {
						call["type"] = delta.Type
					}
					fn := call["function"].(map[string]any)
					fn["name"] = fn["name"].(string) + delta.Function.Name
					fn["arguments"] = fn["arguments"].(string) + delta.Function.Arguments
					if len(fn["arguments"].(string)) > 8192 {
						return rm.Failure("model_budget_exceeded")
					}
				}
			}
		}
		return nil
	}
	flush := func() error {
		if len(data) == 0 {
			return nil
		}
		raw := strings.Join(data, "\n")
		data = nil
		return handle(raw)
	}
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line) + 1
		if total > 1<<20 {
			return nil, rm.Failure("model_budget_exceeded")
		}
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, rm.Failure("model_incomplete")
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if progress != nil {
		if err := progress(rm.Progress{Bytes: total, Final: true}); err != nil {
			return nil, err
		}
	}
	if style == "anthropic" {
		if !done {
			return nil, rm.Failure("model_incomplete")
		}
		message["content"] = blocks
		return json.Marshal(message)
	}
	if style == "gemini" {
		if feedback != nil {
			b, _ := json.Marshal(feedback)
			var value struct{ BlockReason string }
			_ = json.Unmarshal(b, &value)
			if value.BlockReason != "" {
				return nil, rm.Failure("model_refused")
			}
		}
		if finish == "" {
			return nil, rm.Failure("model_incomplete")
		}
		out := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": parts}, "finishReason": finish, "safetyRatings": ratings}}}
		if usage != nil {
			out["usageMetadata"] = usage
		}
		if feedback != nil {
			out["promptFeedback"] = feedback
		}
		return json.Marshal(out)
	}
	if !done || finish == "" {
		return nil, rm.Failure("model_incomplete")
	}
	calls := []any{}
	for i := 0; i < len(chatCalls); i++ {
		if chatCalls[i] == nil {
			return nil, rm.Failure("model_invalid_response")
		}
		calls = append(calls, chatCalls[i])
	}
	out := map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"role": "assistant", "content": nil, "tool_calls": calls}}}}
	if usage != nil {
		out["usage"] = usage
	}
	return json.Marshal(out)
}
