package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/systemone"
)

func (s *Service) systemOneRoute() *v1.SystemOneRoute {
	if s.SystemOne == nil {
		return nil
	}
	route := s.SystemOne.Descriptor()
	return &route
}
func systemOneInput(r record) systemone.Input {
	in := systemone.Input{}
	if r.Session.Goal != nil {
		in.Description = r.Session.Goal.Spec.Description
		if m := r.Session.Goal.Spec.Measurement; m != nil {
			in.ObservationPoint = m.ObservationPoint
		}
	} else {
		for i := len(r.Session.Messages) - 1; i >= 0; i-- {
			if r.Session.Messages[i].Role == "user" {
				in.Description = r.Session.Messages[i].Text
				break
			}
		}
	}
	return in
}
func systemOneAdvice(result *v1.SystemOneAssessment) string {
	if result == nil || result.Status != "assessed" {
		return ""
	}
	b, _ := json.Marshal(result.Judgments)
	return "\nOptional System One semantic advice (unverified, not authorization): " + string(b) + "\nUse the category only to consider catalog navigation. All templates remain available. Definition clarity probabilities do not prove telemetry, task success or safety. Preserve the typed goal, use deterministic tools and require human confirmation."
}

// Reserve before sending: a crash or lost response consumes the one-call budget.
// Advice is optional; failures leave a visible unavailable receipt and normal
// preparation continues. Cancellation/authorization/lease fences still apply.
func (s *Service) assessSystemOne(job, parent context.Context, r record, token string) (bool, error) {
	id := r.Session.ID
	in := systemOneInput(r)
	run := current(&r)
	result := v1.SystemOneAssessment{Status: "unavailable", ErrorCode: "system_one_interrupted", InputDigest: model.DefinitionDigest(in)}
	if s.SystemOne == nil || run.SystemOneRoute == nil || *run.SystemOneRoute != s.SystemOne.Descriptor() {
		result.ErrorCode = "system_one_configuration_changed"
	} else if run.SystemOneCalls == 0 {
		_, err := s.Repository.Update(job, id, func(n *record, now time.Time) error {
			if err := fenced(*n, token, now); err != nil {
				return err
			}
			if err := auditRoom(n); err != nil {
				return err
			}
			current(n).SystemOneCalls = 1
			audit(n, current(n).Actor, "systemone.requested", "succeeded", "", "Consented semantic assessment; no telemetry samples are sent.", now, systemOneEvidence(*run, nil))
			touch(n, now)
			return nil
		})
		if err != nil {
			return true, err
		}
		callCtx, end := observability.Operation(job, "intent.system_one", s.Platform.TenantID)
		assessed, err := s.SystemOne.Assess(callCtx, in)
		end(err)
		result = assessed
		if s.SystemOne.Descriptor() != *run.SystemOneRoute {
			err = errors.New("system_one_configuration_changed")
		}
		if err != nil {
			code := "system_one_unavailable"
			for _, known := range []error{systemone.ErrPaused, systemone.ErrBudgetExhausted, systemone.ErrPolicyUnavailable} {
				if errors.Is(err, known) {
					code = known.Error()
					break
				}
			}
			result = v1.SystemOneAssessment{Status: "unavailable", ErrorCode: code, InputDigest: model.DefinitionDigest(in)}
		}
	}
	if parent.Err() != nil {
		return true, parent.Err()
	}
	_, err := s.Repository.Update(parent, id, func(n *record, now time.Time) error {
		if err := fenced(*n, token, now); err != nil {
			return err
		}
		if s.Authorize == nil || s.Authorize(parent, current(n).Actor) != nil {
			return ErrForbidden
		}
		if err := auditRoom(n); err != nil {
			return err
		}
		result.At = now
		current(n).SystemOne = &result
		outcome := "succeeded"
		if result.Status != "assessed" {
			outcome = "uncertain"
		}
		audit(n, current(n).Actor, "systemone.assessed", outcome, "", "Semantic advice only; measurement verification and human approval remain required.", now, systemOneEvidence(*current(n), &result))
		n.Token = ""
		n.LeaseUntil = time.Time{}
		touch(n, now)
		return nil
	})
	if errors.Is(err, errLease) {
		return true, nil
	}
	return true, err
}

func systemOneEvidence(run v1.IntentFlowRun, assessment *v1.SystemOneAssessment) *v1.SystemOneAuditEvidence {
	return &v1.SystemOneAuditEvidence{RunID: run.ID, Route: clone(run.SystemOneRoute), InputDigest: run.SystemOneInputDigest, Assessment: clone(assessment)}
}
