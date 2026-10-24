package intentflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func rollbackDigest(tenant, conversation string, r v1.IntentRollback) string {
	return hash(struct {
		Tenant, Conversation, ID, Original, Reason string
		Changes                                    []v1.IntentDefinitionChange
		Expected                                   []v1.IntentAppliedResource
		Blockers                                   []string
	}{tenant, conversation, r.ID, r.OriginalDigest, r.Reason, r.Changes, r.Expected, r.Blockers})
}
func findChangeSet(r *record, digest string, rollback bool) *v1.IntentChangeSet {
	match := func(c *v1.IntentChangeSet) bool {
		return (!rollback && c.Digest == digest) || (rollback && c.Rollback != nil && c.Rollback.Digest == digest)
	}
	if c := r.Session.ChangeSet; c != nil && match(c) {
		return c
	}
	for i := range r.Session.ChangeSetHistory {
		c := &r.Session.ChangeSetHistory[i]
		if match(c) {
			return c
		}
	}
	return nil
}
func (s *Service) PrepareRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (out v1.IntentConversation, e error) {
	defer func() { s.auditFailure(ctx, id, "rollback.proposed", in.Digest, e) }()
	a, e := human(ctx, s.Platform.TenantID)
	if e != nil {
		return out, e
	}
	if e = s.reviewReason(in, true); e != nil {
		return out, e
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 {
		return out, model.Invalid("confirm the original change-set digest, version and rollback reason")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		c := findChangeSet(r, in.Digest, false)
		if active(*r) || r.Session.Version != in.ExpectedVersion || c == nil || c.State != "applied" || c.Digest != changeDigest(s.Platform.TenantID, id, *c) {
			return ErrConflict
		}
		if c.Rollback != nil && c.Rollback.State != "rejected" {
			return nil
		}
		if e := auditRoom(r); e != nil {
			return e
		}
		if c.Rollback != nil {
			if len(c.RollbackHistory) >= 8 {
				return ErrLimit
			}
			c.RollbackHistory = append(c.RollbackHistory, clone(*c.Rollback))
		}
		rollback := &v1.IntentRollback{ID: model.NewID(), OriginalDigest: c.Digest, State: "proposed", Reason: in.Reason, CreatedAt: now, CreatedBy: a, Changes: []v1.IntentDefinitionChange{}, Expected: clone(c.Applied)}
		for _, change := range c.Changes {
			if change.Action == "unchanged" {
				continue
			}
			if len(change.Previous) == 0 {
				rollback.Blockers = append(rollback.Blockers, fmt.Sprintf("%s has no previous definition. Creation cannot be undone by configuration rollback; resource retirement is not implemented.", change.Name))
				continue
			}
			before, e := previousDefinition(change)
			if e != nil {
				return e
			}
			var expected int64
			for _, applied := range c.Applied {
				if applied.Kind == change.Kind && applied.Name == change.Name {
					expected = applied.Revision
				}
			}
			if expected == 0 {
				return ErrConflict
			}
			manifest, _ := json.Marshal(before.Manifest())
			rollback.Changes = append(rollback.Changes, v1.IntentDefinitionChange{Kind: before.Kind(), Name: before.Metadata().Name, Action: "restore", ExpectedRevision: expected, Digest: hash(before.Manifest()), Manifest: manifest, Previous: clone(change.Manifest)})
		}
		if len(rollback.Changes) == 0 && len(rollback.Blockers) == 0 {
			rollback.Blockers = append(rollback.Blockers, "The applied change set did not change a definition.")
		}
		if len(rollback.Blockers) > 0 {
			rollback.State = "blocked"
		}
		rollback.Digest = rollbackDigest(s.Platform.TenantID, id, *rollback)
		if rollback.State != "blocked" {
			if _, e := s.Platform.RestoreIntentDefinitions(ctx, id+"/rollback/"+rollback.ID, rollback.Digest, false, rollback.Changes, rollback.Expected, r.Session.Scope.Source, c.SourceDigest, v1.IntentOperationEvidence{}); e != nil {
				return e
			}
		}
		c.Rollback = rollback
		touch(r, now)
		outcome := "succeeded"
		if rollback.State == "blocked" {
			outcome = "blocked"
		}
		audit(r, a, "rollback.proposed", outcome, rollback.Digest, in.Reason, now)
		return nil
	})
	return r.Session, e
}
func (s *Service) ReviewRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet, apply bool) (out v1.IntentConversation, e error) {
	action := "rollback.approved"
	if apply {
		action = "rollback.applied"
	}
	defer func() { s.auditFailure(ctx, id, action, in.Digest, e) }()
	a, e := human(ctx, s.Platform.TenantID)
	if e != nil {
		return out, e
	}
	if e = s.reviewReason(in, !apply); e != nil {
		return out, e
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 {
		return out, model.Invalid("confirm the reviewed rollback digest and version")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		c := findChangeSet(r, in.Digest, true)
		if c == nil || c.State != "applied" || c.Digest != changeDigest(s.Platform.TenantID, id, *c) {
			return ErrConflict
		}
		rollback := c.Rollback
		if rollback.Digest != rollbackDigest(s.Platform.TenantID, id, *rollback) {
			return ErrConflict
		}
		if apply && rollback.State == "applied" {
			return nil
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion || len(rollback.Blockers) > 0 {
			return ErrConflict
		}
		if (!apply && rollback.State != "proposed") || (apply && (rollback.State != "approved" || rollback.ApprovedBy == nil || rollback.ApprovedAt == nil)) {
			return ErrConflict
		}
		if apply && a.Subject == rollback.ApprovedBy.Subject {
			return ErrForbidden
		}
		if e := auditRoom(r); e != nil {
			return e
		}
		evidence := v1.IntentOperationEvidence{Operation: "rollback", ChangeSetDigest: rollback.Digest}
		if apply {
			evidence.ApprovedBy = *rollback.ApprovedBy
			evidence.ApprovedAt = *rollback.ApprovedAt
			evidence.Reason = rollback.ApprovalReason
		}
		receipt, e := s.Platform.RestoreIntentDefinitions(ctx, id+"/rollback/"+rollback.ID, rollback.Digest, apply, rollback.Changes, rollback.Expected, r.Session.Scope.Source, c.SourceDigest, evidence)
		if e != nil {
			return e
		}
		if apply {
			rollback.State = "applied"
			rollback.AppliedBy = &receipt.Actor
			rollback.AppliedAt = &receipt.At
			rollback.Applied = receipt.Resources
			a = receipt.Actor
		} else {
			rollback.State = "approved"
			rollback.ApprovedBy = &a
			rollback.ApprovedAt = &now
			rollback.ApprovalReason = in.Reason
		}
		touch(r, now)
		eventReason := in.Reason
		if apply {
			eventReason = rollback.ApprovalReason
		}
		audit(r, a, action, "succeeded", rollback.Digest, reasonOrDefault(eventReason), now)
		return nil
	})
	return r.Session, e
}

func (s *Service) RejectRollback(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (out v1.IntentConversation, e error) {
	defer func() { s.auditFailure(ctx, id, "rollback.rejected", in.Digest, e) }()
	a, e := human(ctx, s.Platform.TenantID)
	if e != nil {
		return out, e
	}
	if e = s.reviewReason(in, true); e != nil {
		return out, e
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 {
		return out, model.Invalid("confirm the rollback digest, version and rejection reason")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		c := findChangeSet(r, in.Digest, true)
		if c == nil || active(*r) || r.Session.Version != in.ExpectedVersion || c.Rollback.Digest != rollbackDigest(s.Platform.TenantID, id, *c.Rollback) || (c.Rollback.State != "proposed" && c.Rollback.State != "approved") {
			return ErrConflict
		}
		if e := auditRoom(r); e != nil {
			return e
		}
		c.Rollback.State = "rejected"
		touch(r, now)
		audit(r, a, "rollback.rejected", "succeeded", in.Digest, in.Reason, now)
		return nil
	})
	return r.Session, e
}
