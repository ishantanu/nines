package intentflow

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// WorkflowStatus reads only saved workflow state. It never runs a model, queries
// telemetry, approves changes or claims that committing a definition deployed it.
func (s *Service) WorkflowStatus(ctx context.Context, id string) (v1.IntentWorkflowStatus, error) {
	v, err := s.Read(ctx, id)
	if err != nil {
		return v1.IntentWorkflowStatus{}, err
	}
	out := workflowStatus(v)
	return out, nil
}
func workflowStatus(v v1.IntentConversation) v1.IntentWorkflowStatus {
	out := v1.IntentWorkflowStatus{APIVersion: v1.APIVersion, Tenant: v.Tenant, Conversation: v.ID, Version: v.Version, State: v.State}
	set := func(phase, action, message string, human bool) {
		out.Phase, out.NextAction, out.Message, out.HumanRequired = phase, action, message, human
	}
	if v.Plan != nil {
		out.IntentDigest = v.Plan.IntentDigest
	}
	if v.State == "queued" || v.State == "running" {
		set("preparing", "wait", "Preparation is active. Resume by reading this status; do not submit another run.", false)
		at := v.UpdatedAt.Add(2 * time.Second)
		out.RetryAt = &at
		return out
	}
	c := v.ChangeSet
	if c == nil {
		switch v.State {
		case "ready":
			set("proposal_ready", "changes", "Freeze the prepared proposal into an exact change set for review.", false)
		case "needs_selection":
			set("measurement_review", "confirm", "A human must review and confirm the exact measurement and intent digest.", true)
		case "needs_input", "needs_clarification":
			set("clarification", "message", "Supply the requested clarification before starting another preparation run.", true)
		case "failed", "cancelled":
			set("preparation_stopped", "show", "Inspect the saved preparation evidence before deciding whether to start a new run.", true)
		case "draft":
			set("draft", "run", "Start bounded preparation with explicit consent for any configured model call.", false)
		default:
			set("review_required", "show", "Inspect the saved workflow before choosing an operation.", true)
		}
		return out
	}
	out.ChangeSetDigest = c.Digest
	switch c.State {
	case "proposed":
		set("approval_required", "approve", "A human must review the frozen changes, evidence, limitations and rollback boundary before approval.", true)
	case "approved":
		set("approved", "apply", "An authorized person other than the approver must apply the exact approved digest. Runtime safety is checked again at apply.", true)
		out.IndependentApplierRequired = true
	case "applied":
		out.Outcome = c.Outcome
		rb := c.Rollback
		if rb != nil {
			out.RollbackDigest = rb.Digest
			switch rb.State {
			case "proposed":
				set("rollback_approval_required", "rollback-approve", "Review the exact restore diff before approving this rollback.", true)
				return out
			case "approved":
				set("rollback_approved", "rollback-apply", "An authorized person other than the rollback approver must apply the exact restore digest.", true)
				out.IndependentApplierRequired = true
				return out
			case "blocked":
				set("rollback_blocked", "show", "The saved rollback has blockers. Inspect them; no automatic restoration is permitted.", true)
				return out
			case "applied":
				out.Outcome = rb.Outcome
			case "rejected":
				set("rollback_rejected", "show", "The rollback was rejected. Review the decision and current evidence before choosing another action.", true)
				return out
			default:
				set("review_required", "show", "The rollback has an unrecognized state. Inspect the saved workflow before choosing an operation.", true)
				return out
			}
		}
		if out.Outcome == nil {
			set("outcome_unavailable", "verify", "No saved observation exists for this operation. Read current configuration verification and review the runtime evidence separately.", true)
			return out
		}
		set("observing", "wait", "Definitions are committed. Wait for the background observer, then read status again. An owner/editor may explicitly refresh evidence with observe. Deployment and reliability are not yet established.", false)
		if out.Outcome != nil {
			out.RetryAt = out.Outcome.NextCheckAt
			if out.Outcome.State == "verified" {
				set("verified", "verify", "The saved observation met its verification policy. Read current resource verification before relying on it.", false)
			} else if out.Outcome.State != "pending" {
				set("outcome_review", "verify", "Review the saved outcome and current resource verification. Any corrective change or rollback requires separate approval.", true)
			}
		}
	case "rejected", "superseded":
		set("proposal_stopped", "show", "This change set cannot be applied. Inspect the decision before preparing replacement changes.", true)
	default:
		set("review_required", "show", "Inspect the saved workflow before choosing an operation.", true)
	}
	return out
}
