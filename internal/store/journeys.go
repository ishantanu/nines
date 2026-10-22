package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/ishantanu/nines/internal/model"
)

// JourneyStore uses compare-and-swap revisions. expected=0 creates a resource;
// updates require the exact current revision. Run evidence is immutable by ID.
type JourneyStore interface {
	ApplyJourney(context.Context, model.JourneyRevision, int64) (model.JourneyRevision, error)
	Journeys(context.Context) ([]model.JourneyRevision, error)
	JourneyHistory(context.Context, string) ([]model.JourneyRevision, error)
	JourneyRevision(context.Context, string, int64) (model.JourneyRevision, error)
	PutRun(context.Context, model.JourneyRun) (model.JourneyRun, error)
	Runs(context.Context, string) ([]model.JourneyRun, error)
}

func copyJSON[T any](v T) T {
	b, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(b, &out)
	return out
}
func equalEvidence(a, b model.JourneyRun) bool {
	x, _ := json.Marshal(a.Evidence)
	y, _ := json.Marshal(b.Evidence)
	return string(x) == string(y)
}
func (m *Memory) ApplyJourney(ctx context.Context, next model.JourneyRevision, expected int64) (model.JourneyRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return next, err
	}
	if m.journeys == nil {
		m.journeys = map[string][]model.JourneyRevision{}
	}
	history := m.journeys[next.Manifest.Metadata.Name]
	var current model.JourneyRevision
	if len(history) > 0 {
		current = history[len(history)-1]
	}
	if current.Revision != expected {
		return current, model.ErrRevisionConflict
	}
	if current.Digest == next.Digest {
		return copyJSON(current), nil
	}
	next.Revision = expected + 1
	m.journeys[next.Manifest.Metadata.Name] = append(history, copyJSON(next))
	return copyJSON(next), nil
}
func (m *Memory) Journeys(ctx context.Context) ([]model.JourneyRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []model.JourneyRevision{}
	for _, h := range m.journeys {
		out = append(out, copyJSON(h[len(h)-1]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.Metadata.Name < out[j].Manifest.Metadata.Name })
	return out, nil
}
func (m *Memory) JourneyHistory(ctx context.Context, id string) ([]model.JourneyRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h := m.journeys[id]
	if len(h) == 0 {
		return nil, model.ErrJourneyNotFound
	}
	return copyJSON(h), nil
}
func (m *Memory) JourneyRevision(ctx context.Context, id string, rev int64) (model.JourneyRevision, error) {
	h, err := m.JourneyHistory(ctx, id)
	if err != nil {
		return model.JourneyRevision{}, err
	}
	if rev == 0 {
		return h[len(h)-1], nil
	}
	if rev < 1 || rev > int64(len(h)) {
		return model.JourneyRevision{}, model.ErrJourneyNotFound
	}
	return h[rev-1], nil
}
func (m *Memory) PutRun(ctx context.Context, r model.JourneyRun) (model.JourneyRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return r, err
	}
	h := m.journeys[r.JourneyID]
	rev := r.Evidence.JourneyRevision
	if rev < 1 || rev > int64(len(h)) {
		return r, model.ErrJourneyNotFound
	}
	if m.runs == nil {
		m.runs = map[string]map[string]model.JourneyRun{}
	}
	if m.runs[r.JourneyID] == nil {
		m.runs[r.JourneyID] = map[string]model.JourneyRun{}
	}
	if old, ok := m.runs[r.JourneyID][r.Evidence.ID]; ok {
		if !equalEvidence(old, r) {
			return r, model.ErrRunConflict
		}
		return copyJSON(old), nil
	}
	m.runs[r.JourneyID][r.Evidence.ID] = copyJSON(r)
	return copyJSON(r), nil
}
func (m *Memory) Runs(ctx context.Context, id string) ([]model.JourneyRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(m.journeys[id]) == 0 {
		return nil, model.ErrJourneyNotFound
	}
	out := []model.JourneyRun{}
	for _, r := range m.runs[id] {
		out = append(out, copyJSON(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceivedAt.Equal(out[j].ReceivedAt) {
			return out[i].Evidence.ID < out[j].Evidence.ID
		}
		return out[i].ReceivedAt.After(out[j].ReceivedAt)
	})
	if len(out) > 200 {
		out = out[:200]
	}
	return out, nil
}
func (p *Postgres) ApplyJourney(ctx context.Context, next model.JourneyRevision, expected int64) (model.JourneyRevision, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return next, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	// A name-scoped advisory lock also serializes two concurrent initial creates.
	id := next.Manifest.Metadata.Name
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':' || $1,654239119))", id); err != nil {
		return next, err
	}
	var b []byte
	current := model.JourneyRevision{}
	err = tx.QueryRow(ctx, "SELECT document FROM journey_revisions WHERE journey_id=$1 ORDER BY revision DESC LIMIT 1", id).Scan(&b)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return next, err
	}
	if err == nil {
		if err = json.Unmarshal(b, &current); err != nil {
			return next, err
		}
	}
	if current.Revision != expected {
		return current, model.ErrRevisionConflict
	}
	if current.Digest == next.Digest {
		return current, tx.Commit(ctx)
	}
	next.Revision = expected + 1
	b, err = json.Marshal(next)
	if err != nil {
		return next, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO journey_revisions(journey_id,revision,document) VALUES($1,$2,$3)", id, next.Revision, b)
	if err != nil {
		return next, err
	}
	return next, tx.Commit(ctx)
}
func (p *Postgres) Journeys(ctx context.Context) ([]model.JourneyRevision, error) {
	rows, err := p.pool.Query(ctx, "SELECT DISTINCT ON (journey_id) document FROM journey_revisions ORDER BY journey_id,revision DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.JourneyRevision{}
	for rows.Next() {
		var b []byte
		var r model.JourneyRevision
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
func (p *Postgres) JourneyHistory(ctx context.Context, id string) ([]model.JourneyRevision, error) {
	rows, err := p.pool.Query(ctx, "SELECT document FROM journey_revisions WHERE journey_id=$1 ORDER BY revision", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.JourneyRevision{}
	for rows.Next() {
		var b []byte
		var r model.JourneyRevision
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, model.ErrJourneyNotFound
	}
	return out, nil
}
func (p *Postgres) JourneyRevision(ctx context.Context, id string, rev int64) (model.JourneyRevision, error) {
	query := "SELECT document FROM journey_revisions WHERE journey_id=$1 AND revision=$2"
	args := []any{id, rev}
	if rev == 0 {
		query = "SELECT document FROM journey_revisions WHERE journey_id=$1 ORDER BY revision DESC LIMIT 1"
		args = []any{id}
	}
	var b []byte
	var r model.JourneyRevision
	err := p.pool.QueryRow(ctx, query, args...).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, model.ErrJourneyNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}
func (p *Postgres) PutRun(ctx context.Context, r model.JourneyRun) (model.JourneyRun, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	tag, err := p.pool.Exec(ctx, "INSERT INTO journey_runs(journey_id,run_id,revision,received_at,document) VALUES($1,$2,$3,$4,$5) ON CONFLICT(journey_id,run_id) DO NOTHING", r.JourneyID, r.Evidence.ID, r.Evidence.JourneyRevision, r.ReceivedAt, b)
	if err != nil {
		return r, err
	}
	if tag.RowsAffected() == 1 {
		return r, nil
	}
	var old model.JourneyRun
	if err = p.pool.QueryRow(ctx, "SELECT document FROM journey_runs WHERE journey_id=$1 AND run_id=$2", r.JourneyID, r.Evidence.ID).Scan(&b); err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &old); err != nil {
		return r, err
	}
	if !equalEvidence(old, r) {
		return old, model.ErrRunConflict
	}
	return old, nil
}
func (p *Postgres) Runs(ctx context.Context, id string) ([]model.JourneyRun, error) {
	if _, err := p.JourneyRevision(ctx, id, 0); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, "SELECT document FROM journey_runs WHERE journey_id=$1 ORDER BY received_at DESC,run_id LIMIT 200", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.JourneyRun{}
	for rows.Next() {
		var b []byte
		var r model.JourneyRun
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
