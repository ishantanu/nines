package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

func changeDigest(tenant, id string, c v1.IntentChangeSet) string {
	return hash(struct {
		Tenant, Conversation, ID, Intent, Source string
		Binding                                  v1.IntentSelection
		Changes                                  []v1.IntentDefinitionChange
		Recommendation                           *v1.IntentRecommendation `json:"Recommendation,omitempty"`
	}{tenant, id, c.ID, c.IntentDigest, c.SourceDigest, c.Binding, c.Changes, c.Recommendation})
}
func invalidateChangeSet(r *record, a v1.InvestigationActor, reason string, now time.Time) error {
	if r.Session.ChangeSet == nil {
		return nil
	}
	if len(r.Session.ChangeSetHistory) >= 8 {
		return ErrLimit
	}
	if e := auditRoom(r); e != nil {
		return e
	}
	c := r.Session.ChangeSet
	action := "change_set.replaced"
	switch c.State {
	case "proposed", "approved":
		c.State = "superseded"
		action = "change_set.superseded"
	case "applied":
		action = "change_set.retained"
	}
	audit(r, a, action, "succeeded", c.Digest, reason, now)
	r.Session.ChangeSetHistory = append(r.Session.ChangeSetHistory, clone(*c))
	r.Session.ChangeSet = nil
	return nil
}
func (s *Service) PrepareChangeSet(ctx context.Context, id string, version int64) (out v1.IntentConversation, e error) {
	defer func() { s.auditFailure(ctx, id, "change_set.proposed", "", e) }()
	a, e := actor(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if !identifier.MatchString(id) || version < 1 {
		return v1.IntentConversation{}, model.Invalid("provide conversation ID and expectedVersion")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if active(*r) || r.Session.Version != version {
			return ErrConflict
		}
		if r.Session.ChangeSet != nil && r.Session.ChangeSet.State == "rejected" {
			if e := invalidateChangeSet(r, a, "Rejected proposal replaced by a new review.", now); e != nil {
				return e
			}
		}
		if r.Session.ChangeSet != nil {
			return nil
		}
		if err := auditRoom(r); err != nil {
			return err
		}
		if len(r.Session.ChangeSetHistory) >= 8 {
			return ErrLimit
		}
		p := r.Session.Plan
		if p != nil && p.SourceDigest != s.Platform.SourceFingerprint(r.Session.Scope.Source) {
			return ErrConflict
		}
		if r.Session.State != "ready" || r.Session.Goal == nil || r.Session.Selection == nil || p == nil || p.Status != "ready" || p.Compiled == nil || p.SourceDigest != s.Platform.SourceFingerprint(r.Session.Scope.Source) || p.IntentDigest != hash(*r.Session.Goal) || r.Session.Selection.IntentDigest != p.IntentDigest {
			return model.Invalid("prepare and review an intent proposal with an explicitly confirmed measurement first")
		}
		c := v1.IntentChangeSet{ID: model.NewID(), SourceDigest: p.SourceDigest, State: "proposed", CreatedAt: now, CreatedBy: a, IntentDigest: p.IntentDigest, Binding: r.Session.Selection.Binding, Changes: []v1.IntentDefinitionChange{}}
		def := model.SLODefinition(p.Compiled.Manifest)
		before, err := s.Platform.PlanDefinition(ctx, model.Definition{SLO: &def})
		if err != nil {
			return err
		}
		if before.ExpectedRevision != p.ExpectedRevision || before.Digest != p.Compiled.Digest {
			return ErrConflict
		}
		manifest, _ := json.Marshal(p.Compiled.Manifest)
		change := v1.IntentDefinitionChange{Kind: v1.SLOKind, Name: def.Metadata.Name, Action: before.Action, ExpectedRevision: p.ExpectedRevision, Digest: p.Compiled.Digest, Manifest: manifest}
		if before.Previous != nil {
			previous := before.Previous
			if revision, ok := previous.(service.ResourceRevision); ok {
				previous = revision.Manifest
			}
			change.Previous, _ = json.Marshal(previous)
		}
		c.Changes = append(c.Changes, change)
		if p.Journey != nil {
			old, err := s.Platform.Journeys.JourneyRevision(ctx, p.Journey.Manifest.Metadata.Name, 0)
			if err != nil {
				return err
			}
			if old.Revision != p.Journey.ExpectedRevision {
				return ErrConflict
			}
			manifest, _ := json.Marshal(p.Journey.Manifest)
			previous, _ := json.Marshal(old.Manifest)
			action := "update"
			if old.Digest == p.Journey.Digest {
				action = "unchanged"
			}
			c.Changes = append(c.Changes, v1.IntentDefinitionChange{Kind: v1.JourneyKind, Name: p.Journey.Manifest.Metadata.Name, Action: action, ExpectedRevision: p.Journey.ExpectedRevision, Digest: p.Journey.Digest, Manifest: manifest, Previous: previous})
		}
		c.Recommendation = recommendation(r, c)
		if s.sensitiveInput(c.Recommendation) {
			return model.Invalid("recommendation evidence contains configured credentials")
		}
		c.Digest = changeDigest(s.Platform.TenantID, id, c)
		if _, err := s.Platform.IntentDefinitionBatch(ctx, id+"/"+c.ID, c.Digest, false, *r.Session.Goal, c.Binding, c.Changes, c.SourceDigest); err != nil {
			return err
		}
		r.Session.ChangeSet = &c
		encoded, _ := json.Marshal(r)
		if len(encoded) > maxDocument-16384 {
			return ErrLimit
		}
		touch(r, now)
		audit(r, a, "change_set.proposed", "succeeded", c.Digest, "Frozen recommendation, measurement provenance and exact resource changes for review.", now)
		return nil
	})
	return r.Session, e
}
func human(ctx context.Context, tenant string) (v1.InvestigationActor, error) {
	a, e := actor(ctx, tenant)
	if e != nil {
		return a, e
	}
	p, _ := tenancy.FromContext(ctx)
	if p.Role != "owner" && p.Role != "editor" {
		return a, ErrForbidden
	}
	return a, nil
}
func (s *Service) ApproveChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (out v1.IntentConversation, e error) {
	defer func() { s.auditFailure(ctx, id, "change_set.approved", in.Digest, e) }()
	if e = s.reviewReason(in, true); e != nil {
		return out, e
	}
	a, e := human(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 {
		return v1.IntentConversation{}, model.Invalid("explicitly confirm the reviewed change-set digest and conversation revision")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		c := r.Session.ChangeSet
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion || c == nil || c.Digest != in.Digest || changeDigest(s.Platform.TenantID, id, *c) != in.Digest || r.Session.Goal == nil || c.IntentDigest != hash(*r.Session.Goal) || c.SourceDigest != s.Platform.SourceFingerprint(r.Session.Scope.Source) {
			return ErrConflict
		}
		if c.State == "approved" {
			return nil
		}
		if c.State != "proposed" {
			return ErrConflict
		}
		if _, err := s.Platform.IntentDefinitionBatch(ctx, id+"/"+c.ID, c.Digest, false, *r.Session.Goal, c.Binding, c.Changes, c.SourceDigest); err != nil {
			return err
		}
		if err := auditRoom(r); err != nil {
			return err
		}
		c.ApprovedBy = &a
		c.ApprovalReason = in.Reason
		c.ApprovedAt = &now
		c.State = "approved"
		touch(r, now)
		audit(r, a, "change_set.approved", "succeeded", c.Digest, c.ApprovalReason, now)
		return nil
	})
	return r.Session, e
}
func (s *Service) ApplyChangeSet(ctx context.Context, id string, in v1.ReviewIntentChangeSet) (out v1.IntentConversation, e error) {
	defer func() { s.auditFailure(ctx, id, "change_set.applied", in.Digest, e) }()
	a, e := human(ctx, s.Platform.TenantID)
	if e != nil {
		return v1.IntentConversation{}, e
	}
	if !identifier.MatchString(id) || !in.Confirm || in.ExpectedVersion < 1 || len(in.Digest) != 64 {
		return v1.IntentConversation{}, model.Invalid("explicitly apply the approved digest and conversation revision")
	}
	r, e := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		c := r.Session.ChangeSet
		if r.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if c == nil || c.Digest != in.Digest || changeDigest(s.Platform.TenantID, id, *c) != in.Digest {
			return ErrConflict
		}
		// Exact retries return the immutable receipt without reapplying or overwriting
		// later external edits. Read-back verification reports those edits separately.
		if c.State == "applied" {
			return nil
		}
		if active(*r) || r.Session.Version != in.ExpectedVersion || c.State != "approved" || c.ApprovedBy == nil || c.ApprovedAt == nil || r.Session.Goal == nil || hash(*r.Session.Goal) != c.IntentDigest {
			return ErrConflict
		}
		if a.Subject == c.ApprovedBy.Subject {
			return ErrForbidden
		}
		if err := auditRoom(r); err != nil {
			return err
		}
		evidence := v1.IntentOperationEvidence{Operation: "apply", ChangeSetDigest: c.Digest, ApprovedBy: *c.ApprovedBy, ApprovedAt: *c.ApprovedAt, Reason: c.ApprovalReason}
		if c.Recommendation != nil {
			evidence.RecommendationDigest = hash(c.Recommendation)
		}
		applied, err := s.Platform.IntentDefinitionBatch(ctx, id+"/"+c.ID, c.Digest, true, *r.Session.Goal, c.Binding, c.Changes, c.SourceDigest, evidence)
		if err != nil {
			return err
		}
		c.Applied = applied.Resources
		c.AppliedAt = &applied.At
		c.AppliedBy = &applied.Actor
		c.State = "applied"
		touch(r, now)
		audit(r, applied.Actor, "change_set.applied", "succeeded", c.Digest, "Approved definitions committed atomically. External rules were not deployed.", now)
		return nil
	})
	return r.Session, e
}
func (s *Service) VerifyChangeSet(ctx context.Context, id string) (v1.IntentChangeSetVerification, error) {
	r, e := s.Read(ctx, id)
	out := v1.IntentChangeSetVerification{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Conversation: id, Resources: []v1.IntentResourceVerification{}}
	if e != nil {
		return out, e
	}
	c := r.ChangeSet
	if c == nil || c.State != "applied" {
		return out, model.Invalid("apply a reviewed change set before verification")
	}
	out.Digest = c.Digest
	out.SourceState = "matches"
	if c.SourceDigest != s.Platform.SourceFingerprint(r.Scope.Source) {
		out.SourceState = "changed"
	}
	out.Operation = "apply"
	appliedResources := clone(c.Applied)
	if rollback := c.Rollback; rollback != nil && rollback.State == "applied" {
		out.Operation = "rollback"
		out.RollbackDigest = rollback.Digest
		for _, restored := range rollback.Applied {
			for i, original := range appliedResources {
				if original.Kind == restored.Kind && original.Name == restored.Name {
					appliedResources[i] = restored
				}
			}
		}
	}
	for _, expected := range appliedResources {
		v := v1.IntentResourceVerification{IntentAppliedResource: expected, State: "matches", Health: "unknown"}
		if expected.Kind == v1.SLOKind {
			actual, err := s.Platform.ReadSLO(ctx, expected.Name)
			if errors.Is(err, model.ErrNotFound) {
				v.State = "missing"
			} else if err != nil {
				return out, err
			} else {
				v.CurrentRevision = model.ConfigRevision(actual.SLO)
				v.CurrentDigest = hash(model.DefinitionFromSLO(actual.SLO))
				if v.CurrentRevision != expected.Revision || v.CurrentDigest != expected.Digest {
					v.State = "drifted"
				}
				v.Health = service.SLOHealth(actual.Status)
			}
		} else {
			actual, err := s.Platform.Journeys.JourneyRevision(ctx, expected.Name, 0)
			if errors.Is(err, model.ErrJourneyNotFound) {
				v.State = "missing"
			} else if err != nil {
				return out, err
			} else {
				v.CurrentRevision = actual.Revision
				v.CurrentDigest = actual.Digest
				view, err := s.Platform.JourneyView(ctx, actual)
				if err != nil {
					return out, err
				}
				v.Health = view.Health
				if actual.Revision != expected.Revision || actual.Digest != expected.Digest {
					v.State = "drifted"
				}
			}
		}
		if out.SourceState != "matches" {
			v.Health = "unknown"
		}
		out.Resources = append(out.Resources, v)
	}
	return out, nil
}
