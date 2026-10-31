package intentflow

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestWorkflowStatusGatesAndSavedOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, state, change, rollback, outcome, action string
		human, independent                             bool
	}{
		{name: "active", state: "running", action: "wait"},
		{name: "clarification", state: "needs_input", action: "message", human: true},
		{name: "measurement", state: "needs_selection", action: "confirm", human: true},
		{name: "ready", state: "ready", action: "changes"},
		{name: "approval", state: "ready", change: "proposed", action: "approve", human: true},
		{name: "apply", state: "ready", change: "approved", action: "apply", human: true, independent: true},
		{name: "committed_is_not_healthy", state: "ready", change: "applied", outcome: "pending", action: "wait"},
		{name: "verified", state: "ready", change: "applied", outcome: "verified", action: "verify"},
		{name: "regressed", state: "ready", change: "applied", outcome: "regressed", action: "verify", human: true},
		{name: "insufficient", state: "ready", change: "applied", outcome: "insufficient_evidence", action: "verify", human: true},
		{name: "unknown_outcome", state: "ready", change: "applied", outcome: "future_state", action: "verify", human: true},
		{name: "restore_review", state: "ready", change: "applied", rollback: "proposed", action: "rollback-approve", human: true},
		{name: "restore_apply", state: "ready", change: "applied", rollback: "approved", action: "rollback-apply", human: true, independent: true},
		{name: "restore_blocked", state: "ready", change: "applied", rollback: "blocked", action: "show", human: true},
		{name: "restore_rejected", state: "ready", change: "applied", rollback: "rejected", action: "show", human: true},
		{name: "restore_unknown", state: "ready", change: "applied", rollback: "future", action: "show", human: true},
		{name: "restore_observation", state: "ready", change: "applied", rollback: "applied", outcome: "pending", action: "wait"},
		{name: "rejected", state: "ready", change: "rejected", action: "show", human: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := v1.IntentConversation{Tenant: "alpha", IntentConversationSummary: v1.IntentConversationSummary{ID: "chat", Version: 7, State: tc.state}}
			if tc.change != "" {
				v.ChangeSet = &v1.IntentChangeSet{State: tc.change, Digest: "change"}
				outcome := &v1.IntentOutcome{State: tc.outcome}
				if tc.rollback != "" {
					v.ChangeSet.Rollback = &v1.IntentRollback{State: tc.rollback, Digest: "restore", Outcome: outcome}
				} else {
					v.ChangeSet.Outcome = outcome
				}
			}
			got := workflowStatus(v)
			if got.NextAction != tc.action || got.HumanRequired != tc.human || got.IndependentApplierRequired != tc.independent || got.Version != 7 || got.Conversation != "chat" || got.Tenant != "alpha" {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestWorkflowStatusWithoutObservationDoesNotWaitForever(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		v := v1.IntentConversation{Tenant: "alpha", IntentConversationSummary: v1.IntentConversationSummary{ID: "chat", Version: 3}, ChangeSet: &v1.IntentChangeSet{State: "applied", Digest: "change"}}
		if rollback {
			v.ChangeSet.Rollback = &v1.IntentRollback{State: "applied", Digest: "restore"}
		}
		status := workflowStatus(v)
		if status.Phase != "outcome_unavailable" || status.NextAction != "verify" || !status.HumanRequired {
			t.Fatalf("rollback=%v: %+v", rollback, status)
		}
	}
}
