package intentflow

import (
	"context"
	"encoding/json"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func TestIntentCandidatesWorkerPreservesGoalAndRequiresReview(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := input()
	in.Scope.Journey = nil
	in.Goal.Spec.Journey = nil
	in.Goal.Spec.Measurement = nil
	before := hash(in.Goal)
	v, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = &scriptedModel{route: "candidates", turn: func(_ context.Context, request rm.Request) (rm.Response, error) {
		last := request.Messages[len(request.Messages)-1]
		found := false
		for _, tool := range request.Tools {
			if tool.Name == "discover_measurement_candidates" {
				found = true
			}
		}
		if last.Name == "" {
			if !found {
				t.Fatal("missing optional discovery tool")
			}
			return response("discover_measurement_candidates", map[string]string{"category": "request-serving"}), nil
		}
		if found {
			t.Fatal("candidate discovery offered twice")
		}
		if last.Name != "discover_measurement_candidates" {
			t.Fatal(last.Name)
		}
		var receipt v1.IntentCandidateDiscovery
		if json.Unmarshal(last.Result, &receipt) != nil || receipt.Scope.Source != in.Scope.Source || receipt.Scope.Population["service"] != "api" || !receipt.HumanConfirmationRequired {
			t.Fatal(string(last.Result))
		}
		return response("ask_user", map[string]any{"question": "Which completed outcomes count as success and how are retries counted?", "fields": []string{"semantics"}}), nil
	}}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "candidate-run", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 4)
	v, err = s.Read(ctx, v.ID)
	if err != nil || hash(v.Goal) != before || v.Selection != nil || v.Template != nil || v.ChangeSet != nil {
		t.Fatal("candidate discovery changed goal or approved a proposal", err)
	}
	if len(v.Runs) != 1 || len(v.Runs[0].Steps) != 2 || v.Runs[0].ModelCalls != 2 {
		t.Fatal("unexpected run budget", v.Runs)
	}
	r, err := s.Repository.Read(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Session.Runs[0].Steps[0].IntentDigest != before {
		t.Fatal("candidate step lost server-stamped exact goal digest")
	}
	if !discoveredIntentCandidates(r) {
		t.Fatal("receipt was not persisted")
	}
	if _, err = s.discoverIntentCandidates(ctx, &r, json.RawMessage(`{}`)); err == nil {
		t.Fatal("repeated discovery accepted")
	}
	r.Session.Runs[0].Steps = nil
	for _, args := range []string{`{"source":"other"}`, `{"population":{"service":"other"}}`, `{"target":0.9}`, `{"prefix":"a","prefix":"b"}`} {
		if _, err = s.discoverIntentCandidates(ctx, &r, json.RawMessage(args)); err == nil {
			t.Fatal("model scope override accepted", args)
		}
	}
}
