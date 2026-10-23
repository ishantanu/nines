package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentmodel"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func reviewInput(v v1.IntentConversation, digest, reason string) v1.ReviewIntentChangeSet {
	return v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: digest, Confirm: true, Reason: reason}
}
func updatedChangeFixture(t *testing.T, pg bool) (*Service, context.Context, v1.IntentConversation) {
	t.Helper()
	s, ctx, v := readyChangeFixture(t, pg)
	raw, _ := json.Marshal(v.Plan.Compiled.Manifest)
	old, e := model.ParseDefinition(raw)
	if e != nil {
		t.Fatal(e)
	}
	old.SLO.Spec.Target = .99
	old.SLO.Spec.Intent.Target = .99
	if _, e = s.Platform.ApplyDefinition(ctx, old, 0); e != nil {
		t.Fatal(e)
	}
	_, e = s.Repository.Update(ctx, v.ID, func(r *record, now time.Time) error { r.Session.Plan.ExpectedRevision = 1; touch(r, now); return nil })
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Read(ctx, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	return s, ctx, v
}
func appliedUpdate(t *testing.T, pg bool) (*Service, context.Context, v1.IntentConversation) {
	t.Helper()
	s, ctx, v := updatedChangeFixture(t, pg)
	var e error
	v, e = s.PrepareChangeSet(ctx, v.ID, v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed the API population, stricter budget and historical limitations."))
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.ApplyChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
	if e != nil {
		t.Fatal(e)
	}
	return s, ctx, v
}
func TestRecommendationApprovalRejectionAndAudit(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := updatedChangeFixture(t, pg)
			v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil {
				t.Fatal(e)
			}
			report := v.ChangeSet.Recommendation
			if report == nil || report.RollbackStatus != "restorable_updates" || report.CompilerVersion == "" || report.RequestedBy.Subject == "" || report.Binding.Revision != 1 || report.EvidenceDigest == "" || report.SourceDigest == "" || len(report.Fields) == 0 || !strings.Contains(strings.Join(report.Limitations, " "), "No matching historical backtest") {
				t.Fatal(report)
			}
			targetDiff := false
			for _, field := range report.Fields {
				if field.Field == "spec.target" {
					targetDiff = string(field.Before) == "0.99" && string(field.After) == "0.999"
				}
			}
			if !targetDiff {
				t.Fatal("target trade-off not explained", report.Fields)
			}
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Subject: "agent", Role: "agent", Kind: "api_token"})
			if _, e = s.ApproveChangeSet(agent, v.ID, reviewInput(v, v.ChangeSet.Digest, "Approve blindly")); !errors.Is(e, ErrForbidden) {
				t.Fatal(e)
			}
			v, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Checked the population; stricter target is intentional."))
			if e != nil {
				t.Fatal(e)
			}
			oldDigest := v.ChangeSet.Digest
			v, e = s.RejectChangeSet(ctx, v.ID, reviewInput(v, oldDigest, "Withdraw: need a representative backtest before production."))
			if e != nil || v.ChangeSet.State != "rejected" {
				t.Fatal(v.ChangeSet, e)
			}
			if _, e = s.ApplyChangeSet(ctx, v.ID, reviewInput(v, oldDigest, "")); !errors.Is(e, ErrConflict) {
				t.Fatal("rejected approval applied", e)
			}
			actual, _ := s.Platform.Store.Read(ctx, "intent-created")
			if actual.SLO.Target != .99 {
				t.Fatal("rejection changed SLO")
			}
			auditLog, e := s.Audit(ctx, v.ID)
			if e != nil || auditLog.Integrity != "verified" || len(auditLog.Events) != 5 || len(auditLog.ChangeSets) != 1 {
				t.Fatal(auditLog, e)
			}
			if auditLog.Events[1].Outcome != "denied" || auditLog.Events[2].Actor.Subject != "owner" || !strings.Contains(auditLog.Events[2].Reason, "population") {
				t.Fatal(auditLog.Events)
			}
			v, e = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil || v.ChangeSet.Digest == oldDigest || v.ChangeSet.ApprovedBy != nil || len(v.ChangeSetHistory) != 1 {
				t.Fatal("rejection lost or approval carried forward", v, e)
			}
		})
	}
}
func TestReviewedConfigurationRollbackAndIdempotentRetry(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := appliedUpdate(t, pg)
			v, e := s.PrepareRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Restore the previous target and CUJ definition after review."))
			if e != nil {
				t.Fatal(e)
			}
			rollback := v.ChangeSet.Rollback
			if rollback == nil || rollback.State != "proposed" || len(rollback.Changes) != 2 {
				t.Fatal(rollback)
			}
			current, _ := s.Platform.Store.Read(ctx, "intent-created")
			if current.SLO.Target != .999 || current.SLO.Revision != 2 {
				t.Fatal("preparing rollback changed definitions")
			}
			exported, e := s.ExportRollbackKubernetes(ctx, v.ID, "team-commerce", rollback.Digest)
			if e != nil || !strings.Contains(exported.YAML, "nines.dev/rollback-of") || !strings.Contains(exported.YAML, "ServiceLevelObjective") {
				t.Fatal(exported, e)
			}
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Subject: "agent", Role: "agent", Kind: "api_token"})
			if _, e = s.ReviewRollback(agent, v.ID, reviewInput(v, rollback.Digest, "No human approval"), false); !errors.Is(e, ErrForbidden) {
				t.Fatal(e)
			}
			v, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, rollback.Digest, "Verified previous settings and external rule deployment boundary."), false)
			if e != nil {
				t.Fatal(e)
			}
			apply := reviewInput(v, rollback.Digest, "")
			v, e = s.ReviewRollback(ctx, v.ID, apply, true)
			if e != nil {
				t.Fatal(e)
			}
			current, _ = s.Platform.Store.Read(ctx, "intent-created")
			if current.SLO.Target != .99 || current.SLO.Revision != 3 {
				t.Fatal(current.SLO)
			}
			journey, _ := s.Platform.Journeys.JourneyRevision(ctx, "checkout", 0)
			if journey.Revision != 3 || len(journey.Manifest.Spec.SLOs) != 1 {
				t.Fatal(journey)
			}
			if v.ChangeSet.Rollback.AppliedBy == nil || len(v.ChangeSet.Rollback.Applied) != 2 {
				t.Fatal(v.ChangeSet.Rollback)
			}
			receipt, err := s.Platform.Store.(store.DefinitionBatchStore).DefinitionBatch(ctx, v.ID+"/rollback/"+rollback.ID, rollback.Digest, true, func(store.Store) (json.RawMessage, error) { t.Error("receipt re-executed"); return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			var stored v1.IntentApplyReceipt
			if json.Unmarshal(receipt, &stored) != nil || stored.Evidence == nil || stored.Evidence.Operation != "rollback" || stored.Evidence.ApprovedBy.Subject != "owner" || stored.Actor.Subject != "owner" {
				t.Fatal(string(receipt))
			}
			later := model.DefinitionFromSLO(current.SLO)
			later.Spec.Target = .98
			later.Spec.Intent.Target = .98
			if _, e = s.Platform.ApplyDefinition(ctx, model.Definition{SLO: &later}, 3); e != nil {
				t.Fatal(e)
			}
			if _, e = s.ReviewRollback(ctx, v.ID, apply, true); e != nil {
				t.Fatal("exact retry failed", e)
			}
			current, _ = s.Platform.Store.Read(ctx, "intent-created")
			if current.SLO.Target != .98 || current.SLO.Revision != 4 {
				t.Fatal("retry overwrote a later edit")
			}
			log, e := s.Audit(ctx, v.ID)
			if e != nil || log.Integrity != "verified" {
				t.Fatal(log, e)
			}
			if pg {
				previous := s.Repository.(*postgres)
				dsn := previous.pool.Config().ConnString()
				previous.Close()
				reopened, err := NewPostgres(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(reopened.Close)
				restarted := New(s.Platform, reopened)
				recovered, err := restarted.Audit(ctx, v.ID)
				if err != nil || recovered.Integrity != "verified" || len(recovered.Events) != len(log.Events) || recovered.Events[len(recovered.Events)-1].Hash != log.Events[len(log.Events)-1].Hash {
					t.Fatal("audit did not survive repository restart", recovered, err)
				}
			}
		})
	}
}
func TestRollbackRejectsConcurrentEditsAndNewResourceDeletion(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := appliedUpdate(t, pg)
			v, e := s.PrepareRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Investigate regression; restore previous configuration."))
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, "Approved exact restore diff."), false)
			if e != nil {
				t.Fatal(e)
			}
			current, _ := s.Platform.Store.Read(ctx, "intent-created")
			next := model.DefinitionFromSLO(current.SLO)
			next.Spec.Target = .998
			next.Spec.Intent.Target = .998
			if _, e = s.Platform.ApplyDefinition(ctx, model.Definition{SLO: &next}, 2); e != nil {
				t.Fatal(e)
			}
			if _, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, ""), true); !errors.Is(e, model.ErrRevisionConflict) {
				t.Fatal("stale rollback applied", e)
			}
			journey, _ := s.Platform.Journeys.JourneyRevision(ctx, "checkout", 0)
			if journey.Revision != 2 {
				t.Fatal("rollback partially restored CUJ")
			}
			s, ctx, v = readyChangeFixture(t, pg)
			v, e = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Review new SLO and journey link."))
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ApplyChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.PrepareRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Request rollback of a newly created SLO."))
			if e != nil || v.ChangeSet.Rollback.State != "blocked" || len(v.ChangeSet.Rollback.Blockers) == 0 {
				t.Fatal(v.ChangeSet.Rollback, e)
			}
			if _, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, "Cannot approve deletion by implication."), false); !errors.Is(e, ErrConflict) {
				t.Fatal(e)
			}
			if _, e = s.ExportRollbackKubernetes(ctx, v.ID, "team-commerce", v.ChangeSet.Rollback.Digest); !errors.Is(e, ErrConflict) {
				t.Fatal("partial rollback exported", e)
			}
			journey, _ = s.Platform.Journeys.JourneyRevision(ctx, "checkout", 0)
			if journey.Revision != 2 {
				t.Fatal("blocked rollback changed CUJ")
			}
		})
	}
}

type failRollbackCommit struct {
	Repository
	fail bool
}

func (f *failRollbackCommit) Update(ctx context.Context, id string, fn func(*record, time.Time) error) (record, error) {
	return f.Repository.Update(ctx, id, func(r *record, now time.Time) error {
		if e := fn(r, now); e != nil {
			return e
		}
		if f.fail && r.Session.ChangeSet != nil && r.Session.ChangeSet.Rollback != nil && r.Session.ChangeSet.Rollback.State == "applied" {
			f.fail = false
			return errors.New("fixture conversation commit lost")
		}
		return nil
	})
}
func TestRollbackReceiptRecoversOriginalActorAfterConversationFailure(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := appliedUpdate(t, pg)
			v, e := s.PrepareRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Restore after failed rollout."))
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, "Checked the restore plan."), false)
			if e != nil {
				t.Fatal(e)
			}
			in := reviewInput(v, v.ChangeSet.Rollback.Digest, "")
			original := s.Repository
			s.Repository = &failRollbackCommit{Repository: original, fail: true}
			if _, e = s.ReviewRollback(ctx, v.ID, in, true); e == nil {
				t.Fatal("fixture did not fail")
			}
			current, _ := s.Platform.Store.Read(ctx, "intent-created")
			if current.SLO.Revision != 3 {
				t.Fatal("resource transaction did not commit")
			}
			other := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Subject: "second-owner", Name: "Second owner", Role: "owner", Kind: "user"})
			v, e = s.ReviewRollback(other, v.ID, in, true)
			if e != nil || v.ChangeSet.Rollback.AppliedBy.Subject != "owner" {
				t.Fatal(v.ChangeSet.Rollback, e)
			}
			current, _ = s.Platform.Store.Read(ctx, "intent-created")
			if current.SLO.Revision != 3 {
				t.Fatal("recovery executed rollback twice")
			}
			log, e := s.Audit(ctx, v.ID)
			if e != nil || log.Integrity != "verified" || log.Events[len(log.Events)-1].Actor.Subject != "owner" {
				t.Fatal(log, e)
			}
		})
	}
}
func TestAuditIntegrityAndCredentialRejection(t *testing.T) {
	s, ctx, v := updatedChangeFixture(t, false)
	v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if e != nil {
		t.Fatal(e)
	}
	key := "fixture-private-provider-key-0123456789"
	adapter, e := intentmodel.NewConfigured(intentmodel.Config{Provider: "custom", Model: "fixture", Endpoint: "http://127.0.0.1:1/chat/completions"}, key)
	if e != nil {
		t.Fatal(e)
	}
	s.Model = adapter
	if _, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, key)); e == nil || strings.Contains(e.Error(), key) {
		t.Fatal("credential-bearing note accepted or echoed", e)
	}
	log, e := s.Audit(ctx, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(log)
	if strings.Contains(string(raw), key) {
		t.Fatal("credential leaked into audit")
	}
	if _, e = s.Repository.Update(ctx, v.ID, func(r *record, _ time.Time) error { r.Session.Audit[0].Reason = "tampered"; return nil }); e != nil {
		t.Fatal(e)
	}
	log, e = s.Audit(ctx, v.ID)
	if e != nil || log.Integrity != "invalid" {
		t.Fatal(log, e)
	}
	if _, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Attempt after corruption")); e == nil {
		t.Fatal("approval appended to corrupt audit")
	}
	other := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "beta", Role: "owner", Subject: "other"})
	if _, e = s.Audit(other, v.ID); !errors.Is(e, ErrForbidden) {
		t.Fatal("cross-tenant audit read", e)
	}
}

func TestApprovalSupersessionIsAuditedAndAppliedHistoryCanBeRestored(t *testing.T) {
	s, ctx, v := updatedChangeFixture(t, false)
	v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed old proposal."))
	if e != nil {
		t.Fatal(e)
	}
	old := v.ChangeSet.Digest
	v, e = s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "clarify", ExpectedVersion: v.Version, Text: "Recheck excluded traffic."})
	if e != nil {
		t.Fatal(e)
	}
	if v.ChangeSet != nil || v.ChangeSetHistory[0].State != "superseded" || v.Audit[len(v.Audit)-1].Action != "change_set.superseded" {
		t.Fatal("approval invalidation is not explicit", v)
	}
	if _, e = s.ApplyChangeSet(ctx, v.ID, reviewInput(v, old, "")); !errors.Is(e, ErrConflict) {
		t.Fatal("superseded approval applied", e)
	}
	s, ctx, v = appliedUpdate(t, false)
	old = v.ChangeSet.Digest
	v, e = s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "next", ExpectedVersion: v.Version, Text: "Investigate another question."})
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.PrepareRollback(ctx, v.ID, reviewInput(v, old, "Restore the retained applied change."))
	if e != nil || v.ChangeSetHistory[0].Rollback == nil {
		t.Fatal(v, e)
	}
	digest := v.ChangeSetHistory[0].Rollback.Digest
	v, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, digest, "Review retained restore plan."), false)
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, digest, ""), true)
	if e != nil || v.ChangeSetHistory[0].Rollback.State != "applied" {
		t.Fatal(v, e)
	}
}

func TestHistoricalRecommendationTamperingBlocksNewApproval(t *testing.T) {
	s, ctx, v := updatedChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.RejectChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Request another review."))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Repository.Update(ctx, v.ID, func(r *record, _ time.Time) error {
		r.Session.ChangeSetHistory[0].Recommendation.Summary = "Tampered historical rationale"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	log, err := s.Audit(ctx, v.ID)
	if err != nil || log.Integrity != "invalid" {
		t.Fatal(log, err)
	}
	if _, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Review current proposal.")); err == nil {
		t.Fatal("approval accepted despite corrupted historical recommendation")
	}
	current, err := s.Platform.Store.Read(ctx, "intent-created")
	if err != nil || current.SLO.Target != .99 {
		t.Fatal("integrity failure changed SLO", err)
	}
}
