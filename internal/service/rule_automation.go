package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/store"
)

func (s *Service) RuleAutomationStatus(record model.Record, manager *rules.Manager) *v1.RuleAutomationStatus {
	if manager == nil {
		return nil
	}
	target, ok := manager.TargetFor(record.SLO)
	if !ok {
		return nil
	}
	rev := model.ConfigRevision(record.SLO)
	if !target.AutoDeploy {
		return &v1.RuleAutomationStatus{Revision: rev, IntegrationStage: v1.IntegrationStage{State: "manual", Code: "automatic_rules_disabled", Message: "Rules deploy only after an explicit reviewed apply.", NextAction: "A tenant owner can enable automatic deployment on this data source."}}
	}
	if old := record.RuleAutomation; old != nil && old.Paused {
		out := *old
		out.Enabled = true
		return &out
	}
	if old := record.RuleAutomation; old != nil && old.Revision == rev && old.TargetDigest == model.DefinitionDigest(target) {
		out := *old
		out.Enabled = true
		if (out.State == "configured" || out.State == "applying") && out.NextAttemptAt != nil && s.Now().After(out.NextAttemptAt.Add(30*time.Second)) {
			out.State = "stale"
			out.Code = "automatic_rules_overdue"
			out.Message = "The last rule deployment check is overdue."
			out.NextAction = "Check the collecting server and rule reconciliation worker; inspect runtime evidence independently."
		}
		return &out
	}
	return &v1.RuleAutomationStatus{Enabled: true, Revision: rev, IntegrationStage: v1.IntegrationStage{State: "pending", Code: "automatic_rules_pending", Message: fmt.Sprintf("Saved revision %d is queued for automatic rule deployment.", rev), NextAction: "The reconciler applies the saved definition. Check configuration, activation and runtime evidence separately."}}
}
func (s *Service) readRuleAutomation(ctx context.Context, id string) (*v1.RuleAutomationStatus, error) {
	if reader, ok := s.Store.(store.RuleAutomationReader); ok {
		return reader.RuleAutomation(ctx, id)
	}
	r, e := s.Store.Read(ctx, id)
	return r.RuleAutomation, e
}
func (s *Service) saveRuleAutomation(ctx context.Context, id string, revision int64, state *v1.RuleAutomationStatus) error {
	return s.Store.Update(ctx, id, func(r *model.Record) error {
		if model.ConfigRevision(r.SLO) != revision {
			return model.ErrRevisionConflict
		}
		r.RuleAutomation = state
		return nil
	})
}

// ReconcileRules is driven only by persisted desired definitions and an owner-
// configured connection policy. It never drafts, approves or changes an SLO.
func (s *Service) ReconcileRules(ctx context.Context, id string, manager *rules.Manager) (out *v1.RuleAutomationStatus, err error) {
	ctx, finish := observability.Operation(ctx, "rules.reconcile", s.TenantID)
	defer func() {
		observed := err
		if observed == nil && out != nil && (out.State == "failed" || out.State == "blocked") {
			observed = errors.New("automatic rule activation failed")
		}
		finish(observed)
	}()
	if manager == nil {
		return nil, model.ErrUnavailable
	}
	s.DefinitionMu.Lock()
	defer s.DefinitionMu.Unlock()
	record, e := s.Store.Read(ctx, id)
	if e != nil {
		return nil, e
	}
	target, ok := manager.TargetFor(record.SLO)
	if !ok || !target.AutoDeploy {
		return s.RuleAutomationStatus(record, manager), nil
	}
	if target.Kind == "prometheus" && !target.Reload {
		return nil, model.Invalid("automatic Prometheus deployment requires reload on the configured destination")
	}
	state := s.RuleAutomationStatus(record, manager)
	if state.Paused {
		return state, nil
	}
	now := s.Now().UTC()
	if state.NextAttemptAt != nil && state.NextAttemptAt.After(now) {
		return state, nil
	}
	state.Enabled = true
	state.Revision = model.ConfigRevision(record.SLO)
	state.TargetDigest = model.DefinitionDigest(target)
	state.Attempts++
	state.LastAttemptAt = &now
	next := now.Add(30 * time.Second)
	state.NextAttemptAt = &next
	state.IntegrationStage = v1.IntegrationStage{State: "applying", Code: "automatic_rules_applying", Message: "Applying rules for the saved SLO revision.", ObservedAt: &now, NextAction: "Wait for the attempt; a saved definition alone does not prove runtime evaluation."}
	if e = s.saveRuleAutomation(ctx, id, state.Revision, state); e != nil {
		return state, e
	}
	attempt, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	plan, e := manager.PlanContext(attempt, record.SLO)
	if e == nil && !plan.Target.AutoDeploy {
		e = model.ErrRevisionConflict
	}
	var result rules.Deployment
	if e == nil {
		state.Digest = plan.Digest
		result, e = manager.Deploy(attempt, record.SLO, plan.Digest)
	}
	completed := s.Now().UTC()
	state.IntegrationStage = v1.IntegrationStage{State: "configured", Code: "automatic_rules_applied", Message: fmt.Sprintf("Rules for saved revision %d were applied to the configured destination. Runtime evaluation is checked separately.", state.Revision), ObservedAt: &completed, NextAction: "Inspect loaded-rule evaluation and query-output evidence; configuration does not establish SLO attainment."}
	next = completed.Add(time.Minute)
	if e != nil {
		state.State = "failed"
		state.Code = "automatic_rules_failed"
		state.Message = "Automatic rule deployment failed. The SLO definition remains saved."
		state.NextAction = "Inspect the rule destination, credentials, file permissions and ownership. The reconciler retries with backoff."
		if errors.Is(e, model.ErrRevisionConflict) {
			state.State = "pending"
			state.Code = "automatic_rules_changed"
			state.Message = "Desired state changed during deployment; a fresh attempt is required."
		}
		delay := min(5*time.Minute, 5*time.Second*time.Duration(1<<min(state.Attempts-1, 6)))
		next = completed.Add(delay)
	} else if result.Activation != nil && (result.Activation.State == "failed" || result.Activation.State == "blocked") {
		state.IntegrationStage = *result.Activation
		state.Message = "Rules were saved, but activation failed. " + state.Message
		delay := min(5*time.Minute, 5*time.Second*time.Duration(1<<min(state.Attempts-1, 6)))
		next = completed.Add(delay)
	} else {
		state.Attempts = 0
	}
	state.NextAttemptAt = &next
	persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if err := s.saveRuleAutomation(persist, id, state.Revision, state); err != nil {
		return state, err
	}
	return state, e
}

// PauseAutomaticRules is called under DefinitionMu for an explicit owner
// withdrawal. The pause survives restarts and definition edits until apply.
func (s *Service) PauseAutomaticRules(ctx context.Context, record model.Record, manager *rules.Manager, expected string) error {
	target, ok := manager.TargetFor(record.SLO)
	if !ok || !target.AutoDeploy {
		return nil
	}
	plan, e := manager.PlanContext(ctx, record.SLO)
	if e != nil {
		return e
	}
	if plan.Digest != expected {
		return model.ErrRevisionConflict
	}
	now := s.Now().UTC()
	state := &v1.RuleAutomationStatus{Enabled: true, Paused: true, Revision: model.ConfigRevision(record.SLO), TargetDigest: model.DefinitionDigest(target), Digest: plan.Digest, IntegrationStage: v1.IntegrationStage{State: "paused", Code: "automatic_rules_paused", Message: "Automatic deployment is paused after an explicit rule withdrawal.", NextAction: "Apply the reviewed rules to resume automatic deployment for this SLO.", ObservedAt: &now}}
	return s.saveRuleAutomation(ctx, record.SLO.ID, state.Revision, state)
}
func (s *Service) ResumeAutomaticRules(ctx context.Context, record model.Record, manager *rules.Manager) error {
	if record.RuleAutomation == nil {
		return nil
	}
	return s.saveRuleAutomation(ctx, record.SLO.ID, model.ConfigRevision(record.SLO), nil)
}

// RunRuleReconciler scans persisted desired state on startup and every five
// seconds. A successful revision is rechecked each minute to repair owned drift.
func (s *Service) RunRuleReconciler(ctx context.Context, manager *rules.Manager, log *slog.Logger) {
	if manager == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	after := ""
	for {
		configs, e := s.Store.List(ctx)
		if e == nil {
			sort.Slice(configs, func(i, j int) bool { return configs[i].ID < configs[j].ID })
			processed := 0
			for _, config := range configs {
				if config.ID <= after {
					continue
				}
				after = config.ID
				target, ok := manager.TargetFor(config)
				if !ok || !target.AutoDeploy {
					continue
				}
				prior, e := s.readRuleAutomation(ctx, config.ID)
				if e != nil {
					continue
				}
				if prior != nil && (prior.Paused || (prior.Revision == model.ConfigRevision(config) && prior.TargetDigest == model.DefinitionDigest(target) && prior.NextAttemptAt != nil && prior.NextAttemptAt.After(s.Now()))) {
					continue
				}
				job, stop := context.WithTimeout(ctx, 30*time.Second)
				state, e := s.ReconcileRules(job, config.ID, manager)
				stop()
				processed++
				if e != nil || state != nil && (state.State == "failed" || state.State == "blocked") {
					log.Warn("Automatic rule deployment needs attention", "slo", config.ID)
				}
				if processed >= 100 || ctx.Err() != nil {
					break
				}
			}
			if processed < 100 {
				after = ""
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) SetRuleAutomation(ctx context.Context, id string, revision int64, paused bool, manager *rules.Manager) (*v1.RuleAutomationStatus, error) {
	if revision < 1 || manager == nil {
		return nil, model.Invalid("provide the current SLO revision and configured rule destination")
	}
	s.DefinitionMu.Lock()
	defer s.DefinitionMu.Unlock()
	record, e := s.Store.Read(ctx, id)
	if e != nil {
		return nil, e
	}
	if model.ConfigRevision(record.SLO) != revision {
		return nil, model.ErrRevisionConflict
	}
	target, ok := manager.TargetFor(record.SLO)
	if !ok || !target.AutoDeploy {
		return nil, model.Invalid("automatic deployment is not enabled on this data source")
	}
	var state *v1.RuleAutomationStatus
	if paused {
		now := s.Now().UTC()
		state = &v1.RuleAutomationStatus{Enabled: true, Paused: true, Revision: revision, TargetDigest: model.DefinitionDigest(target), IntegrationStage: v1.IntegrationStage{State: "paused", Code: "automatic_rules_paused", Message: "An owner paused automatic rule deployment for this SLO. Existing rules remain in place.", NextAction: "Resume to reconcile the latest saved revision.", ObservedAt: &now}}
	}
	if e = s.saveRuleAutomation(ctx, id, revision, state); e != nil {
		return nil, e
	}
	record.RuleAutomation = state
	return s.RuleAutomationStatus(record, manager), nil
}
