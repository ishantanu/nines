package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/workflowhook"
)

func (p *postgres) ClaimWorkflowEvent(ctx context.Context, tenant, target string, since time.Time, token string) (workflowhook.Job, error) {
	var job workflowhook.Job
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return job, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var raw []byte
	var id string
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT e.document,e.id,clock_timestamp() FROM intent_audit_events e LEFT JOIN intent_webhook_deliveries d ON d.tenant=e.tenant AND d.record_id=e.id AND d.target=$2 WHERE e.tenant=$1 AND e.at >= $3 AND e.outcome IN ('succeeded','blocked') AND e.action IN ('change_set.proposed','change_set.approved','change_set.applied','change_set.rejected','rollback.proposed','rollback.approved','rollback.applied','rollback.rejected','change_set.observed','rollback.observed') AND NOT COALESCE(d.delivered,false) AND (d.record_id IS NULL OR (d.lease_until<=clock_timestamp() AND d.next_attempt<=clock_timestamp())) ORDER BY e.at,e.id LIMIT 1 FOR UPDATE OF e SKIP LOCKED`, tenant, target, since).Scan(&raw, &id, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, workflowhook.ErrEmpty
	}
	if err != nil {
		return job, err
	}
	var record v1.AuditRecord
	if err = json.Unmarshal(raw, &record); err != nil {
		return job, err
	}
	if err = validateJournalRecord(record); err != nil {
		return job, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO intent_webhook_deliveries(tenant,target,record_id,token,lease_until,next_attempt,attempts) VALUES($1,$2,$3,$4,$5,$6,1) ON CONFLICT(tenant,target,record_id) DO UPDATE SET token=EXCLUDED.token,lease_until=EXCLUDED.lease_until,attempts=intent_webhook_deliveries.attempts+1 RETURNING attempts`, tenant, target, id, token, now.Add(90*time.Second), now).Scan(&job.Attempts)
	if err != nil {
		return job, err
	}
	if err = tx.Commit(ctx); err != nil {
		return job, err
	}
	job.Record = record
	job.Token = token
	return job, nil
}
func (p *postgres) FinishWorkflowEvent(ctx context.Context, tenant, target, id, token string, delivered bool, code string) error {
	result, err := p.pool.Exec(ctx, `UPDATE intent_webhook_deliveries SET delivered=$5,last_error=$6,token='',lease_until='epoch',next_attempt=clock_timestamp()+LEAST(3600,5*power(2,LEAST(attempts,10)))*interval '1 second' WHERE tenant=$1 AND target=$2 AND record_id=$3 AND token=$4 AND lease_until>clock_timestamp() AND NOT delivered`, tenant, target, id, token, delivered, code)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return workflowhook.ErrLease
	}
	return nil
}
