package intentflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func TestIntentClarificationRejectsQuestionsAboutSuppliedFields(t *testing.T) {
	s, ctx, v := readyInlineEvidenceFixture(t, false)
	r, err := s.Repository.Read(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"question": "What target and rolling window do you want?", "fields": []string{"target", "window", "semantics"}})
	before := hash(r.Session.Goal)
	messages := len(r.Session.Messages)
	out, err := s.execute(ctx, &r, rm.Call{Name: "ask_user", Arguments: raw})
	if err != nil {
		t.Fatal(err)
	}
	result := out.(map[string]any)
	if result["status"] != "fields_already_provided" || len(r.Session.Messages) != messages || hash(r.Session.Goal) != before || r.Session.State != "ready" {
		t.Fatal("redundant question reached user or changed commitments", out)
	}
	supplied, unresolved, ok := clarificationFields(v, []string{"population", "target", "window", "metric", "observationPoint", "semantics"})
	if !ok || len(supplied) != 5 || len(unresolved) != 1 || unresolved[0] != "semantics" {
		t.Fatal(supplied, unresolved, ok)
	}
	for _, fields := range [][]string{{"unknown-field"}, {"target", "target"}} {
		if _, _, ok := clarificationFields(v, fields); ok {
			t.Fatal("invalid fields accepted")
		}
	}
	raw, _ = json.Marshal(map[string]any{"question": "How are retries counted?", "fields": []string{"semantics"}})
	if _, err = s.execute(ctx, &r, rm.Call{Name: "ask_user", Arguments: raw}); err != nil || r.Session.State != "needs_input" {
		t.Fatal("genuinely unresolved semantics did not reach user", err)
	}
}
func TestIntentClarificationPreservesGoalAndInvalidatesPriorConfirmation(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) { runTestIntentClarificationPreservesGoalAndInvalidatesPriorConfirmation(t, pg) })
	}
}
func runTestIntentClarificationPreservesGoalAndInvalidatesPriorConfirmation(t *testing.T, pg bool) {
	s, ctx, v := readyInlineEvidenceFixture(t, pg)
	original := hash(*v.Goal)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	in := v1.AppendIntentConversation{ID: "clarify-retries", ExpectedVersion: v.Version, Text: "Retries are counted as separate attempts.", PreserveGoal: true}
	updated, err := s.Append(ctx, v.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if hash(*updated.Goal) != original || updated.Selection != nil || updated.Plan != nil || updated.Backtest != nil || updated.PreparationEvidence != nil || updated.ChangeSet != nil {
		t.Fatal("clarification lost known facts or reused confirmation")
	}
	retry, err := s.Append(ctx, v.ID, in)
	if err != nil || retry.Version != updated.Version {
		t.Fatal("exact clarification retry not idempotent", err)
	}
	changed := in
	changed.PreserveGoal = false
	if _, err = s.Append(ctx, v.ID, changed); err == nil {
		t.Fatal("different preservation semantics accepted for same message")
	}
	replacement := clone(*updated.Goal)
	changed = v1.AppendIntentConversation{ID: "ambiguous", ExpectedVersion: updated.Version, Text: "Replace goal", Goal: &replacement, PreserveGoal: true}
	if _, err = s.Append(ctx, v.ID, changed); err == nil {
		t.Fatal("ambiguous preservation and replacement accepted")
	}
}

func TestIntentClarificationWorkerDoesNotRepeatKnownTarget(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := input()
	v, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = &scriptedModel{route: "clarification-fixture", turn: func(_ context.Context, req rm.Request) (rm.Response, error) {
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "tool" {
			return response("ask_user", map[string]any{"question": "What target should I use?", "fields": []string{"target"}}), nil
		}
		var result map[string]any
		if err := json.Unmarshal(last.Result, &result); err != nil || result["status"] != "fields_already_provided" {
			t.Fatal("model did not receive supplied-fact feedback", err)
		}
		return response("ask_user", map[string]any{"question": "How should retries be counted?", "fields": []string{"semantics"}}), nil
	}}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "clarify", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 4)
	v, err = s.Read(ctx, v.ID)
	if err != nil || v.State != "needs_input" || v.Goal.Spec.Target != in.Goal.Spec.Target {
		t.Fatal("clarification flow changed target", err)
	}
	for _, m := range v.Messages {
		if strings.Contains(m.Text, "What target should I use?") {
			t.Fatal("known-target question escaped worker guard")
		}
	}
	if !strings.Contains(v.Messages[len(v.Messages)-1].Text, "How should retries be counted?") {
		t.Fatal("unresolved question not recorded")
	}
}
