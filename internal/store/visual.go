package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type VisualStore interface {
	ReserveVisual(context.Context, v1.VisualEvaluation) (v1.VisualEvaluation, bool, error)
	ReadVisual(context.Context, string) (v1.VisualEvaluation, error)
	ListVisual(context.Context, string) ([]v1.VisualEvaluation, error)
	UpdateVisual(context.Context, string, func(*v1.VisualEvaluation) error) (v1.VisualEvaluation, error)
}

func (m *Memory) pruneVisual() {
	for id, r := range m.visual {
		if !r.ExpiresAt.After(time.Now()) {
			delete(m.visual, id)
		}
	}
}
func (m *Memory) ReserveVisual(ctx context.Context, r v1.VisualEvaluation) (v1.VisualEvaluation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return r, false, e
	}
	m.pruneVisual()
	if old, ok := m.visual[r.ID]; ok {
		if old.RequestDigest != r.RequestDigest {
			return old, false, model.ErrConflict
		}
		return copyJSON(old), false, nil
	}
	if len(m.visual) >= 500 {
		return r, false, model.Invalid("visual receipt capacity reached; receipts expire after 30 days")
	}
	if m.visual == nil {
		m.visual = map[string]v1.VisualEvaluation{}
	}
	m.visual[r.ID] = copyJSON(r)
	return copyJSON(r), true, nil
}
func (m *Memory) ReadVisual(ctx context.Context, id string) (v1.VisualEvaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return v1.VisualEvaluation{}, e
	}
	m.pruneVisual()
	r, ok := m.visual[id]
	if !ok {
		return r, model.ErrNotFound
	}
	return copyJSON(r), nil
}
func (m *Memory) ListVisual(ctx context.Context, journey string) ([]v1.VisualEvaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	m.pruneVisual()
	out := []v1.VisualEvaluation{}
	for _, r := range m.visual {
		if r.JourneyID == journey {
			out = append(out, copyJSON(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}
func (m *Memory) UpdateVisual(ctx context.Context, id string, fn func(*v1.VisualEvaluation) error) (v1.VisualEvaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return v1.VisualEvaluation{}, e
	}
	m.pruneVisual()
	r, ok := m.visual[id]
	if !ok {
		return r, model.ErrNotFound
	}
	r = copyJSON(r)
	if e := fn(&r); e != nil {
		return r, e
	}
	m.visual[id] = copyJSON(r)
	return r, nil
}
func visualScan(row pgx.Row) (v1.VisualEvaluation, error) {
	var r v1.VisualEvaluation
	var b []byte
	e := row.Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func (p *Postgres) ReserveVisual(ctx context.Context, r v1.VisualEvaluation) (v1.VisualEvaluation, bool, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return r, false, e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(76449229)`); e != nil {
		return r, false, e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM visual_evaluations WHERE expires_at<=clock_timestamp()`); e != nil {
		return r, false, e
	}
	old, e := visualScan(tx.QueryRow(ctx, `SELECT document FROM visual_evaluations WHERE id=$1`, r.ID))
	if e == nil {
		if old.RequestDigest != r.RequestDigest {
			return old, false, model.ErrConflict
		}
		return old, false, tx.Commit(ctx)
	}
	if !errors.Is(e, model.ErrNotFound) {
		return r, false, e
	}
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM visual_evaluations`).Scan(&n); e != nil {
		return r, false, e
	}
	if n >= 500 {
		return r, false, model.Invalid("visual receipt capacity reached; receipts expire after 30 days")
	}
	b, e := json.Marshal(r)
	if e != nil {
		return r, false, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO visual_evaluations(id,journey_id,expires_at,created_at,document) VALUES($1,$2,$3,$4,$5)`, r.ID, r.JourneyID, r.ExpiresAt, r.CreatedAt, b)
	if e != nil {
		return r, false, e
	}
	return r, true, tx.Commit(ctx)
}
func (p *Postgres) ReadVisual(ctx context.Context, id string) (v1.VisualEvaluation, error) {
	return visualScan(p.pool.QueryRow(ctx, `SELECT document FROM visual_evaluations WHERE id=$1 AND expires_at>clock_timestamp()`, id))
}
func (p *Postgres) ListVisual(ctx context.Context, journey string) ([]v1.VisualEvaluation, error) {
	// Pruning removes expired metadata as well as excluding it from reads.
	if _, e := p.pool.Exec(ctx, `DELETE FROM visual_evaluations WHERE expires_at<=clock_timestamp()`); e != nil {
		return nil, e
	}
	rows, e := p.pool.Query(ctx, `SELECT document FROM visual_evaluations WHERE journey_id=$1 AND expires_at>clock_timestamp() ORDER BY created_at DESC,id LIMIT 100`, journey)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []v1.VisualEvaluation{}
	for rows.Next() {
		r, e := visualScan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (p *Postgres) UpdateVisual(ctx context.Context, id string, fn func(*v1.VisualEvaluation) error) (v1.VisualEvaluation, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return v1.VisualEvaluation{}, e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	r, e := visualScan(tx.QueryRow(ctx, `SELECT document FROM visual_evaluations WHERE id=$1 AND expires_at>clock_timestamp() FOR UPDATE`, id))
	if e != nil {
		return r, e
	}
	if e = fn(&r); e != nil {
		return r, e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return r, e
	}
	if _, e = tx.Exec(ctx, `UPDATE visual_evaluations SET document=$2 WHERE id=$1`, id, b); e != nil {
		return r, e
	}
	return r, tx.Commit(ctx)
}

func (m *Memory) PruneVisual(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneVisual()
	return ctx.Err()
}
func (p *Postgres) PruneVisual(ctx context.Context) error {
	_, e := p.pool.Exec(ctx, `DELETE FROM visual_evaluations WHERE expires_at<=clock_timestamp()`)
	return e
}
