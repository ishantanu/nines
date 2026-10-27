package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/ishantanu/nines/internal/model"
)

// TopologyStore stores immutable versions separately from SLO measurement data.
// Lists are bounded keyset pages; tenant isolation uses the existing store/schema boundary.
type TopologyStore interface {
	ApplyTopology(context.Context, model.TopologyRevision, int64) (model.TopologyRevision, error)
	ReadTopology(context.Context, string, string, int64) (model.TopologyRevision, error)
	ListTopology(context.Context, string, string, int) ([]model.TopologyRevision, error)
}

func topologyKey(kind, id string) string { return kind + "/" + id }
func (m *Memory) ApplyTopology(ctx context.Context, next model.TopologyRevision, expected int64) (model.TopologyRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return next, err
	}
	ids, e := topologySLOs(next)
	if e != nil {
		return next, e
	}
	if e = m.checkRetiredReferences(ids); e != nil {
		return next, e
	}
	if m.topology == nil {
		m.topology = map[string][]model.TopologyRevision{}
	}
	key := topologyKey(next.Kind, next.ID)
	h := m.topology[key]
	var current model.TopologyRevision
	if len(h) > 0 {
		current = h[len(h)-1]
	}
	if current.Revision != expected {
		return current, model.ErrRevisionConflict
	}
	if current.Digest == next.Digest {
		return copyJSON(current), nil
	}
	next.Revision = expected + 1
	m.topology[key] = append(h, copyJSON(next))
	return copyJSON(next), nil
}
func (m *Memory) ReadTopology(ctx context.Context, kind, id string, rev int64) (model.TopologyRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return model.TopologyRevision{}, err
	}
	h := m.topology[topologyKey(kind, id)]
	if len(h) == 0 || rev < 0 || rev > int64(len(h)) {
		return model.TopologyRevision{}, model.ErrNotFound
	}
	if rev == 0 {
		rev = int64(len(h))
	}
	return copyJSON(h[rev-1]), nil
}
func (m *Memory) ListTopology(ctx context.Context, kind, after string, limit int) ([]model.TopologyRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 101 {
		return nil, model.Invalid("invalid page limit")
	}
	out := []model.TopologyRevision{}
	for _, h := range m.topology {
		r := h[len(h)-1]
		key := topologyKey(r.Kind, r.ID)
		if (kind == "" || r.Kind == kind) && key > after {
			i := sort.Search(len(out), func(i int) bool { return topologyKey(out[i].Kind, out[i].ID) > key })
			out = append(out, model.TopologyRevision{})
			copy(out[i+1:], out[i:])
			out[i] = r
			if len(out) > limit {
				out = out[:limit]
			}
		}
	}
	return copyJSON(out), nil
}
func (p *Postgres) ApplyTopology(ctx context.Context, next model.TopologyRevision, expected int64) (model.TopologyRevision, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return next, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	ids, e := topologySLOs(next)
	if e != nil {
		return next, e
	}
	if e = checkPostgresRetiredReferences(ctx, tx, ids); e != nil {
		return next, e
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':' || $1,654239120))", topologyKey(next.Kind, next.ID)); err != nil {
		return next, err
	}
	var b []byte
	var current model.TopologyRevision
	err = tx.QueryRow(ctx, "SELECT document FROM topology_revisions WHERE kind=$1 AND resource_id=$2 ORDER BY revision DESC LIMIT 1", next.Kind, next.ID).Scan(&b)
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
	_, err = tx.Exec(ctx, "INSERT INTO topology_revisions(kind,resource_id,revision,document) VALUES($1,$2,$3,$4)", next.Kind, next.ID, next.Revision, b)
	if err != nil {
		return next, err
	}
	return next, tx.Commit(ctx)
}
func (p *Postgres) ReadTopology(ctx context.Context, kind, id string, rev int64) (model.TopologyRevision, error) {
	q := "SELECT document FROM topology_revisions WHERE kind=$1 AND resource_id=$2 AND revision=$3"
	args := []any{kind, id, rev}
	if rev == 0 {
		q = "SELECT document FROM topology_revisions WHERE kind=$1 AND resource_id=$2 ORDER BY revision DESC LIMIT 1"
		args = []any{kind, id}
	}
	var b []byte
	var r model.TopologyRevision
	err := p.pool.QueryRow(ctx, q, args...).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}
func (p *Postgres) ListTopology(ctx context.Context, kind, after string, limit int) ([]model.TopologyRevision, error) {
	if limit < 1 || limit > 101 {
		return nil, model.Invalid("invalid page limit")
	}
	rows, err := p.pool.Query(ctx, `SELECT document FROM (SELECT DISTINCT ON(kind,resource_id) kind,resource_id,document FROM topology_revisions ORDER BY kind,resource_id,revision DESC) r WHERE ($1='' OR kind=$1) AND (kind || '/' || resource_id) COLLATE "C">$2 COLLATE "C" ORDER BY (kind || '/' || resource_id) COLLATE "C" LIMIT $3`, kind, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TopologyRevision{}
	for rows.Next() {
		var b []byte
		var r model.TopologyRevision
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
