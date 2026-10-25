package service

import (
	"context"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

// IntentSafetyError carries sanitized evidence for the independent audit journal.
type IntentSafetyError struct{ Check v1.IntentSafetyCheck }

func (e *IntentSafetyError) Error() string { return e.Check.Message }
func (e *IntentSafetyError) Unwrap() error { return model.Invalid("action-time safety check blocked") }

// intentSafety runs against the transaction's current record, never a provider.
func (s *Service) intentSafety(ctx context.Context, id string, revision int64, exception *v1.IntentSafetyException, recovery bool) (v1.IntentSafetyCheck, error) {
	now := s.Now().UTC()
	check := v1.IntentSafetyCheck{State: "initial_definition", CheckedAt: now, Message: "Initial definition has no baseline. Human approval permits creation, not a claim of operational health. Configuration rollback cannot remove a new resource."}
	r, err := s.Store.Read(ctx, id)
	if revision == 0 && errors.Is(err, model.ErrNotFound) {
		return check, nil
	}
	if err != nil {
		return check, err
	}
	if model.ConfigRevision(r.SLO) != revision {
		return check, model.ErrRevisionConflict
	}
	x, err := s.statusFromRecord(r, time.Time{})
	if err != nil {
		return check, err
	}
	q := s.qualityFromRecord(r, x, now, false)
	budget := s.budgetFromEvidence(r.SLO, x, q, false)
	check.Quality, check.Budget = &q, &budget
	check.MeasurementDigest = MeasurementDigest(r.SLO)
	check.WindowSeconds = r.SLO.WindowSeconds
	check.BurnState = x.Alert.Phase
	check.State = "allowed"
	check.Message = "Fresh saved evidence meets quantitative quality requirements and the configured budget policy. Normal approval and authorization still apply."
	if q.State != "usable" || x.Alert.Phase == "firing" || (budget.State != "not_configured" && budget.State != "proceed") {
		check.State = "blocked"
		check.Message = "Action-time safety check blocked this change. Review fresh measurement quality, burn-alert state and budget evidence; corrective work requires a separate owner-approved safety exception."
	}
	if recovery {
		check.State = "reviewed_rollback"
		check.Message = "Fresh evidence recorded for an independently approved configuration rollback. Recovery remains available during exhausted budgets or incomplete measurements; revision, source and authorization fences still apply."
		return check, nil
	}
	if check.State == "blocked" && exception != nil {
		p, ok := tenancy.FromContext(ctx)
		if ok && p.TenantID == s.TenantID && p.Role == "owner" && p.Subject != exception.ApprovedBy.Subject && exception.ApprovedBy.Subject != "" && exception.Reason != "" && !exception.ApprovedAt.After(now) && exception.ExpiresAt.After(now) && exception.ExpiresAt.Sub(exception.ApprovedAt) <= 30*time.Minute {
			check.State = "exception"
			check.Message = "A distinct tenant owner is applying an explicitly approved, unexpired corrective-work exception. Fresh adverse evidence is retained; no source, revision or authorization fence is waived."
			check.Exception = exception
		}
	}
	if check.State == "blocked" {
		return check, &IntentSafetyError{Check: check}
	}
	return check, nil
}
