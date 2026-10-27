package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func (s *Service) retirementPlan(ctx context.Context, id, action string) (v1.RetirementPlan, error) {
	out := v1.RetirementPlan{APIVersion: v1.APIVersion, Tenant: s.TenantID, SLO: id, Action: action, Blockers: []string{}}
	if !validResourceID(id) || (action != "archive" && action != "delete") {
		return out, model.Invalid("choose archive or delete for an existing SLO")
	}
	record, e := s.Store.Read(ctx, id)
	if e != nil {
		return out, e
	}
	out.ExpectedRevision = model.ConfigRevision(record.SLO)
	out.Retirement = record.SLO.Retirement
	out.References, e = store.RetirementReferences(ctx, s.Store, id)
	if e != nil {
		return out, e
	}
	if len(out.References) > 0 {
		out.Blockers = append(out.Blockers, "Remove current journey and dependency-map references through reviewed edits before retirement.")
	}
	if action == "archive" {
		if record.SLO.Retired() {
			out.Blockers = append(out.Blockers, "SLO is already archived.")
		}
		if s.RuleManager != nil && record.SLO.HasQueries() {
			if target, ok := s.RuleManager.TargetFor(record.SLO); ok {
				out.RouteDigest = rules.RouteDigest(target)
				plan, e := s.RuleManager.PlanContext(ctx, record.SLO)
				if e != nil {
					return out, e
				}
				out.ResourceUID = plan.PriorUID
				out.RuleDigest = plan.Digest
				out.Group = plan.Group.Name
			}
		}
		if record.SLO.RuleDestination != "" && record.SLO.RuleDestination != out.RouteDigest {
			out.Blockers = append(out.Blockers, "Restore the original recording-rule destination before retirement.")
		}
		if record.OperatorRoute != "" && record.OperatorRoute != out.RouteDigest {
			out.Blockers = append(out.Blockers, "Restore the original operator destination before retirement.")
		}
	} else if !record.SLO.Retired() || record.SLO.Retirement.Cleanup != "complete" {
		out.Blockers = append(out.Blockers, "Archive the SLO and verify owned-rule cleanup before permanent deletion.")
	}
	// Bind both the definition and its current references to the reviewed action.
	out.Digest = model.DefinitionDigest(struct {
		Plan       v1.RetirementPlan
		Definition model.SLODefinition
	}{out, model.DefinitionFromSLO(record.SLO)})
	return out, nil
}
func (s *Service) PlanRetirement(ctx context.Context, id, action string) (v1.RetirementPlan, error) {
	batch, ok := s.Store.(store.DefinitionBatchStore)
	if !ok {
		return v1.RetirementPlan{}, model.ErrUnavailable
	}
	var out v1.RetirementPlan
	// No network in the batch: resolve the rule plan first, then recheck its trusted
	// route after the database snapshot. Operator plans may read Kubernetes.
	plan, e := s.retirementPlan(ctx, id, action)
	if e != nil {
		return out, e
	}
	_, e = batch.DefinitionBatch(ctx, "", "", false, func(st store.Store) (json.RawMessage, error) {
		r, e := st.Read(ctx, id)
		if e != nil {
			return nil, e
		}
		refs, e := store.RetirementReferences(ctx, st, id)
		if e != nil {
			return nil, e
		}
		if model.ConfigRevision(r.SLO) != plan.ExpectedRevision || model.DefinitionDigest(refs) != model.DefinitionDigest(plan.References) {
			return nil, model.ErrRevisionConflict
		}
		out = plan
		return json.Marshal(out)
	})
	return out, e
}
func retirementActor(ctx context.Context) string {
	if p, ok := tenancy.FromContext(ctx); ok {
		if p.Subject != "" {
			return p.Kind + ":" + p.Subject
		}
		return p.Kind + ":" + p.Role
	}
	return "local:operator"
}
func (s *Service) ApplyRetirement(ctx context.Context, id string, in v1.RetirementInput) (v1.RetirementReceipt, error) {
	s.DefinitionMu.Lock()
	defer s.DefinitionMu.Unlock()
	var out v1.RetirementReceipt
	if p, ok := tenancy.FromContext(ctx); ok && (p.TenantID != s.TenantID || p.Role != "owner") {
		return out, model.Invalid("retirement requires a tenant owner")
	}
	if prior, err := s.RetirementReceipt(ctx, id); err == nil {
		for _, event := range prior.Retirement.Audit {
			if event.Action == in.Action && event.Digest == in.Digest && event.Revision == in.ExpectedRevision && (in.Action != "delete" || in.ConfirmName == id) {
				return prior, nil
			}
		}
	}
	plan, e := s.PlanRetirement(ctx, id, in.Action)
	if e != nil {
		return out, e
	}
	if in.ExpectedRevision != plan.ExpectedRevision || in.Digest == "" || in.Digest != plan.Digest {
		return out, model.ErrRevisionConflict
	}
	if len(plan.Blockers) > 0 {
		return out, model.Invalid("%s", plan.Blockers[0])
	}
	if in.Action == "delete" && in.ConfirmName != id {
		return out, model.Invalid("permanent deletion requires confirmName to match the SLO identifier")
	}
	batch, ok := s.Store.(store.DefinitionBatchStore)
	if !ok {
		return out, model.ErrUnavailable
	}
	now := s.Now().UTC()
	var cutoff time.Time
	if in.Action == "archive" {
		record, err := s.Store.Read(ctx, id)
		if err != nil {
			return out, err
		}
		cutoff = s.recordEvaluationAt(record, now)
	}
	event := v1.RetirementEvent{ID: model.NewID(), Action: in.Action, At: now, Actor: retirementActor(ctx), Revision: plan.ExpectedRevision, Digest: in.Digest}
	raw, e := batch.DefinitionBatch(ctx, "retire:"+id+":"+in.Digest, in.Digest, true, func(st store.Store) (json.RawMessage, error) {
		r, e := st.Read(ctx, id)
		if e != nil {
			return nil, e
		}
		if model.ConfigRevision(r.SLO) != in.ExpectedRevision {
			return nil, model.ErrRevisionConflict
		}
		refs, e := store.RetirementReferences(ctx, st, id)
		if e != nil {
			return nil, e
		}
		if len(refs) > 0 {
			return nil, model.ErrRevisionConflict
		}
		out = v1.RetirementReceipt{APIVersion: v1.APIVersion, Tenant: s.TenantID, SLO: id, Revision: in.ExpectedRevision}
		if in.Action == "archive" {
			state := v1.SLORetirement{ResourceUID: plan.ResourceUID, EvaluationAt: cutoff, ArchivedAt: now, OriginalRevision: in.ExpectedRevision, RouteDigest: plan.RouteDigest, RuleDigest: plan.RuleDigest, Group: plan.Group, Cleanup: "pending", Message: "Collection and alert evaluation stopped. Owned-rule cleanup is pending.", Audit: []v1.RetirementEvent{event}}
			if state.RouteDigest == "" {
				state.Cleanup = "complete"
				state.Message = "Archived. No managed rule destination was configured."
			}
			e = st.Update(ctx, id, func(r *model.Record) error {
				if r.SLO.Retired() || model.ConfigRevision(r.SLO) != in.ExpectedRevision {
					return model.ErrRevisionConflict
				}
				// End active notification episodes explicitly before freezing evaluation.
				if len(r.Alert.Policies) > 0 {
					for name, state := range r.Alert.Policies {
						if state.Phase == "firing" {
							r.Events = append(r.Events, model.AlertEvent{ID: model.NewID(), SLOID: id, Kind: "resolved", Reason: "slo_archived", ConfigRevision: in.ExpectedRevision, Time: now, Policy: name, Severity: state.Severity})
						}
					}
				} else if r.Alert.Phase == "firing" {
					r.Events = append(r.Events, model.AlertEvent{ID: model.NewID(), SLOID: id, Kind: "resolved", Reason: "slo_archived", ConfigRevision: in.ExpectedRevision, Time: now})
				}
				r.SLO.Revision = in.ExpectedRevision + 1
				r.SLO.Retirement = &state
				r.Alert = model.AlertState{Phase: "archived"}
				return nil
			})
			out.Revision = in.ExpectedRevision + 1
			out.Retirement = state
		} else {
			if !r.SLO.Retired() || r.SLO.Retirement.Cleanup != "complete" {
				return nil, model.ErrRevisionConflict
			}
			out.Retirement = *r.SLO.Retirement
			out.Retirement.Audit = append(out.Retirement.Audit, event)
			out.DeletedAt = &now
			retirement, ok := st.(store.RetirementStore)
			if !ok {
				return nil, model.ErrUnavailable
			}
			e = retirement.PurgeSLO(ctx, id, out)
		}
		if e != nil {
			return nil, e
		}
		return json.Marshal(out)
	})
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}
func (s *Service) RetirementReceipt(ctx context.Context, id string) (v1.RetirementReceipt, error) {
	if !validResourceID(id) {
		return v1.RetirementReceipt{}, model.Invalid("invalid SLO identifier")
	}
	r, e := s.Store.Read(ctx, id)
	if e == nil {
		if !r.SLO.Retired() {
			return v1.RetirementReceipt{}, model.Invalid("SLO is active")
		}
		return v1.RetirementReceipt{APIVersion: v1.APIVersion, Tenant: s.TenantID, SLO: id, Revision: model.ConfigRevision(r.SLO), Retirement: *r.SLO.Retirement}, nil
	}
	if !errors.Is(e, model.ErrNotFound) {
		return v1.RetirementReceipt{}, e
	}
	st, ok := s.Store.(store.RetirementStore)
	if !ok {
		return v1.RetirementReceipt{}, model.ErrUnavailable
	}
	return st.RetirementReceipt(ctx, id)
}
func (s *Service) ReconcileRetirement(ctx context.Context, id string) (v1.RetirementReceipt, error) {
	s.DefinitionMu.Lock()
	defer s.DefinitionMu.Unlock()
	receipt, e := s.RetirementReceipt(ctx, id)
	if e != nil || receipt.DeletedAt != nil || receipt.Retirement.Cleanup == "complete" {
		return receipt, e
	}
	state := receipt.Retirement
	now := s.Now().UTC()
	if now.Before(state.ArchivedAt.Add(30 * time.Second)) {
		return receipt, nil
	}
	if state.NextAttemptAt != nil && state.NextAttemptAt.After(now) {
		return receipt, nil
	}
	r, e := s.Store.Read(ctx, id)
	if e != nil {
		return receipt, e
	}
	done := false
	if s.RuleManager == nil {
		e = model.ErrUnavailable
	} else {
		done, e = s.RuleManager.Retire(ctx, r.SLO)
	}
	state.Attempts++
	state.LastAttemptAt = &now
	next := now.Add(min(5*time.Minute, 5*time.Second*time.Duration(1<<min(state.Attempts-1, 6))))
	state.NextAttemptAt = &next
	state.Cleanup = "pending"
	state.Message = "Owned configuration removal or evaluator absence is still unverified. Keep the original connection available; cleanup will retry."
	if e != nil {
		state.Cleanup = "failed"
		state.Message = "Owned-rule cleanup failed. Check original connection availability, credentials and ownership; cleanup will retry."
		var invalid *model.ValidationError
		if errors.As(e, &invalid) {
			state.Cleanup = "blocked"
			state.Message = "Cleanup is blocked by destination or ownership checks. Restore the original trusted route and verify the original resource UID; cleanup will retry without removing a replacement resource."
		}
	}
	if done {
		state.Cleanup = "complete"
		state.Message = "Owned rule configuration was removed and evaluator absence verified. Historical provider samples follow the provider retention policy."
		state.NextAttemptAt = nil
		state.Audit = append(state.Audit, v1.RetirementEvent{ID: model.NewID(), Action: "cleanup_verified", At: now, Actor: "system:retirement", Revision: receipt.Revision, Digest: state.RouteDigest})
	}
	save := s.Store.Update(ctx, id, func(r *model.Record) error {
		if !r.SLO.Retired() || model.ConfigRevision(r.SLO) != receipt.Revision {
			return model.ErrRevisionConflict
		}
		r.SLO.Retirement = &state
		return nil
	})
	if save != nil {
		return receipt, save
	}
	receipt.Retirement = state
	if e != nil {
		return receipt, fmt.Errorf("owned cleanup remains pending: %w", model.ErrUnavailable)
	}
	return receipt, nil
}
