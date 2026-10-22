package investigation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/reviewskills"
)

var ErrForbidden = errors.New("review run access denied")
var errLeaseLost = errors.New("review run lease no longer valid")
var errAttemptLimit = errors.New("read-only step attempt limit reached")

func (s *Service) CreateRun(ctx context.Context, id string, in v1.CreateInvestigationRun) (v1.InvestigationRun, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.InvestigationRun{}, err
	}
	if !identifier.MatchString(in.ID) || in.ExpectedVersion < 1 {
		return v1.InvestigationRun{}, model.Invalid("run requires an ID and positive expectedVersion")
	}
	review, err := s.Read(ctx, id)
	if err != nil {
		return v1.InvestigationRun{}, err
	}
	if in.Mode != "" && in.Mode != "deterministic" && in.Mode != "model" || in.Mode != "model" && in.Question != "" || in.Mode == "model" && !validText(in.Question, 4000) {
		return v1.InvestigationRun{}, model.Invalid("choose deterministic or model mode; model mode requires a question of 1–4000 UTF-8 bytes")
	}
	candidate := v1.InvestigationRun{Tenant: s.Platform.TenantID, ID: in.ID, InvestigationID: id, ReviewVersion: in.ExpectedVersion, RequestedBy: actor, Scope: review.Scope, Mode: in.Mode, Question: in.Question, Skill: in.Skill, SkillExplicit: in.Skill != nil}
	if old, err := s.Repository.ReadRun(ctx, id, in.ID); err == nil {
		if !sameRun(old, candidate) {
			return v1.InvestigationRun{}, ErrConflict
		}
		return old, nil
	} else if err != ErrNotFound {
		return v1.InvestigationRun{}, err
	}
	if candidate.Skill == nil {
		ref := reviewskills.Default(in.Mode)
		candidate.Skill = &ref
	}
	candidate.SkillInputDigest = skillInputDigest(candidate)
	if err := checkSkill(candidate); err != nil {
		return v1.InvestigationRun{}, model.Invalid("skill must match an exact available version/digest, mode and scope")
	}
	now := time.Now().UTC()
	candidate.APIVersion = v1.APIVersion
	candidate.Version = 1
	candidate.Status = "queued"
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	candidate.DeadlineAt = now.Add(5 * time.Minute)
	candidate.Steps = []v1.InvestigationRunStep{{Tool: "read_context", Status: "pending"}, {Tool: "assess_evidence", Status: "pending"}}
	if in.Mode == "model" {
		if s.Model == nil {
			return candidate, rm.Failure("model_not_configured")
		}
		route := s.Model.Descriptor()
		candidate.ModelRoute = &route
		candidate.ModelPolicy = modelPolicyFor(candidate)
		candidate.Steps = append(candidate.Steps, v1.InvestigationRunStep{Tool: "model_turn", Status: "pending"})
	}
	return s.Repository.CreateRun(ctx, candidate)
}
func (s *Service) ReadRun(ctx context.Context, review, id string) (v1.InvestigationRun, error) {
	if !identifier.MatchString(review) || !identifier.MatchString(id) {
		return v1.InvestigationRun{}, model.Invalid("invalid review or run ID")
	}
	r, err := s.Repository.ReadRun(ctx, review, id)
	if err == nil && r.Tenant != s.Platform.TenantID {
		return v1.InvestigationRun{}, ErrNotFound
	}
	return r, err
}
func (s *Service) ListRuns(ctx context.Context, review string) (v1.InvestigationRunList, error) {
	out := v1.InvestigationRunList{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Items: []v1.InvestigationRun{}}
	if _, err := s.Read(ctx, review); err != nil {
		return out, err
	}
	items, err := s.Repository.ListRuns(ctx, review)
	if err != nil {
		return out, err
	}
	for _, r := range items {
		if r.Tenant != s.Platform.TenantID {
			return out, ErrNotFound
		}
	}
	out.Items = items
	return out, nil
}
func (s *Service) CancelRun(ctx context.Context, review, id string, in v1.CancelInvestigationRun) (v1.InvestigationRun, error) {
	actor, err := s.actor(ctx)
	if err != nil {
		return v1.InvestigationRun{}, err
	}
	if !identifier.MatchString(review) || !identifier.MatchString(id) || in.ExpectedVersion < 1 {
		return v1.InvestigationRun{}, model.Invalid("cancel requires valid IDs and positive expectedVersion")
	}
	return s.Repository.updateRun(ctx, review, id, func(r *runRecord) error {
		if r.Run.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		p, _ := tenancy.FromContext(ctx)
		if p.Role != "owner" && (r.Run.RequestedBy.Subject != actor.Subject || r.Run.RequestedBy.Kind != actor.Kind) {
			return ErrForbidden
		}
		if r.Run.Status == "cancelled" {
			return nil
		}
		if terminal(r.Run.Status) || r.Run.Version != in.ExpectedVersion {
			return ErrConflict
		}
		r.Run.CancelledBy = &actor
		finish(r, "cancelled", "")
		return nil
	})
}

// Worker executes exactly one allowlisted step per claim. Results become visible
// only after a fenced checkpoint commits. Completed steps are never re-executed.
type Worker struct {
	Service *Service
	execute func(context.Context, v1.InvestigationRun, int) (v1.InvestigationRunStep, error)
}

func (s *Service) RunWorker(ctx context.Context, log *slog.Logger) {
	worker := Worker{Service: s}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		found, err := worker.WorkOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.ErrorContext(ctx, "review worker operation failed", "tenant", s.Platform.TenantID)
		}
		if found && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) WorkOne(ctx context.Context) (bool, error) {
	s := w.Service
	token := model.NewID()
	r, err := s.Repository.claimRun(ctx, token)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if terminal(r.Run.Status) {
		return true, nil
	}
	stepTimeout := 15 * time.Second
	if r.Run.Mode == "model" {
		stepTimeout = 60 * time.Second
	}
	deadline := time.Now().Add(stepTimeout)
	if r.Run.DeadlineAt.Before(deadline) {
		deadline = r.Run.DeadlineAt
	}
	jobCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if r.Run.Mode == "model" {
		go s.watchCancellation(jobCtx, cancel, r.Run.InvestigationID, r.Run.ID)
	}
	jobCtx, end := observability.Operation(jobCtx, "review.read_only_step", s.Platform.TenantID)
	var resultErr error
	defer func() { end(resultErr) }()
	// Revalidate the initiating identity on every step, including recovery. No
	// credential or broad service principal is stored in the run document.
	if s.AuthorizeRun == nil || r.Run.Tenant != s.Platform.TenantID {
		err = ErrForbidden
	} else {
		err = s.AuthorizeRun(jobCtx, r.Run.RequestedBy)
	}
	index := -1
	for i, step := range r.Run.Steps {
		if step.Status != "completed" {
			index = i
			break
		}
	}
	if err == nil {
		err = checkSkill(r.Run)
	}
	if err == nil && r.Run.Mode == "model" && (s.Model == nil || r.Run.ModelRoute == nil || s.Model.Descriptor() != *r.Run.ModelRoute || r.Run.ModelPolicy != modelPolicyFor(r.Run)) {
		err = rm.Failure("model_configuration_changed")
	}
	if err == nil && (index < 0 || len(r.Run.Steps) != 2 && !modelStepAllowed(r.Run)) {
		err = model.Invalid("invalid read-only plan")
	}
	var request rm.Request
	if err == nil {
		_, err = s.Repository.updateRun(jobCtx, r.Run.InvestigationID, r.Run.ID, func(current *runRecord) error {
			if e := fenced(current, token); e != nil {
				return e
			}
			step := &current.Run.Steps[index]
			if step.Status == "completed" {
				return errLeaseLost
			}
			if step.Attempts >= 3 {
				return errAttemptLimit
			}
			if index >= 2 && step.Tool == "model_turn" {
				var err error
				request, err = prepareTurn(current)
				if err != nil {
					return err
				}
			}
			step.Attempts++
			step.Status = "running"
			current.Run.UpdatedAt = current.Now
			current.Run.Version++
			r = copyRunRecord(*current)
			return nil
		})
	}
	var result v1.InvestigationRunStep
	var outcome modelOutcome
	if err == nil {
		if index >= 2 && r.Run.Mode == "model" {
			result, outcome, err = s.executeModelStep(jobCtx, r, index, request, s.modelProgress(jobCtx, r.Run.InvestigationID, r.Run.ID, token))
		} else if w.execute != nil {
			result, err = w.execute(jobCtx, r.Run, index)
		} else {
			result, err = s.executeReadStep(jobCtx, r.Run, index)
		}
	}
	resultErr = err
	if ctx.Err() != nil {
		return true, ctx.Err()
	} // Shutdown leaves a recoverable lease.
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	_, saveErr := s.Repository.updateRun(saveCtx, r.Run.InvestigationID, r.Run.ID, func(current *runRecord) error {
		if e := fenced(current, token); e != nil {
			return e
		}
		if !current.Run.DeadlineAt.After(current.Now) {
			finish(current, "failed", "deadline_exceeded")
			return nil
		}
		if err != nil {
			var validation *model.ValidationError
			var modelError *rm.Error
			code := "evidence_unavailable"
			permanent := false
			switch {
			case errors.As(err, &modelError):
				code = modelError.Code
				permanent = code != "model_unavailable"
			case errors.Is(err, errAttemptLimit):
				code = "attempt_limit"
				permanent = true
			case errors.As(err, &validation):
				code = "scope_unavailable"
				permanent = true
			case errors.Is(err, ErrForbidden):
				code = "access_revoked"
				permanent = true
			case errors.Is(err, model.ErrRevisionConflict):
				code = "scope_revision_conflict"
				permanent = true
			case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrJourneyNotFound):
				code = "resource_not_found"
				permanent = true
			}
			if index < 0 || index >= len(current.Run.Steps) || current.Run.Steps[index].Attempts >= 3 {
				permanent = true
			}
			if permanent {
				finish(current, "failed", code)
			} else {
				current.Token = ""
				current.Until = current.Now
				current.Run.ErrorCode = code
				current.Run.Version++
				current.Run.UpdatedAt = current.Now
			}
			return nil
		}
		if r.Run.Mode == "model" && index >= 2 {
			applyModelOutcome(current, index, outcome)
		}
		result.Status = "completed"
		result.Attempts = current.Run.Steps[index].Attempts
		at := current.Now
		result.CompletedAt = &at
		current.Run.Steps[index] = result
		current.Run.ErrorCode = ""
		current.Token = ""
		current.Until = current.Now
		current.Run.UpdatedAt = current.Now
		current.Run.Version++
		if current.Run.Mode == "model" && current.Run.ModelReview != nil || current.Run.Mode != "model" && index == len(current.Run.Steps)-1 {
			if skillErr := completeSkill(current); skillErr != nil {
				var typed *rm.Error
				code := "skill_output_invalid"
				if errors.As(skillErr, &typed) {
					code = typed.Code
				}
				current.Run.ModelReview = nil
				finish(current, "failed", code)
				return nil
			}
			finish(current, "completed", "")
		}
		return nil
	})
	if errors.Is(saveErr, errLeaseLost) {
		return true, nil
	}
	return true, saveErr
}
func fenced(r *runRecord, token string) error {
	if terminal(r.Run.Status) || r.Token != token || !r.Until.After(r.Now) {
		return errLeaseLost
	}
	return nil
}
func (s *Service) executeReadStep(ctx context.Context, r v1.InvestigationRun, i int) (v1.InvestigationRunStep, error) {
	if r.Mode != "model" && !skillAllows(r, r.Steps[i].Tool) {
		return v1.InvestigationRunStep{}, rm.Failure("unsafe_tool_call")
	}
	if i == 0 && r.Steps[i].Tool == "read_context" {
		b, err := s.context(ctx, r.Scope)
		if err != nil {
			return v1.InvestigationRunStep{}, err
		}
		digest := sha256.Sum256(b)
		return v1.InvestigationRunStep{Tool: "read_context", Evidence: &v1.InvestigationEvidence{CapturedAt: time.Now().UTC(), Digest: hex.EncodeToString(digest[:]), Context: b}}, nil
	}
	if i == 1 && r.Steps[i].Tool == "assess_evidence" && r.Steps[0].Evidence != nil {
		assessment, err := assess(r.Scope, *r.Steps[0].Evidence)
		return v1.InvestigationRunStep{Tool: "assess_evidence", Assessment: assessment}, err
	}
	return v1.InvestigationRunStep{}, model.Invalid("tool is not in the read-only plan")
}
func assess(scope v1.InvestigationScope, e v1.InvestigationEvidence) (*v1.EvidenceAssessment, error) {
	digest := sha256.Sum256(e.Context)
	if hex.EncodeToString(digest[:]) != e.Digest {
		return nil, model.Invalid("evidence digest mismatch")
	}
	out := &v1.EvidenceAssessment{EvidenceDigest: e.Digest, State: "insufficient_evidence", Findings: []v1.EvidenceFinding{}, Limitations: []string{"This assessment uses stored point-in-time context, not a new provider query or a causal diagnosis.", "Window comparisons do not establish end-to-end user journey success."}}
	add := func(c v1.SLOContext) {
		finding := v1.EvidenceFinding{ResourceID: c.Resource.ID, State: "insufficient_evidence", Message: "The stored window does not establish an SLI comparison. Missing data remains unknown."}
		m := c.Measurement
		if c.MeasurementHealth != nil && c.MeasurementHealth.EvaluationBlocked {
			finding.Message = "The measurement contract is incompatible or unverified. A target comparison is not established."
		} else if m.DataStatus == "no_traffic" {
			finding.Message = "No traffic was observed. Zero traffic does not establish success."
		} else if m.DataStatus == "complete" && m.Coverage >= 1 && m.Reliability != nil {
			finding.State = "window_within_target"
			if *m.Reliability < m.Target {
				finding.State = "window_below_target"
			}
			finding.Message = fmt.Sprintf("Stored window SLI %.3f%% compared with target %.3f%%.", *m.Reliability*100, m.Target*100)
		}
		out.Findings = append(out.Findings, finding)
		out.Limitations = append(out.Limitations, c.Warnings...)
	}
	switch scope.Kind {
	case v1.SLOKind:
		var c v1.SLOContext
		if err := json.Unmarshal(e.Context, &c); err != nil {
			return nil, err
		}
		add(c)
		out.State = out.Findings[0].State
	case v1.JourneyKind:
		var c v1.JourneyContext
		if err := json.Unmarshal(e.Context, &c); err != nil {
			return nil, err
		}
		out.State = "linked_objectives_assessed"
		for _, link := range c.Links {
			if link.Context != nil {
				add(*link.Context)
				if out.Findings[len(out.Findings)-1].State == "insufficient_evidence" {
					out.State = "insufficient_evidence"
				}
			} else {
				out.State = "insufficient_evidence"
				out.Findings = append(out.Findings, v1.EvidenceFinding{ResourceID: link.Link.ID, State: "insufficient_evidence", Message: "Linked objective context is unavailable."})
			}
		}
		if c.NextCursor != "" || len(c.Links) == 0 {
			out.State = "insufficient_evidence"
		}
		if c.NextCursor != "" {
			out.Limitations = append(out.Limitations, "Only the first ten linked objectives were captured.")
		}
		out.Limitations = append(out.Limitations, c.Warnings...)
	default:
		return nil, model.Invalid("unsupported evidence kind")
	}
	return out, nil
}
