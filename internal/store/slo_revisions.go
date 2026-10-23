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
	"github.com/ishantanu/nines/internal/tenancy"
)

// SLOArchive freezes only bounded, already retained observations. It contains
// no provider credentials or external network state.
type SLOArchive struct {
	Summary v1.SLORevision `json:"summary"`
	Record  model.Record   `json:"record"`
}
type SLORevisionStore interface {
	SLORevisions(context.Context, string, int64, int) ([]v1.SLORevision, error)
	SLORevision(context.Context, string, int64) (SLOArchive, error)
}

func archiveSLO(ctx context.Context, r model.Record) SLOArchive {
	frozen := clone(r)
	frozen.OperatorRoute = ""
	principal, _ := tenancy.FromContext(ctx)
	manifest := model.DefinitionFromSLO(frozen.SLO)
	return SLOArchive{Summary: v1.SLORevision{
		Revision: model.ConfigRevision(frozen.SLO), Digest: model.DefinitionDigest(manifest), Manifest: v1.ServiceLevelObjective(manifest),
		ArchivedAt: time.Now().UTC(), ArchivedBy: v1.InvestigationActor{Subject: principal.Subject, Name: principal.Name, Kind: principal.Kind},
		IntervalSamples: len(frozen.Points), WindowObservations: len(frozen.Windows),
	}, Record: frozen}
}
func (m *Memory) SLORevisions(ctx context.Context, id string, before int64, limit int) ([]v1.SLORevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := m.records[id]; !ok {
		return nil, model.ErrNotFound
	}
	out := []v1.SLORevision{}
	for revision, archive := range m.archives[id] {
		if before == 0 || revision < before {
			out = append(out, copyJSON(archive.Summary))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out[:min(len(out), limit)], nil
}
func (m *Memory) SLORevision(ctx context.Context, id string, revision int64) (SLOArchive, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return SLOArchive{}, err
	}
	archive, ok := m.archives[id][revision]
	if !ok {
		return SLOArchive{}, model.ErrNotFound
	}
	return copyJSON(archive), nil
}
func (p *Postgres) SLORevisions(ctx context.Context, id string, before int64, limit int) ([]v1.SLORevision, error) {
	rows, err := p.pool.Query(ctx, "SELECT summary FROM slo_revision_archives WHERE slo_id=$1 AND ($2::bigint=0 OR revision<$2) ORDER BY revision DESC LIMIT $3", id, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []v1.SLORevision{}
	for rows.Next() {
		var body []byte
		var summary v1.SLORevision
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(body, &summary); err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}
func (p *Postgres) SLORevision(ctx context.Context, id string, revision int64) (SLOArchive, error) {
	var body []byte
	err := p.pool.QueryRow(ctx, "SELECT document FROM slo_revision_archives WHERE slo_id=$1 AND revision=$2", id, revision).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return SLOArchive{}, model.ErrNotFound
	}
	if err != nil {
		return SLOArchive{}, err
	}
	var archive SLOArchive
	err = json.Unmarshal(body, &archive)
	return archive, err
}
