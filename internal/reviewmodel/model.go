// Package reviewmodel defines vendor-neutral, bounded model conversations.
package reviewmodel

import (
	"context"
	"encoding/json"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type Descriptor = v1.ReviewModelRoute
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}
type Call struct {
	ID         string          `json:"id"`
	ProviderID string          `json:"providerId,omitempty"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
}

// Native preserves provider-owned correlation/signature metadata. It is private
// storage, never a UI response, metric attribute or source of executable authority.
type Message struct {
	Role       string          `json:"role"`
	Text       string          `json:"text,omitempty"`
	Native     json.RawMessage `json:"native,omitempty"`
	CallID     string          `json:"callId,omitempty"`
	ProviderID string          `json:"providerId,omitempty"`
	Name       string          `json:"name,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}
type Request struct {
	System          string    `json:"system"`
	Messages        []Message `json:"messages"`
	Tools           []Tool    `json:"tools"`
	MaxOutputTokens int       `json:"maxOutputTokens"`
}
type Response struct {
	Calls                     []Call
	Native                    json.RawMessage
	InputTokens, OutputTokens int
	UsageKnown                bool
}
type Progress struct {
	Bytes int
	Final bool
}
type Model interface {
	Descriptor() Descriptor
	Turn(context.Context, Request, func(Progress) error) (Response, error)
}
type Error struct{ Code string }

func (e *Error) Error() string  { return e.Code }
func Failure(code string) error { return &Error{Code: code} }
