package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/auditarchive"
)

func (p *postgres) ClaimArchive(ctx context.Context, tenant, target string, days int, token string) (auditarchive.Job, error) {
	var out auditarchive.Job
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var raw []byte
	var id string
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT e.document,e.id,clock_timestamp() FROM intent_audit_events e LEFT JOIN intent_audit_archives a ON a.tenant=e.tenant AND a.record_id=e.id AND a.target=$2 WHERE e.tenant=$1 AND a.receipt IS NULL AND (a.record_id IS NULL OR (a.lease_until<=clock_timestamp() AND a.next_attempt<=clock_timestamp())) ORDER BY e.at,e.id LIMIT 1 FOR UPDATE OF e SKIP LOCKED`, tenant, target).Scan(&raw, &id, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, auditarchive.ErrEmpty
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Record); err != nil {
		return out, err
	}
	if err = validateJournalRecord(out.Record); err != nil {
		return out, err
	}
	retain := now.UTC().Truncate(time.Second).AddDate(0, 0, days)
	err = tx.QueryRow(ctx, `INSERT INTO intent_audit_archives(tenant,target,record_id,token,lease_until,next_attempt,retain_until,attempts) VALUES($1,$2,$3,$4,$5,$6,$7,1) ON CONFLICT(tenant,target,record_id) DO UPDATE SET token=$4,lease_until=$5,retain_until=CASE WHEN intent_audit_archives.retain_until<=clock_timestamp() THEN $7 ELSE intent_audit_archives.retain_until END,attempts=intent_audit_archives.attempts+1 RETURNING retain_until,attempts`, tenant, target, id, token, now.Add(90*time.Second), now, retain).Scan(&out.RetainUntil, &out.Attempts)
	if err != nil {
		return out, err
	}
	out.Token = token
	return out, tx.Commit(ctx)
}
func (p *postgres) FinishArchive(ctx context.Context, tenant, target, id, token string, receipt *v1.AuditArchiveReceipt, code string) error {
	var raw []byte
	var err error
	if receipt != nil {
		raw, err = json.Marshal(receipt)
		if err != nil {
			return err
		}
	}
	result, err := p.pool.Exec(ctx, `UPDATE intent_audit_archives SET receipt=$5,last_error=$6,token='',lease_until='epoch',next_attempt=clock_timestamp()+LEAST(3600,5*power(2,LEAST(attempts,10))) * interval '1 second' WHERE tenant=$1 AND target=$2 AND record_id=$3 AND token=$4 AND lease_until>clock_timestamp() AND receipt IS NULL`, tenant, target, id, token, raw, code)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return auditarchive.ErrLease
	}
	return nil
}
func (p *postgres) ArchiveStatus(ctx context.Context, tenant, target, id string) (v1.AuditArchiveStatus, error) {
	out := v1.AuditArchiveStatus{APIVersion: v1.APIVersion, Tenant: tenant, Enabled: true, Target: target, State: "pending", RecordID: id}
	var total int64
	err := p.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE a.receipt IS NULL),count(*) FILTER(WHERE a.receipt IS NULL AND a.last_error<>''),count(*) FILTER(WHERE a.receipt IS NOT NULL),count(*) FILTER(WHERE a.receipt IS NOT NULL AND (a.receipt->>'retainUntil')::timestamptz>clock_timestamp()),count(*) FILTER(WHERE a.receipt IS NOT NULL AND (a.receipt->>'retainUntil')::timestamptz<=clock_timestamp()),COALESCE(max(a.last_error) FILTER(WHERE a.receipt IS NULL),'') FROM intent_audit_events e LEFT JOIN intent_audit_archives a ON a.tenant=e.tenant AND a.record_id=e.id AND a.target=$2 WHERE e.tenant=$1 AND ($3='' OR e.id=NULLIF($3,'')::uuid)`, tenant, target, id).Scan(&total, &out.Pending, &out.Failed, &out.Verified, &out.Retained, &out.Expired, &out.LastError)
	if err != nil {
		return out, err
	}
	if id != "" && total == 0 {
		return out, ErrNotFound
	}
	if out.Pending == 0 {
		out.State = "verified"
	}
	if out.Failed > 0 {
		out.State = "retrying"
	}
	if id != "" && out.Verified == 1 {
		var raw []byte
		err = p.pool.QueryRow(ctx, `SELECT receipt FROM intent_audit_archives WHERE tenant=$1 AND target=$2 AND record_id=$3`, tenant, target, id).Scan(&raw)
		if err != nil {
			return out, err
		}
		out.Receipt = &v1.AuditArchiveReceipt{}
		err = json.Unmarshal(raw, out.Receipt)
	}
	return out, err
}
