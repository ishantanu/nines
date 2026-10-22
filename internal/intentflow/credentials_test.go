package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	rm "github.com/ishantanu/nines/internal/reviewmodel"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentmodel"
	"github.com/ishantanu/nines/internal/model"
)

func TestConversationRejectsConfiguredModelKeyBeforePersistence(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	const key = "fixture-private-provider-key-0123456789"
	client, err := intentmodel.NewConfigured(intentmodel.Config{Provider: "custom", Model: "fixture", Endpoint: "http://127.0.0.1:1/chat/completions"}, key)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = client
	in := input()
	in.Text = "Use this credential: " + key
	if _, err := s.Create(ctx, in); err == nil || strings.Contains(err.Error(), key) {
		t.Fatal("sensitive input persisted or leaked", err)
	}
	if _, err := s.Read(ctx, in.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("rejected conversation was stored", err)
	}
	in = input()
	value, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(ctx, value.ID, v1.AppendIntentConversation{ID: "credential", ExpectedVersion: value.Version, Text: key}); err == nil || strings.Contains(err.Error(), key) {
		t.Fatal("sensitive clarification persisted or leaked", err)
	}
	after, err := s.Read(ctx, value.ID)
	if err != nil || model.DefinitionDigest(value) != model.DefinitionDigest(after) {
		t.Fatal("rejected message changed the conversation", err)
	}
}

func TestModelFailureCodesAreUsefulAndNeverEchoArbitraryProviderText(t *testing.T) {
	for _, test := range []struct{ code, want string }{{"model_truncated", "model_truncated"}, {"model_capability_unavailable", "model_capability_unavailable"}, {"private-provider-key", "model_unavailable"}} {
		s, ctx, _ := fixture(t, false)
		value, err := s.Create(ctx, input())
		if err != nil {
			t.Fatal(err)
		}
		s.Model = &scriptedModel{route: "failure", turn: func(context.Context, rm.Request) (rm.Response, error) { return rm.Response{}, rm.Failure(test.code) }}
		value, err = s.Start(ctx, value.ID, v1.CreateIntentFlowRun{ID: "failure", ExpectedVersion: value.Version, Mode: "model", ModelConsent: true})
		if err != nil {
			t.Fatal(err)
		}
		work(t, s, ctx, 1)
		value, err = s.Read(ctx, value.ID)
		if err != nil || value.Runs[0].ErrorCode != test.want {
			t.Fatal(value, err)
		}
		public, _ := json.Marshal(value)
		if strings.Contains(string(public), "private-provider-key") {
			t.Fatal("provider error leaked")
		}
	}
}
