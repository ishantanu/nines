package intentflow

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestJournalRetainsHistoryBeyondConversationLimit(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil {
				t.Fatal(err)
			}
			original := clone(*v.ChangeSet)
			for i := 0; i < 260; i++ {
				if _, err = s.PrepareChangeSet(ctx, v.ID, v.Version+100); !errors.Is(err, ErrConflict) {
					t.Fatal(err)
				}
			}
			recent, err := s.Read(ctx, v.ID)
			if err != nil || len(recent.Audit) != 256 || recent.Audit[0].Sequence != 6 {
				t.Fatal(len(recent.Audit), err)
			}
			first, err := s.Audit(ctx, v.ID)
			if err != nil || first.Integrity != "verified" || len(first.Events) != 256 || first.Events[0].Sequence != 1 || first.NextAfter != 256 || first.Through != 261 {
				t.Fatal(first, err)
			}
			second, err := s.AuditPage(ctx, v.ID, first.NextAfter, first.Through)
			if err != nil || second.Integrity != "verified" || len(second.Events) != 5 || second.NextAfter != 0 || second.Events[0].PreviousHash != first.Events[255].Hash {
				t.Fatal(second, err)
			}
			// A concurrent append cannot change the frozen export range.
			s.auditFailure(ctx, v.ID, "change_set.proposed", "", ErrConflict)
			again, err := s.AuditPage(ctx, v.ID, first.NextAfter, first.Through)
			if err != nil || len(again.Events) != 5 || again.Through != 261 {
				t.Fatal(again, err)
			}
			page, err := s.ListAudit(ctx, v1.AuditQuery{Conversation: v.ID, Action: "change_set.proposed", Outcome: "succeeded", Resource: "intent-created", Limit: 1})
			if err != nil || len(page.Items) != 1 || page.Items[0].Proposal != nil {
				t.Fatal(page, err)
			}
			saved, err := s.AuditRecord(ctx, page.Items[0].ID)
			if err != nil || saved.Proposal == nil || saved.Proposal.Digest != original.Digest || hash(saved.Proposal.Recommendation) != hash(original.Recommendation) {
				t.Fatal(saved, err)
			}
			if pg {
				db := s.Repository.(*postgres)
				reopened, err := NewPostgres(ctx, db.pool.Config().ConnString())
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				rows, err := reopened.Audit(ctx, AuditSearch{Tenant: "alpha", ID: saved.ID, Query: v1.AuditQuery{Limit: 1}})
				if err != nil || len(rows) != 1 || rows[0].Proposal.Digest != original.Digest {
					t.Fatal(rows, err)
				}
			}
		})
	}
}
func TestJournalPaginationScopeAndAccess(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			for i := 0; i < 4; i++ {
				s.auditFailure(ctx, v.ID, "change_set.proposed", "", ErrConflict)
			}
			q := v1.AuditQuery{Conversation: v.ID, Actor: "owner", Resource: "intent-created", Outcome: "conflict", Limit: 2}
			first, err := s.ListAudit(ctx, q)
			if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
				t.Fatal(first, err)
			}
			s.auditFailure(ctx, v.ID, "change_set.proposed", "", ErrConflict)
			q.Cursor = first.NextCursor
			second, err := s.ListAudit(ctx, q)
			if err != nil || len(second.Items) != 2 || second.NextCursor != "" || second.Items[0].ID == first.Items[0].ID {
				t.Fatal(second, err)
			}
			q.Actor = "other"
			if _, err = s.ListAudit(ctx, q); err == nil {
				t.Fatal("cursor accepted for changed filters")
			}
			p, _ := tenancy.FromContext(ctx)
			for _, role := range []string{"viewer", "agent"} {
				p.Role = role
				restricted := tenancy.WithPrincipal(ctx, p)
				if _, err = s.ListAudit(restricted, v1.AuditQuery{}); !errors.Is(err, ErrForbidden) {
					t.Fatal(role, err)
				}
				if _, err = s.AuditRecord(restricted, first.Items[0].ID); !errors.Is(err, ErrForbidden) {
					t.Fatal(role, err)
				}
			}
			p.Role = "owner"
			p.TenantID = "another"
			if _, err = s.ListAudit(tenancy.WithPrincipal(ctx, p), v1.AuditQuery{}); !errors.Is(err, ErrForbidden) {
				t.Fatal(err)
			}
			originalTenant := s.Platform.TenantID
			s.Platform.TenantID = "another"
			q = v1.AuditQuery{Cursor: first.NextCursor, Conversation: v.ID, Actor: "owner", Outcome: "conflict"}
			if _, err = s.ListAudit(tenancy.WithPrincipal(ctx, p), q); err == nil {
				t.Fatal("cross-tenant cursor accepted")
			}
			s.Platform.TenantID = originalTenant
			for _, q := range []v1.AuditQuery{{Limit: 101}, {Limit: -1}, {Action: "invalid"}, {Outcome: "anything"}, {From: time.Now(), To: time.Now().Add(-time.Hour)}} {
				if _, err = s.ListAudit(ctx, q); err == nil {
					t.Fatal(q)
				}
			}
		})
	}
}
func TestAuditHTTPStrictQueriesAndFrozenRecord(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Mount(mux, s)
	for _, path := range []string{"/api/audit?tenant=other", "/api/audit?limit=1&limit=2", "/api/audit?limit=0", "/api/audit?from=bad", "/api/audit?%zz", "/api/intent-conversations/" + v.ID + "/audit?after=-1"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil).WithContext(ctx))
		if w.Code != 400 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/audit", nil).WithContext(ctx))
	var page v1.AuditPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/audit/"+page.Items[0].ID, nil).WithContext(ctx))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "recommendation") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err = s.AuditRecord(ctx, model.NewID()); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
func TestRollbackRequiresIndependentApplier(t *testing.T) {
	s, ctx, v := appliedUpdate(t, false)
	v, err := s.PrepareRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Restore prior target after review."))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, "Reviewed prior definitions and current revisions."), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, ""), true); !errors.Is(err, ErrForbidden) {
		t.Fatal("self-applied rollback", err)
	}
	if _, err = s.ReviewRollback(independentApplier(ctx), v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, ""), true); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedProposalAttemptIsAuditedWithoutBody(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	mux := http.NewServeMux()
	Mount(mux, s)
	req := httptest.NewRequest("POST", "/api/intent-conversations/"+v.ID+"/change-set", strings.NewReader(`{"expectedVersion":1,"unexpected":"private-sentinel"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	log, err := s.Audit(ctx, v.ID)
	raw, _ := json.Marshal(log)
	if w.Code != 400 || err != nil || len(log.Events) != 1 || log.Events[0].Outcome != "failed" || strings.Contains(string(raw), "private-sentinel") {
		t.Fatal(w.Code, log, err)
	}
}

func TestFirstJournalStartRecoversRetainedAuditWithoutHidingLaterDeletion(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, true)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	db := s.Repository.(*postgres)
	dsn := db.pool.Config().ConnString()
	if _, err = db.pool.Exec(ctx, `DROP TABLE intent_audit_events`); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	s.Repository = recovered
	log, err := s.Audit(ctx, v.ID)
	if err != nil || log.Integrity != "verified" || len(log.Events) != 1 {
		t.Fatal(log, err)
	}
	page, err := s.ListAudit(ctx, v1.AuditQuery{Conversation: v.ID})
	if err != nil || len(page.Items) != 1 || !page.Items[0].RecoveredFromConversation {
		t.Fatal(page, err)
	}
	saved, err := s.AuditRecord(ctx, page.Items[0].ID)
	if err != nil || saved.Proposal == nil || saved.Proposal.Digest != v.ChangeSet.Digest {
		t.Fatal(saved, err)
	}
	if _, err = db.pool.Exec(ctx, `DELETE FROM intent_audit_events`); err != nil {
		t.Fatal(err)
	}
	again, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	s.Repository = again
	log, err = s.Audit(ctx, v.ID)
	if err != nil || log.Integrity != "invalid" {
		t.Fatal("ordinary restart concealed deleted journal", log, err)
	}
	if _, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed legacy proposal.")); err == nil {
		t.Fatal("approval accepted after journal head deletion")
	}
	current, err := s.Read(ctx, v.ID)
	if err != nil || current.ChangeSet.State != "proposed" {
		t.Fatal(current, err)
	}
}
