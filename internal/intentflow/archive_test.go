package intentflow

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/auditarchive"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

type archiveSink struct {
	calls      int
	fail       bool
	seenTenant string
}

func (s *archiveSink) Store(_ context.Context, job auditarchive.Job) (v1.AuditArchiveReceipt, error) {
	s.calls++
	s.seenTenant = job.Record.Tenant
	if s.fail {
		return v1.AuditArchiveReceipt{}, auditarchive.ErrRetention
	}
	return v1.AuditArchiveReceipt{Bucket: "test", Key: job.Record.ID, VersionID: "version", Mode: "COMPLIANCE", SHA256: "hash", RetainUntil: job.RetainUntil, VerifiedAt: time.Now().UTC()}, nil
}
func TestArchiveDeliveryLeaseRetryPersistenceAndTenantScope(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil {
				t.Fatal(err)
			}
			config := auditarchive.Config{Bucket: "test-bucket", Region: "us-east-1", ExpectedBucketOwner: "123456789012", Prefix: "audit", RetentionDays: 30}
			target := config.Target()
			job, err := s.Repository.ClaimArchive(ctx, "alpha", target, 30, "worker-one")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Repository.ClaimArchive(ctx, "alpha", target, 30, "worker-two"); !errors.Is(err, auditarchive.ErrEmpty) {
				t.Fatal("leased work claimed twice", err)
			}
			if _, err = s.Repository.ClaimArchive(ctx, "another", target, 30, "other"); !errors.Is(err, auditarchive.ErrEmpty) {
				t.Fatal("cross-tenant event claimed", err)
			}
			if err = s.Repository.FinishArchive(ctx, "alpha", target, job.Record.ID, "wrong-token", nil, "destination_unavailable"); !errors.Is(err, auditarchive.ErrLease) {
				t.Fatal(err)
			}
			if err = s.Repository.FinishArchive(ctx, "another", target, job.Record.ID, "worker-one", nil, "destination_unavailable"); !errors.Is(err, auditarchive.ErrLease) {
				t.Fatal(err)
			}
			if err = s.Repository.FinishArchive(ctx, "alpha", target, job.Record.ID, "worker-one", nil, "retention_not_verified"); err != nil {
				t.Fatal(err)
			}
			status, err := s.Repository.ArchiveStatus(ctx, "alpha", target, job.Record.ID)
			if err != nil || status.Verified != 0 || status.Pending != 1 || status.Failed != 1 || status.Receipt != nil {
				t.Fatal(status, err)
			}
			if _, err = s.Repository.ClaimArchive(ctx, "alpha", target, 30, "too-soon"); !errors.Is(err, auditarchive.ErrEmpty) {
				t.Fatal("retry backoff ignored", err)
			}
			// Advance only this synthetic job's retry/lease clocks, without sleeping.
			if pg {
				p := s.Repository.(*postgres)
				if _, err = p.pool.Exec(ctx, `UPDATE intent_audit_archives SET next_attempt='epoch',lease_until='epoch'`); err != nil {
					t.Fatal(err)
				}
			} else {
				m := s.Repository.(*memory)
				state := m.archives[target+"/"+job.Record.ID]
				state.NextAttempt = time.Time{}
				state.LeaseUntil = time.Time{}
				m.archives[target+"/"+job.Record.ID] = state
			}
			sink := &archiveSink{}
			worker := &auditarchive.Worker{Tenant: "alpha", Config: config, Repository: s.Repository, Sink: sink}
			s.Archive = worker
			if found, err := worker.WorkOne(ctx); !found || err != nil {
				t.Fatal(found, err)
			}
			status, err = s.ArchiveStatus(ctx, job.Record.ID)
			if err != nil || status.Verified != 1 || status.Pending != 0 || status.Retained != 1 || status.Receipt == nil || !status.Receipt.RetainUntil.Equal(job.RetainUntil) {
				t.Fatal(status, err)
			}
			if found, err := worker.WorkOne(ctx); found || err != nil || sink.calls != 1 || sink.seenTenant != "alpha" {
				t.Fatal(found, err, sink)
			}
			if pg {
				p := s.Repository.(*postgres)
				reopened, err := NewPostgres(ctx, p.pool.Config().ConnString())
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				saved, err := reopened.ArchiveStatus(ctx, "alpha", target, job.Record.ID)
				if err != nil || saved.Receipt == nil || saved.Receipt.VersionID != "version" {
					t.Fatal(saved, err)
				}
			}
			role, _ := tenancy.FromContext(ctx)
			role.Role = "viewer"
			if _, err = s.ArchiveStatus(tenancy.WithPrincipal(ctx, role), ""); !errors.Is(err, ErrForbidden) {
				t.Fatal(err)
			}
			if _, err = s.ArchiveStatus(ctx, model.NewID()); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			// A changed destination/policy has an independent backlog; old receipts are not reused.
			config.Prefix = "changed"
			s.Archive.Config = config
			status, err = s.ArchiveStatus(ctx, "")
			if err != nil || status.Pending != 1 || status.Verified != 0 {
				t.Fatal(status, err)
			}
			changedTarget := config.Target()
			pending, err := s.Repository.ClaimArchive(ctx, "alpha", changedTarget, 30, "expired-generation")
			if err != nil {
				t.Fatal(err)
			}
			expired := time.Now().UTC().Add(-time.Hour)
			if pg {
				db := s.Repository.(*postgres)
				if _, err = db.pool.Exec(ctx, `UPDATE intent_audit_archives SET retain_until=$1,lease_until='epoch',next_attempt='epoch' WHERE target=$2`, expired, changedTarget); err != nil {
					t.Fatal(err)
				}
			} else {
				m := s.Repository.(*memory)
				state := m.archives[changedTarget+"/"+pending.Record.ID]
				state.RetainUntil = expired
				state.LeaseUntil = time.Time{}
				state.NextAttempt = time.Time{}
				m.archives[changedTarget+"/"+pending.Record.ID] = state
			}
			renewed, err := s.Repository.ClaimArchive(ctx, "alpha", changedTarget, 30, "fresh-generation")
			if err != nil || !renewed.RetainUntil.After(time.Now()) || renewed.Attempts != 2 {
				t.Fatal(renewed, err)
			}

		})
	}
}
