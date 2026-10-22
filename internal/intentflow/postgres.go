package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/observability"
)

type postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, dsn string) (Repository, error) {
	p, e := observability.NewPool(ctx, dsn)
	if e != nil {
		return nil, e
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		p.Close()
		return nil, e
	}
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(88621950)`); e == nil {
		_, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS intent_conversations(id text PRIMARY KEY,summary jsonb NOT NULL,document bytea NOT NULL,status text NOT NULL,lease_until timestamptz NOT NULL);CREATE INDEX IF NOT EXISTS intent_conversations_queue ON intent_conversations(status,lease_until,id)`)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		_ = tx.Rollback(context.Background())
		p.Close()
		return nil, e
	}
	return &postgres{p}, nil
}
func (p *postgres) Close() { p.pool.Close() }
func decodeRow(row pgx.Row) (record, error) {
	var b []byte
	var r record
	e := row.Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func (p *postgres) Read(ctx context.Context, id string) (record, error) {
	return decodeRow(p.pool.QueryRow(ctx, `SELECT document FROM intent_conversations WHERE id=$1`, id))
}
func writeRecord(ctx context.Context, tx pgx.Tx, r record) error {
	if e := validSize(r); e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	s, _ := json.Marshal(r.Session.IntentConversationSummary)
	_, e = tx.Exec(ctx, `UPDATE intent_conversations SET summary=$2,document=$3,status=$4,lease_until=$5 WHERE id=$1`, r.Session.ID, s, b, r.Session.State, r.LeaseUntil)
	return e
}
func (p *postgres) Create(ctx context.Context, r record) (record, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return record{}, e
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(88621951)`); e != nil {
		return record{}, e
	}
	old, e := decodeRow(tx.QueryRow(ctx, `SELECT document FROM intent_conversations WHERE id=$1`, r.Session.ID))
	if e == nil {
		if old.RequestDigest != r.RequestDigest {
			return record{}, ErrConflict
		}
		return old, tx.Commit(ctx)
	}
	if e != ErrNotFound {
		return record{}, e
	}
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM intent_conversations`).Scan(&n); e != nil {
		return record{}, e
	}
	if n >= maxSessions {
		return record{}, ErrLimit
	}
	if e = validSize(r); e != nil {
		return record{}, e
	}
	b, _ := json.Marshal(r)
	s, _ := json.Marshal(r.Session.IntentConversationSummary)
	if _, e = tx.Exec(ctx, `INSERT INTO intent_conversations VALUES($1,$2,$3,$4,$5)`, r.Session.ID, s, b, r.Session.State, r.LeaseUntil); e != nil {
		return record{}, e
	}
	return clone(r), tx.Commit(ctx)
}
func (p *postgres) List(ctx context.Context, after string, limit int) ([]v1.IntentConversationSummary, error) {
	rows, e := p.pool.Query(ctx, `SELECT summary FROM intent_conversations WHERE id>$1 ORDER BY id LIMIT $2`, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []v1.IntentConversationSummary{}
	for rows.Next() {
		var b []byte
		var s v1.IntentConversationSummary
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &s); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (p *postgres) Update(ctx context.Context, id string, fn func(*record, time.Time) error) (record, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return record{}, e
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	var now time.Time
	e = tx.QueryRow(ctx, `SELECT document,clock_timestamp() FROM intent_conversations WHERE id=$1 FOR UPDATE`, id).Scan(&b, &now)
	if errors.Is(e, pgx.ErrNoRows) {
		return record{}, ErrNotFound
	}
	if e != nil {
		return record{}, e
	}
	var r record
	if e = json.Unmarshal(b, &r); e != nil {
		return record{}, e
	}
	if e = fn(&r, now.UTC()); e != nil {
		return record{}, e
	}
	if e = writeRecord(ctx, tx, r); e != nil {
		return record{}, e
	}
	return r, tx.Commit(ctx)
}
func (p *postgres) Claim(ctx context.Context, token string) (record, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return record{}, e
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	var now time.Time
	e = tx.QueryRow(ctx, `SELECT document,clock_timestamp() FROM intent_conversations WHERE status IN ('queued','running') AND lease_until<=clock_timestamp() ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&b, &now)
	if errors.Is(e, pgx.ErrNoRows) {
		return record{}, ErrNotFound
	}
	if e != nil {
		return record{}, e
	}
	var r record
	if e = json.Unmarshal(b, &r); e != nil {
		return record{}, e
	}
	if e = claim(&r, token, now.UTC()); e != nil {
		return record{}, e
	}
	if e = writeRecord(ctx, tx, r); e != nil {
		return record{}, e
	}
	return r, tx.Commit(ctx)
}
