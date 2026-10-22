package workflow

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ishantanu/nines/internal/observability"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgres struct{ pool *pgxpool.Pool }

// NewPostgres honors the search_path in the server-owned connection URL. The
// caller must use the same isolated tenant schema as its platform store.
func NewPostgres(ctx context.Context, url string) (Repository, error) {
	pool, err := observability.NewPool(ctx, url)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(654239120)"); err == nil {
		_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS reliability_workflows(id text PRIMARY KEY,created_at timestamptz NOT NULL,document jsonb NOT NULL); CREATE INDEX IF NOT EXISTS reliability_workflows_created ON reliability_workflows(created_at DESC,id);`)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &postgres{pool}, nil
}
func (p *postgres) Close() { p.pool.Close() }
func (p *postgres) Create(ctx context.Context, w Workflow) (Workflow, error) {
	b, err := json.Marshal(w)
	if err != nil {
		return w, err
	}
	tag, err := p.pool.Exec(ctx, "INSERT INTO reliability_workflows(id,created_at,document) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", w.ID, w.CreatedAt, b)
	if err != nil {
		return w, err
	}
	if tag.RowsAffected() == 1 {
		return w, nil
	}
	old, err := p.Get(ctx, w.ID)
	if err != nil {
		return w, err
	}
	if old.RequestDigest != w.RequestDigest {
		return Workflow{}, ErrConflict
	}
	return old, nil
}
func (p *postgres) Get(ctx context.Context, id string) (Workflow, error) {
	var w Workflow
	var b []byte
	err := p.pool.QueryRow(ctx, "SELECT document FROM reliability_workflows WHERE id=$1", id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	if err != nil {
		return w, err
	}
	err = json.Unmarshal(b, &w)
	return w, err
}
func (p *postgres) List(ctx context.Context) ([]Workflow, error) {
	rows, err := p.pool.Query(ctx, "SELECT document FROM reliability_workflows ORDER BY created_at DESC,id LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workflow{}
	for rows.Next() {
		var b []byte
		var w Workflow
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (p *postgres) Recoverable(ctx context.Context) ([]Workflow, error) {
	rows, err := p.pool.Query(ctx, "SELECT document FROM reliability_workflows WHERE document->>'state'='approved' ORDER BY created_at,id LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workflow{}
	for rows.Next() {
		var b []byte
		var w Workflow
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (p *postgres) Update(ctx context.Context, id string, fn func(*Workflow) error) (Workflow, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Workflow{}, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var w Workflow
	var b []byte
	err = tx.QueryRow(ctx, "SELECT document FROM reliability_workflows WHERE id=$1 FOR UPDATE", id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	if err != nil {
		return w, err
	}
	if err = json.Unmarshal(b, &w); err != nil {
		return w, err
	}
	if err = fn(&w); err != nil {
		return w, err
	}
	b, err = json.Marshal(w)
	if err != nil {
		return w, err
	}
	if _, err = tx.Exec(ctx, "UPDATE reliability_workflows SET document=$2 WHERE id=$1", id, b); err != nil {
		return w, err
	}
	return w, tx.Commit(ctx)
}
