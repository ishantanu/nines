package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

const maxAuditEvents = 256

var auditDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func auditChainValid(events []v1.IntentAuditEvent) bool {
	previous := ""
	for i, event := range events {
		expected := event.Hash
		event.Hash = ""
		if event.Sequence != int64(i+1) || event.PreviousHash != previous || hash(event) != expected {
			return false
		}
		previous = expected
	}
	return true
}

func frozenReviewsValid(session v1.IntentConversation) bool {
	changes := clone(session.ChangeSetHistory)
	if session.ChangeSet != nil {
		changes = append(changes, *session.ChangeSet)
	}
	for _, change := range changes {
		if changeDigest(session.Tenant, session.ID, change) != change.Digest {
			return false
		}
		rollbacks := clone(change.RollbackHistory)
		if change.Rollback != nil {
			rollbacks = append(rollbacks, *change.Rollback)
		}
		for _, rollback := range rollbacks {
			if rollback.OriginalDigest != change.Digest || rollbackDigest(session.Tenant, session.ID, rollback) != rollback.Digest {
				return false
			}
		}
	}
	return true
}

func auditRoom(r *record) error {
	if !auditChainValid(r.Session.Audit) || !frozenReviewsValid(r.Session) {
		return model.Invalid("audit integrity check failed; inspect the saved decision records before changing definitions")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(r.Session.Audit) >= maxAuditEvents || len(b) > maxDocument-16384 {
		return ErrLimit
	}
	return nil
}
func audit(r *record, a v1.InvestigationActor, action, outcome, digest, reason string, now time.Time) {
	e := v1.IntentAuditEvent{Sequence: int64(len(r.Session.Audit) + 1), At: now, Actor: a, Action: action, Outcome: outcome, ChangeSetDigest: digest, Reason: reason, Version: r.Session.Version}
	if n := len(r.Session.Audit); n > 0 {
		e.PreviousHash = r.Session.Audit[n-1].Hash
	}
	e.Hash = hash(e)
	r.Session.Audit = append(r.Session.Audit, e)
}
func (s *Service) reviewReason(in v1.ReviewIntentChangeSet, required bool) error {
	if (required || in.Reason != "") && (!text(in.Reason, 1000) || s.sensitiveInput(in.Reason)) {
		return model.Invalid("provide a review reason of at most 1000 bytes without credentials")
	}
	return nil
}
func reasonOrDefault(reason string) string {
	if reason == "" {
		return "No reviewer note supplied by this client."
	}
	return reason
}
func (s *Service) auditFailure(ctx context.Context, id, action, digest string, cause error) {
	if cause == nil || !identifier.MatchString(id) {
		return
	}
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.TenantID != s.Platform.TenantID || p.Subject == "" {
		return
	}
	outcome := "failed"
	if errors.Is(cause, ErrForbidden) {
		outcome = "denied"
	} else if errors.Is(cause, ErrConflict) || errors.Is(cause, model.ErrRevisionConflict) {
		outcome = "conflict"
	}
	if !auditDigestPattern.MatchString(digest) || s.sensitiveInput(digest) {
		digest = ""
	}
	// Do not persist provider errors, request bodies or potentially credential-bearing reasons.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, _ = s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != p.TenantID {
			return ErrNotFound
		}
		if err := auditRoom(r); err != nil {
			return err
		}
		audit(r, v1.InvestigationActor{Subject: p.Subject, Name: p.Name, Kind: p.Kind}, action, outcome, digest, "Request did not change definitions; inspect current revisions, permissions and review state.", now)
		return nil
	})
}
func (s *Service) Audit(ctx context.Context, id string) (v1.IntentAuditLog, error) {
	principal, ok := tenancy.FromContext(ctx)
	if !ok || principal.TenantID != s.Platform.TenantID || (principal.Role != "owner" && principal.Role != "editor" && principal.Role != "agent" && principal.Role != "viewer") {
		return v1.IntentAuditLog{}, ErrForbidden
	}
	r, e := s.Read(ctx, id)
	out := v1.IntentAuditLog{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Conversation: id, Integrity: "verified", Events: []v1.IntentAuditEvent{}}
	if e != nil {
		return out, e
	}
	if !auditChainValid(r.Audit) || !frozenReviewsValid(r) {
		out.Integrity = "invalid"
	}
	out.ChangeSets = clone(r.ChangeSetHistory)
	if r.ChangeSet != nil {
		out.ChangeSets = append(out.ChangeSets, clone(*r.ChangeSet))
	}
	if out.ChangeSets == nil {
		out.ChangeSets = []v1.IntentChangeSet{}
	}
	out.Events = r.Audit
	if out.Events == nil {
		out.Events = []v1.IntentAuditEvent{}
	}
	return out, nil
}
func previousDefinition(change v1.IntentDefinitionChange) (model.Definition, error) {
	raw := change.Previous
	var wrapper service.ResourceRevision
	if json.Unmarshal(raw, &wrapper) == nil && wrapper.Manifest != nil {
		raw, _ = json.Marshal(wrapper.Manifest)
	}
	return model.ParseDefinition(raw)
}
func recommendation(r *record, c v1.IntentChangeSet) *v1.IntentRecommendation {
	p := r.Session.Plan
	goal := r.Session.Goal
	result := &v1.IntentRecommendation{
		MeasurementDigest: p.Compiled.MeasurementDigest, RequestedBy: r.Session.CreatedBy, CompilerVersion: p.Compiled.CompilerVersion, RollbackStatus: "no_definition_change",
		Summary:     fmt.Sprintf("%s: %g%% good events over %s", goal.Metadata.Name, goal.Spec.Target*100, time.Duration(goal.Spec.WindowSeconds)*time.Second),
		Basis:       []string{"The target, rolling window and alert policies come from the reviewed user goal; the model does not choose deployment permissions.", "The eligible population and success criteria come from the measurement explicitly confirmed by the reviewer."},
		Limitations: []string{"The recommendation summarizes the reviewed goal and recorded evidence; it does not guarantee production reliability.", "Metric presence cannot establish business correctness or end-to-end journey success; review the observation point and excluded traffic.", "Definition application is atomic and performs no provider writes. After commit, rule deployment follows the owner-configured data source policy: automatic reconciliation or explicit reviewed apply.", "Restoring configuration cannot undo consumed error budget or erase historical incidents."},
		Mode:        "guided", Binding: p.Compiled.Binding, SourceDigest: p.SourceDigest, EvidenceDigest: p.Compiled.Evidence.Fingerprint, ObservedFrom: p.Compiled.Evidence.From, ObservedTo: p.Compiled.Evidence.To,
		Fields: []v1.IntentFieldChange{},
	}
	if n := len(r.Session.Runs); n > 0 {
		run := r.Session.Runs[n-1]
		result.RequestedBy = run.Actor
		result.RunID = run.ID
		result.Mode = run.Mode
		result.ModelRoute = run.ModelRoute
		result.DraftRoute = run.DraftRoute
		result.Skill = &run.Skill
		for i := len(r.Session.Messages) - 1; i >= 0; i-- {
			m := r.Session.Messages[i]
			if m.Role == "assistant" && m.RunID == run.ID {
				result.Explanation = m.Text
				break
			}
		}
	}
	result.Limitations = append(result.Limitations, p.Warnings...)
	result.Limitations = append(result.Limitations, p.Compiled.Warnings...)
	result.Limitations = append(result.Limitations, p.Compiled.Evidence.Warnings...)
	if b := r.Session.Backtest; b != nil && b.IntentDigest == c.IntentDigest && b.ManifestDigest == p.Compiled.Digest && b.SourceDigest == c.SourceDigest {
		result.Backtest = &v1.IntentBacktestSummary{From: b.From, To: b.To, ResolutionSeconds: b.ResolutionSeconds, Complete: b.CompleteCutoffs, Unknown: b.UnknownCutoffs, NoTraffic: b.NoTrafficCutoffs, TargetViolations: b.TargetViolationCutoffs, BurnBreaches: b.BurnThresholdCutoffs, Digest: hash(*b)}
	} else {
		result.Limitations = append(result.Limitations, "No matching historical backtest is attached to this recommendation.")
	}
	for _, change := range c.Changes {
		if change.Action == "create" {
			result.RollbackStatus = "creation_has_no_baseline"
		} else if change.Action == "update" && result.RollbackStatus != "creation_has_no_baseline" {
			result.RollbackStatus = "restorable_updates"
		}
		var after map[string]any
		_ = json.Unmarshal(change.Manifest, &after)
		var before map[string]any
		if old, e := previousDefinition(change); e == nil {
			b, _ := json.Marshal(old.Manifest())
			_ = json.Unmarshal(b, &before)
		}
		spec, _ := after["spec"].(map[string]any)
		prior, _ := before["spec"].(map[string]any)
		for _, field := range []struct{ name, impact string }{
			{"service", "Service changes start a new measurement revision with fresh coverage; previous evidence remains archived."},
			{"source", "Source changes require fresh measurement evidence and a separate rule-destination review; operator route changes require withdrawal first."},
			{"measurement", "Changing the SLO-owned measurement starts fresh coverage; the previous contract and evidence remain archived."},
			{"measurementBinding", "Changing the saved measurement revision changes the evidence contract. Old samples and validation are not reused."},
			{"stepSeconds", "Changing the interval changes sampling alignment and required coverage. Existing evidence remains with the previous revision."},
			{"providerWindow", "Counter selector and scrape-cadence changes alter the measured population or success criteria; new coverage is required."},
			{"goodQuery", "Changing the good-event query changes success semantics. Previous observations are archived instead of reinterpreted."},
			{"totalQuery", "Changing the total-event query changes the eligible population. Previous observations are archived instead of reinterpreted."},
			{"target", "A higher target leaves a smaller error budget for the same traffic."},
			{"windowSeconds", "Changing the rolling window changes the evaluation horizon and required coverage."},
			{"policy", "After an approved save, alert policy changes reach the external evaluator through the source's automatic or manual rule delivery policy."},
			{"policies", "Each severity and burn window has independent alert behavior; no notification routing is changed."},
			{"slos", "Journey links describe dependencies; they do not prove end-to-end journey success."},
			{"steps", "Review the exact journey step and required SLO links."},
		} {
			a, old := spec[field.name], prior[field.name]
			if hash(a) == hash(old) {
				continue
			}
			impact := field.impact
			if field.name == "target" {
				next, hasNext := a.(float64)
				previous, hasPrevious := old.(float64)
				if hasNext && hasPrevious {
					impact = fmt.Sprintf("Allowed bad-event fraction changes from %.6g%% to %.6g%%. A lower allowance is stricter; a higher allowance relaxes the objective.", (1-previous)*100, (1-next)*100)
				}
			}
			next, _ := json.Marshal(a)
			previous, _ := json.Marshal(old)
			result.Fields = append(result.Fields, v1.IntentFieldChange{Resource: change.Name, Field: "spec." + field.name, Before: previous, After: next, Impact: impact})
		}
	}
	return result
}
func (s *Service) RejectChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (out v1.IntentConversation, e error) {
	defer func() { s.auditFailure(ctx, id, "change_set.rejected", in.Digest, e) }()
	a, e := human(ctx, s.Platform.TenantID)
	if e != nil {
		return out, e
	}
	if e = s.reviewReason(in, true); e != nil {
		return out, e
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 {
		return out, model.Invalid("confirm the exact digest, version and rejection reason")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		c := r.Session.ChangeSet
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion || c == nil || c.Digest != in.Digest || changeDigest(s.Platform.TenantID, id, *c) != in.Digest || (c.State != "proposed" && c.State != "approved") {
			return ErrConflict
		}
		if e := auditRoom(r); e != nil {
			return e
		}
		c.State = "rejected"
		c.RejectedBy = &a
		c.RejectedAt = &now
		c.RejectionReason = in.Reason
		touch(r, now)
		audit(r, a, "change_set.rejected", "succeeded", c.Digest, in.Reason, now)
		return nil
	})
	return r.Session, e
}

// RecordDenied records an authenticated, tenant-scoped governance denial without
// reading request bodies or accepting a caller-supplied audit event.
func (s *Service) RecordDenied(ctx context.Context, path string) {
	const prefix = "/api/intent-conversations/"
	if !strings.HasPrefix(path, prefix) {
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(path, prefix), "/", 2)
	if len(parts) != 2 || !identifier.MatchString(parts[0]) {
		return
	}
	actions := map[string]string{"change-set/approve": "change_set.approved", "change-set/apply": "change_set.applied", "change-set/reject": "change_set.rejected", "change-set/rollback": "rollback.proposed", "change-set/rollback/approve": "rollback.approved", "change-set/rollback/apply": "rollback.applied", "change-set/rollback/reject": "rollback.rejected"}
	if action := actions[parts[1]]; action != "" {
		s.auditFailure(ctx, parts[0], action, "", ErrForbidden)
	}
}
