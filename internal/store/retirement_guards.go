package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func topologySLOs(r model.TopologyRevision) ([]string, error) {
	if r.Kind != v1.DependencyMapKind {
		return nil, nil
	}
	var d model.ServiceDependencyMap
	if e := json.Unmarshal(r.Manifest, &d); e != nil {
		return nil, e
	}
	var ids []string
	for _, n := range d.Spec.Services {
		ids = append(ids, n.SLORefs...)
	}
	return ids, nil
}
func (m *Memory) checkRetiredReferences(ids []string) error {
	for _, id := range ids {
		if r, ok := m.records[id]; ok && r.SLO.Retired() {
			return model.Invalid("linked SLO %s is archived", id)
		}
		if _, ok := m.tombstones[id]; ok {
			return model.Invalid("linked SLO %s was permanently deleted", id)
		}
	}
	return nil
}
func checkPostgresRetiredReferences(ctx context.Context, tx pgx.Tx, ids []string) error {
	// Serialize validation with retirement's table fence, including a writer that
	// validated its definition before the archive committed on another replica.
	if _, e := tx.Exec(ctx, "SET LOCAL lock_timeout='2s'; LOCK TABLE slos IN SHARE ROW EXCLUSIVE MODE"); e != nil {
		return e
	}
	for _, id := range ids {
		var blocked bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM slos WHERE id=$1 AND config->'retirement' IS NOT NULL AND config->'retirement'<>'null'::jsonb) OR EXISTS(SELECT 1 FROM slo_tombstones WHERE slo_id=$1)", id).Scan(&blocked); e != nil {
			return e
		}
		if blocked {
			return model.Invalid("linked SLO %s is archived or deleted", id)
		}
	}
	return nil
}
