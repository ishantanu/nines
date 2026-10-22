package investigation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

const runTableDDL = `CREATE TABLE IF NOT EXISTS reliability_review_runs(
 review_id text NOT NULL REFERENCES reliability_investigations(id),
 id text NOT NULL, state text NOT NULL, created_at timestamptz NOT NULL,
 lease_token text NOT NULL DEFAULT '', lease_until timestamptz NOT NULL DEFAULT 'epoch',
 document bytea NOT NULL, PRIMARY KEY(review_id,id));
 ALTER TABLE reliability_review_runs ADD COLUMN IF NOT EXISTS private_state bytea NOT NULL DEFAULT '{}'::bytea;
 CREATE INDEX IF NOT EXISTS reliability_review_runs_ready ON reliability_review_runs(created_at) WHERE state IN ('queued','running');
 CREATE INDEX IF NOT EXISTS reliability_review_runs_ready_v2 ON reliability_review_runs(created_at) WHERE state IN ('queued','running','model_queued','model_running');
 CREATE INDEX IF NOT EXISTS reliability_review_runs_ready_v3 ON reliability_review_runs(created_at) WHERE state IN ('queued','running','model_queued','model_running','skill_queued','skill_running');
 CREATE UNIQUE INDEX IF NOT EXISTS reliability_review_runs_one_active_v2 ON reliability_review_runs(review_id) WHERE state IN ('queued','running','model_queued','model_running','skill_queued','skill_running');
 CREATE UNIQUE INDEX IF NOT EXISTS reliability_review_runs_one_active ON reliability_review_runs(review_id) WHERE state IN ('queued','running','model_queued','model_running');`

func scanRun(row pgx.Row) (runRecord, error) {
	var r runRecord
	var b, private []byte
	err := row.Scan(&b, &r.Token, &r.Until, &r.Now, &private)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &r.Run)
		if err == nil {
			err = json.Unmarshal(private, &r.Model)
		}
	}
	return r, err
}
func saveRun(ctx context.Context, tx pgx.Tx, r runRecord) error {
	if err := validRunRecord(r); err != nil {
		return err
	}
	private, err := json.Marshal(r.Model)
	if err != nil {
		return err
	}
	b, err := json.Marshal(r.Run)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE reliability_review_runs SET state=$3,lease_token=$4,lease_until=$5,document=$6,private_state=$7 WHERE review_id=$1 AND id=$2`, r.Run.InvestigationID, r.Run.ID, storageState(r.Run), r.Token, r.Until, b, private)
	return err
}
func (p *postgres) CreateRun(ctx context.Context, v v1.InvestigationRun) (v1.InvestigationRun, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	err = tx.QueryRow(ctx, `SELECT document FROM reliability_investigations WHERE id=$1 FOR UPDATE`, v.InvestigationID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	var parent v1.Investigation
	if err = json.Unmarshal(b, &parent); err != nil {
		return v, err
	}
	if parent.Tenant != v.Tenant {
		return v, ErrNotFound
	}
	old, err := scanRun(tx.QueryRow(ctx, `SELECT document,lease_token,lease_until,clock_timestamp(),private_state FROM reliability_review_runs WHERE review_id=$1 AND id=$2`, v.InvestigationID, v.ID))
	if err == nil {
		if !sameRun(old.Run, v) {
			return v, ErrConflict
		}
		return old.Run, nil
	}
	if err != ErrNotFound {
		return v, err
	}
	if parent.Version != v.ReviewVersion || parent.Scope != v.Scope {
		return v, ErrConflict
	}
	var count, active int
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state IN ('queued','running','model_queued','model_running','skill_queued','skill_running')) FROM reliability_review_runs WHERE review_id=$1`, v.InvestigationID).Scan(&count, &active); err != nil {
		return v, err
	}
	if active != 0 {
		return v, ErrConflict
	}
	if count >= 20 {
		return v, model.Invalid("review run limit reached; create a new review")
	}
	b, err = json.Marshal(v)
	if err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reliability_review_runs(review_id,id,state,created_at,document) VALUES($1,$2,$3,$4,$5)`, v.InvestigationID, v.ID, storageState(v), v.CreatedAt, b); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return v, ErrConflict
		}
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (p *postgres) ReadRun(ctx context.Context, review, id string) (v1.InvestigationRun, error) {
	r, err := scanRun(p.pool.QueryRow(ctx, `SELECT document,lease_token,lease_until,clock_timestamp(),private_state FROM reliability_review_runs WHERE review_id=$1 AND id=$2`, review, id))
	return r.Run, err
}
func (p *postgres) ListRuns(ctx context.Context, review string) ([]v1.InvestigationRun, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM reliability_review_runs WHERE review_id=$1 ORDER BY created_at,id LIMIT 20`, review)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []v1.InvestigationRun{}
	for rows.Next() {
		var b []byte
		var r v1.InvestigationRun
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (p *postgres) updateRun(ctx context.Context, review, id string, fn func(*runRecord) error) (v1.InvestigationRun, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return v1.InvestigationRun{}, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	r, err := scanRun(tx.QueryRow(ctx, `SELECT document,lease_token,lease_until,clock_timestamp(),private_state FROM reliability_review_runs WHERE review_id=$1 AND id=$2 FOR UPDATE`, review, id))
	if err != nil {
		return r.Run, err
	}
	// clock_timestamp is read again after acquiring a contended row lock.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.Now); err != nil {
		return r.Run, err
	}
	if err = fn(&r); err != nil {
		return v1.InvestigationRun{}, err
	}
	if err = saveRun(ctx, tx, r); err != nil {
		return r.Run, err
	}
	return r.Run, tx.Commit(ctx)
}
func (p *postgres) claimRun(ctx context.Context, token string) (runRecord, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return runRecord{}, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	r, err := scanRun(tx.QueryRow(ctx, `SELECT document,lease_token,lease_until,clock_timestamp(),private_state FROM reliability_review_runs WHERE state IN ('queued','running','model_queued','model_running','skill_queued','skill_running') AND lease_until<=clock_timestamp() ORDER BY created_at,review_id,id FOR UPDATE SKIP LOCKED LIMIT 1`))
	if err != nil {
		return r, err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.Now); err != nil {
		return r, err
	}
	acquire(&r, token)
	if err = saveRun(ctx, tx, r); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

// Older workers claim only queued/running. Model jobs use distinct storage states
// so an older binary cannot execute an unsupported plan during a rolling upgrade.
func storageState(run v1.InvestigationRun) string {
	if run.Skill != nil && !terminal(run.Status) {
		return "skill_" + run.Status
	}
	if run.Mode == "model" && !terminal(run.Status) {
		return "model_" + run.Status
	}
	return run.Status
}
