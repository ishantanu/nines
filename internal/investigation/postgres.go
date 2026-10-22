package investigation

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
	pool, err := observability.NewPool(ctx, dsn)
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
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(654239121)"); err == nil {
		_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS reliability_investigations(id text PRIMARY KEY, summary jsonb NOT NULL, document bytea NOT NULL);`+runTableDDL)
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
func (p *postgres) Create(ctx context.Context, v v1.Investigation) (v1.Investigation, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	summary, err := json.Marshal(v.InvestigationSummary)
	if err != nil {
		return v, err
	}
	tag, err := p.pool.Exec(ctx, "INSERT INTO reliability_investigations(id,summary,document) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", v.ID, summary, b)
	if err != nil {
		return v, err
	}
	if tag.RowsAffected() == 1 {
		return clone(v), nil
	}
	old, err := p.Read(ctx, v.ID)
	if err != nil {
		return v, err
	}
	if !sameCreate(old, v) {
		return v, ErrConflict
	}
	return old, nil
}
func (p *postgres) Read(ctx context.Context, id string) (v1.Investigation, error) {
	var v v1.Investigation
	var b []byte
	err := p.pool.QueryRow(ctx, "SELECT document FROM reliability_investigations WHERE id=$1", id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(b, &v)
	return v, err
}
func (p *postgres) List(ctx context.Context, after string, limit int) ([]v1.InvestigationSummary, error) {
	rows, err := p.pool.Query(ctx, "SELECT summary FROM reliability_investigations WHERE id>$1 ORDER BY id LIMIT $2", after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []v1.InvestigationSummary{}
	for rows.Next() {
		var b []byte
		var v v1.InvestigationSummary
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (p *postgres) Update(ctx context.Context, id string, fn func(*v1.Investigation) error) (v1.Investigation, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return v1.Investigation{}, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	var v v1.Investigation
	err = tx.QueryRow(ctx, "SELECT document FROM reliability_investigations WHERE id=$1 FOR UPDATE", id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(b, &v); err != nil {
		return v, err
	}
	if err = fn(&v); err != nil {
		return v, err
	}
	b, err = json.Marshal(v)
	if err != nil {
		return v, err
	}
	summary, err := json.Marshal(v.InvestigationSummary)
	if err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, "UPDATE reliability_investigations SET document=$2,summary=$3 WHERE id=$1", id, b, summary); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
