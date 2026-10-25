package service

import (
	"context"
	"math"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func (s *Service) budgetFromEvidence(c model.SLO, x model.Snapshot, q v1.MeasurementQuality, historical bool) v1.ErrorBudgetDecision {
	d := v1.ErrorBudgetDecision{APIVersion: v1.APIVersion, Tenant: s.TenantID, SLOID: c.ID, Revision: model.ConfigRevision(c), DefinitionDigest: q.DefinitionDigest, SourceDigest: q.SourceDigest, ObservedAt: q.ObservedAt, EvaluatedAt: x.At, State: "not_configured", ReasonCodes: []string{"policy_not_configured"}, Message: "No error-budget response policy is configured. No operational permission is implied.", BudgetRemainingFraction: x.BudgetRemainingFraction, QualityState: q.State, Advisory: true}
	d.Evidence = &v1.BudgetEvidence{Coverage: q.Coverage, Total: q.Total, MinimumEvents: q.MinimumEvents, Freshness: q.Freshness, Checks: q.Checks}
	p := c.ErrorBudgetPolicy
	if p == nil {
		return d
	}
	d.Owner, d.RunbookURL, d.PolicyDigest = p.Owner, p.RunbookURL, model.DefinitionDigest(p)
	d.State = "insufficient_evidence"
	d.ReasonCodes = []string{}
	d.Message = "Current measurement evidence is insufficient for an operational decision. Follow the runbook and review the quality checks."
	if historical {
		d.ReasonCodes = append(d.ReasonCodes, "historical_view")
		return d
	}
	if q.State != "usable" || x.BudgetRemainingFraction == nil || !model.Finite(*x.BudgetRemainingFraction) {
		for _, check := range q.Checks {
			if check.State == "failed" || check.State == "warning" {
				d.ReasonCodes = append(d.ReasonCodes, check.Code)
			}
		}
		if len(d.ReasonCodes) == 0 {
			d.ReasonCodes = append(d.ReasonCodes, "budget_unavailable")
		}
		return d
	}
	if !c.AlertsEnabled() {
		d.State = "review_required"
		d.ReasonCodes = []string{"objective_not_committed"}
		d.Message = "This objective is not committed. Its budget is a measurement preview; an owner must review operational use."
		return d
	}
	if *x.BudgetRemainingFraction <= 0 {
		d.State = p.OnExhausted
		if d.State == "" {
			d.State = "blocked"
		}
		d.ReasonCodes = []string{"budget_exhausted"}
		d.Message = "The observed error budget is exhausted. Apply the configured response and follow the runbook."
		return d
	}
	review := *x.BudgetRemainingFraction <= p.ReviewBelow
	if x.BudgetRemaining != nil && x.BudgetAllowed != nil {
		tolerance := 8 * (math.Nextafter(x.Total, math.Inf(1)) - x.Total)
		review = *x.BudgetRemaining <= p.ReviewBelow**x.BudgetAllowed+tolerance
	}
	if review {
		d.State = "review_required"
		d.ReasonCodes = []string{"remaining_budget_below_review_threshold"}
		d.Message = "Remaining error budget is at or below the review threshold. Obtain an owner decision before proceeding."
		return d
	}
	d.State = "proceed"
	d.ReasonCodes = []string{"budget_within_policy"}
	d.Message = "Current quantitative evidence meets the configured budget policy. Deployment authorization and change approval remain separate."
	return d
}
func (s *Service) ErrorBudgetDecision(ctx context.Context, id string, expected int64) (v1.ErrorBudgetDecision, error) {
	if expected < 0 {
		return v1.ErrorBudgetDecision{}, model.Invalid("expectedRevision must be nonnegative")
	}
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return v1.ErrorBudgetDecision{}, err
	}
	if expected > 0 && model.ConfigRevision(r.SLO) != expected {
		return v1.ErrorBudgetDecision{}, model.ErrRevisionConflict
	}
	x, err := s.statusFromRecord(r, time.Time{})
	if err != nil {
		return v1.ErrorBudgetDecision{}, err
	}
	q := s.qualityFromRecord(r, x, s.Now(), false)
	return s.budgetFromEvidence(r.SLO, x, q, false), nil
}
