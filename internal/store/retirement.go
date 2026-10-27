package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// RetirementStore is used only inside a DefinitionBatch for irreversible work.
type RetirementStore interface {
	PurgeSLO(context.Context, string, v1.RetirementReceipt) error
	RetirementReceipt(context.Context, string) (v1.RetirementReceipt, error)
}

func (m *Memory) PurgeSLO(ctx context.Context, id string, receipt v1.RetirementReceipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	r, ok := m.records[id]
	if !ok {
		return model.ErrNotFound
	}
	if !r.SLO.Retired() || r.SLO.Retirement.Cleanup != "complete" || model.ConfigRevision(r.SLO) != receipt.Revision {
		return model.ErrRevisionConflict
	}
	if m.tombstones == nil {
		m.tombstones = map[string]v1.RetirementReceipt{}
	}
	m.tombstones[id] = copyJSON(receipt)
	delete(m.records, id)
	delete(m.archives, id)
	for key, d := range m.deliveries {
		if d.Event.SLOID == id {
			delete(m.deliveries, key)
		}
	}
	return nil
}
func (m *Memory) RetirementReceipt(ctx context.Context, id string) (v1.RetirementReceipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return v1.RetirementReceipt{}, e
	}
	r, ok := m.tombstones[id]
	if !ok {
		return r, model.ErrNotFound
	}
	return copyJSON(r), nil
}
func (p *Postgres) PurgeSLO(ctx context.Context, id string, receipt v1.RetirementReceipt) error {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, e = tx.Exec(ctx, "LOCK TABLE slos IN SHARE ROW EXCLUSIVE MODE"); e != nil {
		return e
	}
	r, e := load(ctx, tx, id, true)
	if e != nil {
		return e
	}
	if !r.SLO.Retired() || r.SLO.Retirement.Cleanup != "complete" || model.ConfigRevision(r.SLO) != receipt.Revision {
		return model.ErrRevisionConflict
	}
	b, e := json.Marshal(receipt)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO slo_tombstones(slo_id,document)VALUES($1,$2)", id, b); e != nil {
		return e
	}
	for _, q := range []string{"DELETE FROM samples WHERE slo_id=$1", "DELETE FROM window_observations WHERE slo_id=$1", "DELETE FROM slo_revision_archives WHERE slo_id=$1", "DELETE FROM alert_outbox WHERE event->>'sloId'=$1", "DELETE FROM slos WHERE id=$1"} {
		if _, e = tx.Exec(ctx, q, id); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) RetirementReceipt(ctx context.Context, id string) (v1.RetirementReceipt, error) {
	var r v1.RetirementReceipt
	var b []byte
	e := p.pool.QueryRow(ctx, "SELECT document FROM slo_tombstones WHERE slo_id=$1", id).Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		e = model.ErrNotFound
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(b, &r)
	return r, e
}

// RetirementReferences includes latest definitions only. Historical documents
// remain immutable evidence; they cannot introduce new active references.
func RetirementReferences(ctx context.Context, st Store, id string) ([]v1.RetirementReference, error) {
	out := []v1.RetirementReference{}
	if js, ok := st.(JourneyStore); ok {
		journeys, e := js.Journeys(ctx)
		if e != nil {
			return nil, e
		}
		if len(journeys) > 1000 {
			return nil, model.Invalid("retirement requires narrowing the tenant catalog to 1000 journeys")
		}
		for _, j := range journeys {
			for _, link := range j.Manifest.Spec.SLOs {
				if link.ID == id {
					out = append(out, v1.RetirementReference{Kind: v1.JourneyKind, Name: j.Manifest.Metadata.Name, Revision: j.Revision, Required: link.Required})
				}
			}
		}
	}
	if ts, ok := st.(TopologyStore); ok {
		cursor := ""
		count := 0
		for {
			rows, e := ts.ListTopology(ctx, v1.DependencyMapKind, cursor, 100)
			if e != nil {
				return nil, e
			}
			for _, r := range rows {
				count++
				if count > 1000 {
					return nil, model.Invalid("retirement requires narrowing the tenant catalog to 1000 dependency maps")
				}
				var d model.ServiceDependencyMap
				if e = json.Unmarshal(r.Manifest, &d); e != nil {
					return nil, e
				}
				found := false
				for _, n := range d.Spec.Services {
					for _, ref := range n.SLORefs {
						if ref == id {
							found = true
						}
					}
				}
				if found {
					out = append(out, v1.RetirementReference{Kind: r.Kind, Name: r.ID, Revision: r.Revision})
				}
				cursor = topologyKey(r.Kind, r.ID)
			}
			if len(rows) < 100 {
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}
