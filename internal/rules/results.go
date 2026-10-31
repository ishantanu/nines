package rules

import (
	"context"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/newrelic"
)

// NativeResults only reads the exact, content-verified current native revision.
// It never falls back to an older native revision or Nines raw measurements.
func (m *Manager) NativeResults(ctx context.Context, s model.SLO, from, to time.Time) (v1.NativeSLOResults, error) {
	p, err := m.PlanContext(ctx, s)
	if err != nil {
		return v1.NativeSLOResults{}, err
	}
	var out v1.NativeSLOResults
	switch {
	case p.NativeDatadog != nil:
		if !p.NativeDatadog.Matches {
			return out, model.Invalid("publish the current native SLO revision before reading results")
		}
		c, e := datadogClient(p.Target)
		if e != nil {
			return out, e
		}
		out, err = c.NativeResults(ctx, p.NativeDatadog.NativeID, s.Target, from, to)
	case p.NativeNewRelic != nil:
		n := p.NativeNewRelic
		if !n.Matches || n.ResultQueries == nil {
			return out, model.Invalid("publish the current native SLO revision before reading results")
		}
		c, e := newrelicClient(p.Target)
		if e != nil {
			return out, e
		}
		out, err = c.NativeResults(ctx, newrelic.NativeResource{ID: n.NativeID, GUID: n.NativeGUID, ResultQueries: *n.ResultQueries}, s.Target, from, to)
	default:
		return out, model.Invalid("native results currently support Datadog metric SLOs and New Relic count SLIs")
	}
	if err != nil {
		return v1.NativeSLOResults{}, err
	}
	current, ok := m.TargetFor(s)
	if !ok || nativeReadRouteDigest(current) != nativeReadRouteDigest(p.Target) {
		return v1.NativeSLOResults{}, model.ErrRevisionConflict
	}
	out.APIVersion = v1.APIVersion
	out.Kind = "NativeSLOResults"
	out.Tenant = m.Tenant
	out.SLOID = s.ID
	out.Revision = model.ConfigRevision(s)
	out.Source = ConnectionID(s)
	out.CheckedAt = time.Now().UTC()
	return out, nil
}

func (m *Manager) NativeCatalog(ctx context.Context, source string) (v1.NativeSLOCatalog, error) {
	out := v1.NativeSLOCatalog{APIVersion: v1.APIVersion, Kind: "NativeSLOCatalog", Tenant: m.Tenant, Source: source, Items: []v1.NativeSLOSummary{}}
	t, ok := m.target(source)
	if !ok {
		return out, model.ErrNotFound
	}
	out.Provider = t.Kind
	switch t.Kind {
	case "datadog":
		c, e := datadogClient(t)
		if e != nil {
			return out, e
		}
		rows, e := c.NativeCatalog(ctx)
		if e != nil {
			return out, e
		}
		for _, row := range rows {
			_, e = c.NativeDraft(row, source, "import-preview", "preview", "preview")
			item := v1.NativeSLOSummary{ID: row.ID, Name: row.Name, URL: c.NativeURL(row.ID), Importable: e == nil}
			if e != nil {
				item.Reason = "Outside the supported metric/count import subset"
			}
			out.Items = append(out.Items, item)
		}
	case "newrelic":
		c, e := newrelicClient(t)
		if e != nil {
			return out, e
		}
		rows, e := c.NativeCatalog(ctx, t.NewRelicEntityGUID)
		if e != nil {
			return out, e
		}
		for _, row := range rows {
			_, e = c.NativeDraft(row, source, "import-preview", "preview", "preview")
			item := v1.NativeSLOSummary{ID: row.ID, Name: row.Name, URL: c.NativeURL(row.GUID), Importable: e == nil}
			if e != nil {
				item.Reason = "Outside the supported same-event COUNT import subset"
			}
			out.Items = append(out.Items, item)
		}
	default:
		return out, model.Invalid("native catalog supports configured Datadog or New Relic native destinations")
	}
	current, ok := m.target(source)
	if !ok || nativeReadRouteDigest(current) != nativeReadRouteDigest(t) {
		return out, model.ErrRevisionConflict
	}
	return out, nil
}
func (m *Manager) NativeDraft(ctx context.Context, source, nativeID, name, project, service string) (v1.NativeSLODraft, error) {
	out := v1.NativeSLODraft{APIVersion: v1.APIVersion, Kind: "NativeSLODraft", Tenant: m.Tenant, Source: source, NativeID: nativeID, Warnings: []string{"This is a new experimental Nines definition, not adoption or synchronization of the vendor SLO. No vendor resource is modified.", "Review queries, scope, sampling, target and window with nines plan before apply. Burn policies, alerts, corrections and notifications are not imported. Publishing later creates a separately owned native revision."}}
	t, ok := m.target(source)
	if !ok {
		return out, model.ErrNotFound
	}
	var manifest v1.ServiceLevelObjective
	found := false
	switch t.Kind {
	case "datadog":
		c, e := datadogClient(t)
		if e != nil {
			return out, e
		}
		rows, e := c.NativeCatalog(ctx)
		if e != nil {
			return out, e
		}
		for _, row := range rows {
			if row.ID == nativeID {
				manifest, e = c.NativeDraft(row, source, name, project, service)
				if e != nil {
					return out, e
				}
				found = true
				break
			}
		}
	case "newrelic":
		c, e := newrelicClient(t)
		if e != nil {
			return out, e
		}
		rows, e := c.NativeCatalog(ctx, t.NewRelicEntityGUID)
		if e != nil {
			return out, e
		}
		for _, row := range rows {
			if row.ID == nativeID {
				manifest, e = c.NativeDraft(row, source, name, project, service)
				if e != nil {
					return out, e
				}
				found = true
				break
			}
		}
	default:
		return out, model.Invalid("native import supports configured Datadog or New Relic native destinations")
	}
	if !found {
		return out, model.ErrNotFound
	}
	b, err := yaml.Marshal(manifest)
	if err != nil {
		return out, err
	}
	if _, err = model.ParseSLODefinition(b); err != nil {
		return out, err
	}
	current, ok := m.target(source)
	if !ok || nativeReadRouteDigest(current) != nativeReadRouteDigest(t) {
		return out, model.ErrRevisionConflict
	}
	out.Manifest = string(b)
	return out, nil
}

// Credential identities are private and must not be serialized as API evidence.
func nativeReadRouteDigest(t Target) string {
	return model.DefinitionDigest(struct{ Route, DDKey, DDApp, NRKey string }{RouteDigest(t), model.DefinitionDigest(t.DatadogAPIKey), model.DefinitionDigest(t.DatadogApplicationKey), model.DefinitionDigest(t.NewRelicUserKey)})
}
