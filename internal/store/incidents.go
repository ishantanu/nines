package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/ishantanu/nines/internal/model"
)

// IncidentStore holds incident records. UpdateIncidents runs one serialized
// transaction per tenant: change receives every open incident and returns
// the incidents to write, so concurrent alerts join one incident instead of
// racing to open two. A written incident's revision must be one more than
// the stored one (or 1 for a new incident).
type IncidentStore interface {
	UpdateIncidents(ctx context.Context, change func(open []model.Incident) ([]model.Incident, error)) error
	Incident(ctx context.Context, id string) (model.Incident, error)
	Incidents(ctx context.Context, limit int) ([]model.Incident, error)
	// UpdateIncident changes one incident, open or resolved, at its current
	// revision; change must increment Revision.
	UpdateIncident(ctx context.Context, id string, change func(*model.Incident) error) error
}

func (m *Memory) UpdateIncident(ctx context.Context, id string, change func(*model.Incident) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.incidents[id]
	if !ok {
		return model.ErrNotFound
	}
	next := copyJSON(current)
	if err := change(&next); err != nil {
		return err
	}
	if next.Revision != current.Revision+1 || next.ID != id {
		return model.ErrRevisionConflict
	}
	m.incidents[id] = copyJSON(next)
	return ctx.Err()
}

func (p *Postgres) UpdateIncident(ctx context.Context, id string, change func(*model.Incident) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	if err = tx.QueryRow(ctx, "SELECT document FROM incidents WHERE incident_id=$1 FOR UPDATE", id).Scan(&b); errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	} else if err != nil {
		return err
	}
	var current model.Incident
	if err = json.Unmarshal(b, &current); err != nil {
		return err
	}
	next := copyJSON(current)
	if err = change(&next); err != nil {
		return err
	}
	if next.Revision != current.Revision+1 || next.ID != id {
		return model.ErrRevisionConflict
	}
	if b, err = json.Marshal(next); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "UPDATE incidents SET revision=$2, state=$3, document=$4 WHERE incident_id=$1 AND revision=$5", id, next.Revision, next.State, b, current.Revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return model.ErrRevisionConflict
	}
	return tx.Commit(ctx)
}

func (m *Memory) UpdateIncidents(ctx context.Context, change func([]model.Incident) ([]model.Incident, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.incidents == nil {
		m.incidents = map[string]model.Incident{}
	}
	open := []model.Incident{}
	for _, i := range m.incidents {
		if i.Open() {
			open = append(open, copyJSON(i))
		}
	}
	sortIncidents(open)
	writes, err := change(open)
	if err != nil {
		return err
	}
	for _, next := range writes {
		if next.Revision != m.incidents[next.ID].Revision+1 {
			return model.ErrRevisionConflict
		}
	}
	for _, next := range writes {
		m.incidents[next.ID] = copyJSON(next)
	}
	return nil
}

func (m *Memory) Incident(ctx context.Context, id string) (model.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.incidents[id]
	if !ok {
		return i, model.ErrNotFound
	}
	return copyJSON(i), ctx.Err()
}

func (m *Memory) Incidents(ctx context.Context, limit int) ([]model.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []model.Incident{}
	for _, i := range m.incidents {
		out = append(out, copyJSON(i))
	}
	sortIncidents(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, ctx.Err()
}

// sortIncidents orders newest first, then by ID for a stable order.
func sortIncidents(list []model.Incident) {
	sort.Slice(list, func(a, b int) bool {
		if !list[a].OpenedAt.Equal(list[b].OpenedAt) {
			return list[a].OpenedAt.After(list[b].OpenedAt)
		}
		return list[a].ID < list[b].ID
	})
}

func (p *Postgres) UpdateIncidents(ctx context.Context, change func([]model.Incident) ([]model.Incident, error)) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':incidents',654239120))"); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT document FROM incidents WHERE state <> 'resolved' ORDER BY opened_at DESC, incident_id")
	if err != nil {
		return err
	}
	open, err := scanIncidents(rows)
	if err != nil {
		return err
	}
	writes, err := change(open)
	if err != nil {
		return err
	}
	for _, next := range writes {
		b, err := json.Marshal(next)
		if err != nil {
			return err
		}
		var tag interface{ RowsAffected() int64 }
		if next.Revision == 1 {
			tag, err = tx.Exec(ctx, "INSERT INTO incidents (incident_id, revision, state, opened_at, document) VALUES ($1,1,$2,$3,$4) ON CONFLICT DO NOTHING", next.ID, next.State, next.OpenedAt, b)
		} else {
			tag, err = tx.Exec(ctx, "UPDATE incidents SET revision=$2, state=$3, document=$4 WHERE incident_id=$1 AND revision=$5", next.ID, next.Revision, next.State, b, next.Revision-1)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return model.ErrRevisionConflict
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Incident(ctx context.Context, id string) (model.Incident, error) {
	var b []byte
	var i model.Incident
	err := p.pool.QueryRow(ctx, "SELECT document FROM incidents WHERE incident_id=$1", id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, model.ErrNotFound
	}
	if err != nil {
		return i, err
	}
	return i, json.Unmarshal(b, &i)
}

func (p *Postgres) Incidents(ctx context.Context, limit int) ([]model.Incident, error) {
	rows, err := p.pool.Query(ctx, "SELECT document FROM incidents ORDER BY opened_at DESC, incident_id LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	return scanIncidents(rows)
}

func scanIncidents(rows pgx.Rows) ([]model.Incident, error) {
	defer rows.Close()
	out := []model.Incident{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var i model.Incident
		if err := json.Unmarshal(b, &i); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
