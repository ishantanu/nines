package replay

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/observability"
)

type postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, dsn string) (Repository, error) {
	p, err := observability.NewPool(ctx, dsn)
	if err != nil {
		return nil, err
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		p.Close()
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(76449219)`)
	if err != nil {
		_ = tx.Rollback(context.Background())
		p.Close()
		return nil, err
	}
	_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS browser_session_replays(id text PRIMARY KEY,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL,metadata bytea NOT NULL,document bytea NOT NULL);CREATE INDEX IF NOT EXISTS browser_session_replays_expiry ON browser_session_replays(expires_at);`)
	if err != nil {
		_ = tx.Rollback(context.Background())
		p.Close()
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		_ = tx.Rollback(context.Background())
		p.Close()
		return nil, err
	}
	return &postgres{p}, nil
}
func (p *postgres) Close() { p.pool.Close() }
func scan(row pgx.Row) (record, error) {
	var r record
	var b []byte
	err := row.Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &r)
	}
	return r, err
}
func (p *postgres) Prune(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM browser_session_replays WHERE expires_at<=clock_timestamp()`)
	return err
}
func (p *postgres) Read(ctx context.Context, id string) (record, error) {
	return scan(p.pool.QueryRow(ctx, `SELECT document FROM browser_session_replays WHERE id=$1 AND expires_at>clock_timestamp()`, id))
}
func (p *postgres) List(ctx context.Context) ([]v1.SessionReplay, error) {
	rows, err := p.pool.Query(ctx, `SELECT metadata FROM browser_session_replays WHERE expires_at>clock_timestamp() ORDER BY created_at DESC,id LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []v1.SessionReplay{}
	for rows.Next() {
		var b []byte
		var m v1.SessionReplay
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (p *postgres) Create(ctx context.Context, r record) (record, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(76449218)`)
	if err != nil {
		return r, err
	}
	_, err = tx.Exec(ctx, `DELETE FROM browser_session_replays WHERE expires_at<=clock_timestamp()`)
	if err != nil {
		return r, err
	}
	old, err := scan(tx.QueryRow(ctx, `SELECT document FROM browser_session_replays WHERE id=$1`, r.Meta.ID))
	if err == nil {
		if old.RequestDigest != r.RequestDigest {
			return r, ErrConflict
		}
		return old, tx.Commit(ctx)
	}
	if err != ErrNotFound {
		return r, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM browser_session_replays`).Scan(&count); err != nil {
		return r, err
	}
	if count >= MaxSessions {
		return r, ErrLimit
	}
	meta, _ := json.Marshal(r.Meta)
	b, _ := json.Marshal(r)
	_, err = tx.Exec(ctx, `INSERT INTO browser_session_replays VALUES($1,$2,$3,$4,$5)`, r.Meta.ID, r.Meta.ExpiresAt, r.Meta.CreatedAt, meta, b)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (p *postgres) Update(ctx context.Context, id string, fn func(*record) error) (record, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return record{}, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	r, err := scan(tx.QueryRow(ctx, `SELECT document FROM browser_session_replays WHERE id=$1 AND expires_at>clock_timestamp() FOR UPDATE`, id))
	if err != nil {
		return r, err
	}
	if err = fn(&r); err != nil {
		return r, err
	}
	b, _ := json.Marshal(r)
	meta, _ := json.Marshal(r.Meta)
	_, err = tx.Exec(ctx, `UPDATE browser_session_replays SET document=$2,metadata=$3,expires_at=$4 WHERE id=$1`, id, b, meta, r.Meta.ExpiresAt)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

func (p *postgres) Delete(ctx context.Context, id string, version int64) error {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	r, e := scan(tx.QueryRow(ctx, `SELECT document FROM browser_session_replays WHERE id=$1 AND expires_at>clock_timestamp() FOR UPDATE`, id))
	if e != nil {
		return e
	}
	if r.Meta.Version != version {
		return ErrConflict
	}
	if _, e = tx.Exec(ctx, `DELETE FROM browser_session_replays WHERE id=$1`, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
