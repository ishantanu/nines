package store

import (
	"context"
	"slices"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// CatalogStore provides bounded metadata reads without loading measurements,
// manifests or whole inventory responses. Search scans are deadline-bound by API.
type CatalogStore interface {
	SearchResources(context.Context, CatalogQuery) ([]v1.ResourceSummary, error)
}
type CatalogQuery struct {
	Kind, Project, Query, AfterKind, AfterID, LinkedSLO string
	Limit                                               int
}

func (q CatalogQuery) Validate() error {
	if q.Kind != "" && q.Kind != v1.SLOKind && q.Kind != v1.JourneyKind || q.Limit < 1 || q.Limit > 101 || len(q.Project) > 200 || len(q.Query) > 200 {
		return model.Invalid("invalid resource search")
	}
	if q.AfterKind != "" && q.AfterKind != v1.SLOKind && q.AfterKind != v1.JourneyKind {
		return model.Invalid("invalid search position")
	}
	return nil
}
func SLOSummary(s model.SLO) v1.ResourceSummary {
	return v1.ResourceSummary{Kind: v1.SLOKind, ID: s.ID, Project: s.Project, DisplayName: s.Name, Service: s.Service, Revision: model.ConfigRevision(s)}
}
func JourneySummary(j model.JourneyRevision) v1.ResourceSummary {
	return v1.ResourceSummary{Kind: v1.JourneyKind, ID: j.Manifest.Metadata.Name, Project: j.Manifest.Metadata.Project, DisplayName: j.Manifest.Spec.DisplayName, Revision: j.Revision}
}
func compareResource(a, b v1.ResourceSummary) int {
	if a.Kind != b.Kind {
		return strings.Compare(a.Kind, b.Kind)
	}
	return strings.Compare(a.ID, b.ID)
}
func (m *Memory) SearchResources(ctx context.Context, q CatalogQuery) ([]v1.ResourceSummary, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []v1.ResourceSummary{}
	add := func(x v1.ResourceSummary) {
		if q.Kind != "" && q.Kind != x.Kind || q.Project != "" && q.Project != x.Project || compareResource(x, v1.ResourceSummary{Kind: q.AfterKind, ID: q.AfterID}) <= 0 {
			return
		}
		if q.Query != "" && !strings.Contains(strings.ToLower(x.ID), q.Query) && !strings.Contains(strings.ToLower(x.DisplayName), q.Query) {
			return
		}
		i, _ := slices.BinarySearchFunc(out, x, compareResource)
		out = slices.Insert(out, i, x)
		if len(out) > q.Limit {
			out = out[:q.Limit]
		}
	}
	for _, r := range m.records {
		if q.LinkedSLO != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		add(SLOSummary(r.SLO))
	}
	for _, h := range m.journeys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(h) > 0 {
			latest := h[len(h)-1]
			if q.LinkedSLO != "" {
				matched := false
				for _, link := range latest.Manifest.Spec.SLOs {
					if link.ID == q.LinkedSLO {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			add(JourneySummary(latest))
		}
	}
	return out, ctx.Err()
}
func (p *Postgres) SearchResources(ctx context.Context, q CatalogQuery) ([]v1.ResourceSummary, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	// Project only compact fields. The PK supports keyset and latest-revision
	// anti-joins; literal substring search intentionally has no wildcard syntax.
	rows, err := p.pool.Query(ctx, `WITH resources AS (
 SELECT 'ServiceLevelObjective'::text AS kind,id,config->>'project' AS project,config->>'name' AS name,config->>'service' AS service,COALESCE((config->>'revision')::bigint,1) AS revision FROM slos
 WHERE $7='' AND $1 IN ('','ServiceLevelObjective') AND ('ServiceLevelObjective' COLLATE "C",id COLLATE "C")>($4::text COLLATE "C",$5::text COLLATE "C")
 UNION ALL
 SELECT 'CriticalUserJourney',j.journey_id,j.document->'manifest'->'metadata'->>'project',j.document->'manifest'->'spec'->>'displayName','',j.revision FROM journey_revisions j
 WHERE $1 IN ('','CriticalUserJourney') AND ('CriticalUserJourney' COLLATE "C",j.journey_id COLLATE "C")>($4::text COLLATE "C",$5::text COLLATE "C")
 AND ($7='' OR EXISTS (SELECT 1 FROM jsonb_array_elements(j.document->'manifest'->'spec'->'slos') link WHERE link->>'id'=$7))
 AND NOT EXISTS (SELECT 1 FROM journey_revisions newer WHERE newer.journey_id=j.journey_id AND newer.revision>j.revision)
 ) SELECT kind,id,project,name,service,GREATEST(revision,1) FROM resources WHERE ($2='' OR project=$2) AND ($3='' OR strpos(lower(id),$3)>0 OR strpos(lower(name),$3)>0) ORDER BY kind COLLATE "C",id COLLATE "C" LIMIT $6`, q.Kind, q.Project, q.Query, q.AfterKind, q.AfterID, q.Limit, q.LinkedSLO)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []v1.ResourceSummary{}
	for rows.Next() {
		var x v1.ResourceSummary
		if err = rows.Scan(&x.Kind, &x.ID, &x.Project, &x.DisplayName, &x.Service, &x.Revision); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
