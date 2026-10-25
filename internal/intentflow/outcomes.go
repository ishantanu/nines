package intentflow

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
)

func observationPolicy(step int64) *v1.IntentObservationPolicy {
	warmup, observe := max(120, 2*step), max(300, 5*step)
	return &v1.IntentObservationPolicy{WarmupSeconds: warmup, ObserveSeconds: observe, DeadlineSeconds: warmup + observe + 3600}
}
func newOutcome(session v1.IntentConversation, c v1.IntentChangeSet, operation, digest string, at time.Time, resources []v1.IntentAppliedResource) *v1.IntentOutcome {
	if c.ObservationPolicy == nil {
		return nil
	}
	rollbackStatus := "separate_review_required"
	if c.Recommendation != nil && c.Recommendation.RollbackStatus != "restorable_updates" {
		rollbackStatus = c.Recommendation.RollbackStatus
	}
	for _, r := range resources {
		if r.Kind == v1.SLOKind {
			return &v1.IntentOutcome{APIVersion: v1.APIVersion, Tenant: session.Tenant, Conversation: session.ID, ChangeSetDigest: c.Digest, Operation: operation, OperationDigest: digest, SLOID: r.Name, Revision: r.Revision, DefinitionDigest: r.Digest, SourceDigest: c.SourceDigest, State: "pending", Code: "warmup", Message: "Configuration committed. Waiting for warmup and fresh runtime/SLI evidence; no health outcome is claimed.", StartedAt: at, CheckedAt: at, WarmupUntil: at.Add(time.Duration(c.ObservationPolicy.WarmupSeconds) * time.Second), DeadlineAt: at.Add(time.Duration(c.ObservationPolicy.DeadlineSeconds) * time.Second), Comparison: "not_comparable", RollbackStatus: rollbackStatus, Limitations: []string{"Observed changes in rolling attainment do not establish causality; the window can include traffic before this configuration change.", "Configuration rollback cannot recover consumed error budget or undo creation. The observer never deploys, reloads or restores resources.", "Verification requires current rule evaluation and query-dataset output as well as usable saved SLI evidence. Notification delivery is not verified."}}
		}
	}
	return nil
}
func pendingOutcome(r *record, digest string) (*v1.IntentChangeSet, *v1.IntentOutcome, *v1.IntentSafetyCheck) {
	changes := []*v1.IntentChangeSet{}
	if r.Session.ChangeSet != nil {
		changes = append(changes, r.Session.ChangeSet)
	}
	for i := range r.Session.ChangeSetHistory {
		changes = append(changes, &r.Session.ChangeSetHistory[i])
	}
	for _, c := range changes {
		if c.State != "applied" {
			continue
		}
		if rb := c.Rollback; rb != nil && rb.State == "applied" {
			if rb.Outcome != nil && ((digest == "" && rb.Outcome.State == "pending") || rb.Digest == digest || c.Digest == digest) {
				return c, rb.Outcome, rb.SafetyCheck
			}
			continue
		}
		if c.Outcome != nil && (digest == "" || c.Digest == digest) {
			if digest != "" || c.Outcome.State == "pending" {
				return c, c.Outcome, c.SafetyCheck
			}
		}
	}
	return nil, nil, nil
}
func (s *Service) inspectOutcome(ctx context.Context, c v1.IntentChangeSet, old v1.IntentOutcome, baseline *v1.IntentSafetyCheck) (v1.IntentOutcome, error) {
	out := clone(old)
	now := s.Platform.Now().UTC()
	out.CheckedAt = now
	if now.Sub(old.CheckedAt) > max(90*time.Second, 2*time.Duration(c.ObservationPolicy.ObserveSeconds/5)*time.Second) {
		out.HealthySince = nil
	}
	out.State = "pending"
	out.Code = "waiting_evidence"
	out.Message = "Waiting for complete, fresh SLI evidence and verified current rule evaluation/output."
	out.Quality = nil
	out.Budget = nil
	out.Deployment = nil
	out.Evaluation = nil
	out.RuleOutput = nil
	out.Reliability = nil
	end := func() (v1.IntentOutcome, error) {
		if out.State == "pending" && !now.Before(out.DeadlineAt) {
			out.State = "insufficient_evidence"
			out.Message = "The observation deadline elapsed without sufficient verified evidence. Review the recorded checks and decide whether corrective work or rollback is appropriate."
		}
		out.NextCheckAt = nil
		if out.State == "pending" {
			next := now.Add(30 * time.Second)
			out.NextCheckAt = &next
		}
		return out, nil
	}
	view, err := s.Platform.ReadSLO(ctx, out.SLOID)
	if errors.Is(err, model.ErrNotFound) {
		out.State = "drifted"
		out.Code = "definition_missing"
		out.Message = "The applied SLO no longer exists."
		return end()
	}
	if err != nil {
		return out, err
	}
	if model.ConfigRevision(view.SLO) != out.Revision || hash(model.DefinitionFromSLO(view.SLO)) != out.DefinitionDigest || s.Platform.SourceFingerprint(view.SLO.Source) != out.SourceDigest {
		out.State = "drifted"
		out.Code = "definition_or_source_changed"
		out.Message = "The applied definition or source changed. Earlier runtime evidence cannot verify this operation."
		out.HealthySince = nil
		return end()
	}
	out.Quality = view.Quality
	out.Budget = view.BudgetDecision
	out.Reliability = view.Status.Reliability
	out.BurnState = view.Status.Alert.Phase
	if baseline != nil && baseline.Quality != nil && baseline.Quality.State == "usable" && baseline.Quality.Total > 0 && baseline.WindowSeconds == view.SLO.WindowSeconds && baseline.MeasurementDigest == service.MeasurementDigest(view.SLO) && baseline.Quality.SourceDigest == out.SourceDigest {
		before := baseline.Quality.Good / baseline.Quality.Total
		out.BaselineReliability = &before
		out.Comparison = "comparable_rolling_windows"
	}
	if now.Before(out.WarmupUntil) {
		out.Code = "warmup"
		out.Message = "Waiting for the approved warmup period before runtime inspection."
		return end()
	}
	integration, err := s.Platform.IntegrationStatus(ctx, out.SLOID)
	if err != nil {
		return out, err
	}
	if integration.Revision != out.Revision {
		return out, model.ErrRevisionConflict
	}
	out.Deployment = &integration.Deployment
	out.Evaluation = &integration.Evaluation
	out.RuleOutput = integration.RuleOutput
	// Reject concurrent source/definition changes before accepting provider evidence.
	current, err := s.Platform.Store.Read(ctx, out.SLOID)
	if err != nil {
		return out, err
	}
	if model.ConfigRevision(current.SLO) != out.Revision || hash(model.DefinitionFromSLO(current.SLO)) != out.DefinitionDigest || s.Platform.SourceFingerprint(current.SLO.Source) != out.SourceDigest {
		out.State = "drifted"
		out.Code = "definition_or_source_changed"
		out.Message = "Source or definition changed during runtime inspection."
		out.HealthySince = nil
		return end()
	}
	fresh := view.Quality != nil && view.Quality.State == "usable" && !view.Quality.EvaluatedAt.Before(out.WarmupUntil) && view.Status.Reliability != nil
	runtime := integration.Evaluation.State == "evaluating" && integration.Evaluation.ObservedAt != nil && !integration.Evaluation.ObservedAt.Before(out.WarmupUntil) && integration.RuleOutput != nil && integration.RuleOutput.State == "verified" && integration.RuleOutput.ObservedAt != nil && !integration.RuleOutput.ObservedAt.Before(out.WarmupUntil) && (integration.Deployment.State == "written" || integration.Deployment.State == "stored" || integration.Deployment.State == "configured")
	if !fresh {
		out.HealthySince = nil
		return end()
	}
	if out.BurnState == "firing" {
		out.State = "review_required"
		out.Code = "active_burn_alert"
		out.Message = "A current burn-rate alert is firing. Healthy rolling attainment does not override the need for owner review."
		out.HealthySince = nil
		return end()
	}
	if out.BaselineReliability != nil && *out.Reliability+8*(math.Nextafter(1, 2)-1) < *out.BaselineReliability {
		out.State = "regressed"
		out.Code = "observed_attainment_decreased"
		out.Message = "Observed attainment decreased against a comparable pre-change baseline. This is correlation, not proof of causality. Review evidence and prepare a separately approved rollback if appropriate."
		out.HealthySince = nil
		return end()
	}
	if *out.Reliability < view.SLO.Target {
		out.State = "target_not_met"
		out.Code = "observed_sli_below_target"
		out.Message = "Verified current SLI remains below the target after warmup. Review the evidence and choose corrective work or a separately approved rollback."
		out.HealthySince = nil
		return end()
	}
	if out.BurnState == "pending" {
		out.Code = "burn_alert_pending"
		out.Message = "A burn-rate alert is pending; qualifying observation restarts only after it clears."
		out.HealthySince = nil
		return end()
	}
	if !runtime {
		out.HealthySince = nil
		out.Code = "rule_activation_unverified"
		out.Message = "SLI evidence is current, but rule storage, evaluation or query-output evidence is not yet verified."
		return end()
	}
	if out.Budget != nil && out.Budget.State != "not_configured" && out.Budget.State != "proceed" {
		out.State = "review_required"
		out.Code = "budget_response_required"
		out.Message = "Current measured SLI requires an owner response under the configured error-budget policy."
		out.HealthySince = nil
		return end()
	}
	if !now.Before(out.DeadlineAt) {
		out.Code = "observation_deadline"
		return end()
	}
	if out.HealthySince == nil {
		at := view.Quality.EvaluatedAt
		out.HealthySince = &at
	}
	if view.Quality.EvaluatedAt.Sub(*out.HealthySince) < time.Duration(c.ObservationPolicy.ObserveSeconds)*time.Second {
		out.Code = "observing"
		out.Message = "Fresh current SLI meets the target and rules are evaluating. Waiting for the full observation period; a single successful sample is insufficient."
		return end()
	}
	out.State = "verified"
	out.Code = "runtime_and_sli_verified"
	out.Message = "Current rules and query output are verified, and fresh observed SLI met the target across the observation checks. This does not prove the configuration caused an improvement."
	if out.BaselineReliability != nil && *out.Reliability > *out.BaselineReliability {
		out.Comparison = "observed_improvement"
	}
	return end()
}

// observeOutcome performs bounded provider reads outside the conversation/database
// transaction, then fences the persisted report against the captured operation.
func (s *Service) observeOutcome(ctx context.Context, id, digest string, expected int64, who v1.InvestigationActor) (v1.IntentConversation, error) {
	saved, err := s.Repository.Read(ctx, id)
	if err != nil {
		return v1.IntentConversation{}, err
	}
	if saved.Session.Tenant != s.Platform.TenantID {
		return v1.IntentConversation{}, ErrNotFound
	}
	if expected > 0 && saved.Session.Version != expected {
		return v1.IntentConversation{}, ErrConflict
	}
	c, old, baseline := pendingOutcome(&saved, digest)
	if old == nil {
		return saved.Session, model.Invalid("no tracked applied operation is available")
	}
	if old.State != "pending" {
		return saved.Session, nil
	}
	job, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	next, err := s.inspectOutcome(job, *c, *old, baseline)
	if err != nil {
		return saved.Session, err
	}
	result, err := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if r.Session.Version != saved.Session.Version {
			return ErrConflict
		}
		actual, report, _ := pendingOutcome(r, next.OperationDigest)
		if actual == nil || actual.Digest != c.Digest || report == nil || report.State != "pending" || !report.CheckedAt.Equal(old.CheckedAt) || report.Revision != next.Revision {
			return ErrConflict
		}
		if err := auditRoom(r); err != nil {
			return err
		}
		*report = next
		touch(r, now)
		action := "change_set.observed"
		if next.Operation == "rollback" {
			action = "rollback.observed"
		}
		result := "succeeded"
		if next.State != "pending" && next.State != "verified" {
			result = "blocked"
		}
		audit(r, who, action, result, next.OperationDigest, next.Message, now, next)
		return nil
	})
	return result.Session, err
}
func (s *Service) ObserveChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (out v1.IntentConversation, err error) {
	defer func() { s.auditFailure(ctx, id, "change_set.observed", in.Digest, err) }()
	who, err := human(ctx, s.Platform.TenantID)
	if err != nil {
		return out, err
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 || in.SafetyExceptionReason != "" {
		return out, model.Invalid("confirm the applied operation digest and conversation version")
	}
	return s.observeOutcome(ctx, id, in.Digest, in.ExpectedVersion, who)
}
func (s *Service) RunOutcomeObserver(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	after := ""
	for ctx.Err() == nil {
		cycle, cancel := context.WithTimeout(ctx, 25*time.Second)
		rows, err := s.Repository.List(cycle, after, 20)
		if err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "intent observation scan unavailable", "tenant", s.Platform.TenantID)
		}
		if len(rows) == 0 {
			after = ""
		}
		visited := 0
		for _, row := range rows {
			if cycle.Err() != nil {
				break
			}
			visited++
			after = row.ID
			r, readErr := s.Repository.Read(cycle, row.ID)
			if readErr != nil {
				continue
			}
			_, report, _ := pendingOutcome(&r, "")
			if report == nil || report.State != "pending" || (report.NextCheckAt != nil && report.NextCheckAt.After(s.Platform.Now())) {
				continue
			}
			_, err = s.observeOutcome(cycle, row.ID, report.OperationDigest, 0, v1.InvestigationActor{Subject: "nines-outcome-observer", Name: "Nines outcome observer", Kind: "system"})
			if err != nil && !errors.Is(err, ErrConflict) && cycle.Err() == nil {
				log.WarnContext(ctx, "intent outcome check incomplete", "tenant", s.Platform.TenantID)
			}
		}
		if len(rows) < 20 && visited == len(rows) {
			after = ""
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
