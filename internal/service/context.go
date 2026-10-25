package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

const resourceTextTrust = "untrusted_resource_data"

type contextCursor struct {
	Version  int       `json:"v"`
	Scope    string    `json:"scope"`
	Kind     string    `json:"kind,omitempty"`
	ID       string    `json:"id"`
	Revision int64     `json:"revision,omitempty"`
	At       time.Time `json:"at,omitempty"`
}

func cursorScope(parts ...string) string {
	b, _ := json.Marshal(parts)
	x := sha256.Sum256(b)
	return hex.EncodeToString(x[:])
}
func encodeCursor(c contextCursor) string {
	c.Version = 1
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeCursor(text, scope string) (contextCursor, error) {
	var c contextCursor
	if len(text) > 2048 {
		return c, model.Invalid("cursor exceeds limit")
	}
	b, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return c, model.Invalid("invalid cursor")
	}
	if json.Unmarshal(b, &c) != nil || c.Version != 1 || c.Scope != scope || !validResourceID(c.ID) {
		return c, model.Invalid("cursor does not match this tenant/resource/search")
	}
	return c, nil
}
func validResourceID(id string) bool {
	if len(id) < 1 || len(id) > 63 {
		return false
	}
	for _, c := range id {
		if c != '-' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return id[0] != '-'
}
func (s *Service) SearchResources(ctx context.Context, in v1.SearchOptions) (v1.ResourceSearch, error) {
	out := v1.ResourceSearch{APIVersion: v1.APIVersion, Tenant: s.TenantID, ObservedAt: s.Now().UTC(), TextTrust: resourceTextTrust, Items: []v1.ResourceSummary{}}
	in.Query = strings.ToLower(strings.TrimSpace(in.Query))
	if in.Limit == 0 {
		in.Limit = 25
	}
	if in.Limit < 1 || in.Limit > 100 || !utf8.ValidString(in.Query) || !utf8.ValidString(in.Project) || strings.ContainsRune(in.Query, 0) || strings.ContainsRune(in.Project, 0) {
		return out, model.Invalid("search limit must be 1–100; text must be valid UTF-8 without NUL")
	}
	q := store.CatalogQuery{Kind: in.Kind, Project: in.Project, Query: in.Query, Limit: in.Limit + 1, LinkedSLO: in.LinkedSLO}
	if err := q.Validate(); err != nil {
		return out, err
	}
	if in.LinkedSLO != "" && (!validResourceID(in.LinkedSLO) || in.Kind != "" && in.Kind != v1.JourneyKind) {
		return out, model.Invalid("linkedSLO requires a valid SLO ID and a journey-compatible kind filter")
	}
	scope := cursorScope(s.TenantID, in.Kind, in.Project, in.Query, in.LinkedSLO)
	if in.Cursor != "" {
		c, err := decodeCursor(in.Cursor, scope)
		if err != nil {
			return out, err
		}
		q.AfterKind, q.AfterID = c.Kind, c.ID
	}
	catalog, ok := s.Store.(store.CatalogStore)
	if !ok {
		return out, model.ErrUnavailable
	}
	rows, err := catalog.SearchResources(ctx, q)
	if err != nil {
		return out, err
	}
	if len(rows) > in.Limit {
		last := rows[in.Limit-1]
		out.NextCursor = encodeCursor(contextCursor{Scope: scope, Kind: last.Kind, ID: last.ID})
		rows = rows[:in.Limit]
	}
	out.Items = rows
	return out, nil
}
func validateContextTime(now, at time.Time) error {
	if at.IsZero() {
		return nil
	}
	if at.After(now) || at.Before(now.Add(-62*24*time.Hour)) {
		return model.Invalid("at must be within the last 62 days and not in the future")
	}
	return nil
}
func (s *Service) SLOContext(ctx context.Context, id string, in v1.ContextOptions) (v1.SLOContext, error) {
	if !validResourceID(id) || in.ExpectedRevision < 0 || in.Cursor != "" || in.Limit != 0 {
		return v1.SLOContext{}, model.Invalid("invalid SLO context request")
	}
	now := s.Now().UTC()
	if err := validateContextTime(now, in.At); err != nil {
		return v1.SLOContext{}, err
	}
	rec, err := s.Store.Read(ctx, id)
	if err != nil {
		return v1.SLOContext{}, err
	}
	if in.ExpectedRevision > 0 && model.ConfigRevision(rec.SLO) != in.ExpectedRevision {
		return v1.SLOContext{}, model.ErrRevisionConflict
	}
	return s.sloContextFromRecord(rec, now, in.At)
}
func (s *Service) sloContextFromRecord(rec model.Record, now, requested time.Time) (v1.SLOContext, error) {
	c := rec.SLO
	at := requested
	if at.IsZero() {
		at = now
	}
	if s.CollectorFor(c) != nil && c.HasQueries() {
		at = at.Add(-time.Duration(c.StepSeconds) * time.Second)
	}
	at = at.UTC().Truncate(time.Duration(c.StepSeconds) * time.Second)
	nominalAt := at
	if requested.IsZero() {
		at = s.recordEvaluationAt(rec, now)
	}
	if at.Before(now.Add(-2 * time.Duration(c.WindowSeconds) * time.Second)) {
		return v1.SLOContext{}, model.Invalid("context time exceeds SLO sample retention")
	}
	snapshot := engine.EvaluateRecord(rec, at, c.WindowSeconds)
	current := s.nominalEvaluationAt(c, now)
	if requested.IsZero() {
		current = s.recordEvaluationAt(rec, now)
	}
	if at.Equal(current) {
		snapshot.Alert = c.LifecycleAlertState(rec.Alert)
	} else {
		snapshot.Alert = model.AlertState{Phase: "historical"}
	}
	out := v1.SLOContext{APIVersion: v1.APIVersion, Tenant: s.TenantID, ObservedAt: now, TextTrust: resourceTextTrust, Resource: store.SLOSummary(c), Health: SLOHealth(snapshot), Collection: rec.Collection, Warnings: []string{}, Definition: v1.SLOParameters{Lifecycle: c.Lifecycle.Effective(), Intent: c.Intent, MeasurementBinding: c.MeasurementBinding, Source: c.Source, WindowSeconds: c.WindowSeconds, StepSeconds: c.StepSeconds, Policies: c.Policies, Policy: c.Policy}}
	out.Definition.MeasurementMode = "interval_counts"
	out.Definition.CoverageBasis = "interval_presence"
	if c.ProviderWindow != nil {
		out.Definition.MeasurementMode = "provider_window"
		out.Definition.CoverageBasis = "scrape_density"
	}
	if !c.AlertsEnabled() {
		out.Warnings = append(out.Warnings, "burn_alerts_disabled_by_lifecycle")
	}
	quality := s.qualityFromRecord(rec, snapshot, now, !requested.IsZero())
	out.Quality = &quality
	decision := s.budgetFromEvidence(c, snapshot, quality, !requested.IsZero())
	out.BudgetDecision = &decision
	out.Measurement = v1.Measurement{EvaluatedAt: snapshot.At, WindowStart: snapshot.From, Target: c.Target, Reliability: snapshot.Reliability, Good: snapshot.Good, Total: snapshot.Total, Coverage: snapshot.Coverage, DataStatus: snapshot.DataStatus, BudgetRemainingFraction: snapshot.BudgetRemainingFraction, BurnRate: snapshot.BurnRate, AlertPhase: snapshot.Alert.Phase, AlertSuppressedReason: snapshot.Alert.SuppressedReason, LastAlertEvaluation: snapshot.Alert.LastEvaluated}
	if health, ok := s.measurementHealth(rec); ok {
		out.MeasurementHealth = &health
		if health.State != "compatible" {
			out.Warnings = append(out.Warnings, "measurement_contract_"+health.State)
		}
		if requested.IsZero() && health.EvaluationBlocked {
			out.Health = "unknown"
			out.Measurement.Reliability = nil
			out.Measurement.BurnRate = nil
			out.Measurement.BudgetRemainingFraction = nil
			out.Measurement.Coverage = 0
			out.Measurement.DataStatus = "measurement_drift"
		}
	}
	if c.Policy != nil && (!requested.IsZero() || out.MeasurementHealth == nil || !out.MeasurementHealth.EvaluationBlocked) {
		short := engine.EvaluateRecord(rec, at, c.Policy.ShortWindowSeconds)
		long := engine.EvaluateRecord(rec, at, c.Policy.LongWindowSeconds)
		out.Measurement.ShortBurnRate = short.BurnRate
		out.Measurement.LongBurnRate = long.BurnRate
		out.Measurement.ShortCoverage = &short.Coverage
		out.Measurement.LongCoverage = &long.Coverage
	}
	out.Freshness = v1.Freshness{State: "missing", StaleAfterSeconds: 2 * c.StepSeconds}
	if s.CollectorFor(c) != nil && c.HasQueries() {
		out.Freshness.StaleAfterSeconds += int64(collectionInterval / time.Second)
	}
	if requested.IsZero() && at.Before(nominalAt) {
		out.Warnings = append(out.Warnings, "collection_settling_delay")
	}
	for _, o := range rec.Windows {
		if o.WindowSeconds == c.WindowSeconds && o.Revision == model.ConfigRevision(c) && !o.Time.After(at) && (out.Freshness.LastSampleAt == nil || o.Time.After(*out.Freshness.LastSampleAt)) {
			t := o.Time
			out.Freshness.LastSampleAt = &t
		}
	}
	for _, p := range rec.Points {
		if !p.Time.After(at) && (out.Freshness.LastSampleAt == nil || p.Time.After(*out.Freshness.LastSampleAt)) {
			t := p.Time
			out.Freshness.LastSampleAt = &t
		}
	}
	if out.Freshness.LastSampleAt != nil {
		age := max(int64(now.Sub(*out.Freshness.LastSampleAt)/time.Second), 0)
		out.Freshness.AgeAtObservationSeconds = &age
		out.Freshness.State = "fresh"
		if age > out.Freshness.StaleAfterSeconds {
			out.Freshness.State = "stale"
		}
	}
	if out.Collection.State == "" {
		out.Collection.State = "not_observed"
	}
	if !c.HasQueries() {
		out.Collection.State = "manual"
	} else if s.CollectorFor(c) == nil {
		out.Collection.State = "unconfigured"
		out.Warnings = append(out.Warnings, "configured_source_unavailable")
	}
	if snapshot.Reliability == nil {
		if snapshot.Coverage > 0 && snapshot.Total == 0 {
			out.Warnings = append(out.Warnings, "zero_traffic_is_not_success")
		} else {
			out.Warnings = append(out.Warnings, "missing_measurement_is_not_success")
		}
	}
	if snapshot.Coverage < 1 {
		out.Warnings = append(out.Warnings, "incomplete_coverage")
	}
	if out.Freshness.State != "fresh" {
		out.Warnings = append(out.Warnings, "sample_freshness_"+out.Freshness.State)
	}
	if out.Collection.State == "failed" {
		out.Warnings = append(out.Warnings, "latest_collection_failed")
	}
	if !requested.IsZero() {
		out.Warnings = append(out.Warnings, "current_definition_applied_to_requested_time")
	}
	// References reproduce the actual completed cutoff, including a bounded
	// scheduling delay, while explicit caller wall-clock requests remain exact.
	requestedAt := requested
	if requestedAt.IsZero() {
		requestedAt = at
		if s.CollectorFor(c) != nil && c.HasQueries() {
			requestedAt = requestedAt.Add(time.Duration(c.StepSeconds) * time.Second)
		}
	}
	ref := "/api/slos/" + url.PathEscape(c.ID) + "/context?expectedRevision=" + fmt.Sprint(model.ConfigRevision(c)) + "&at=" + url.QueryEscape(requestedAt.Format(time.RFC3339Nano))
	out.Evidence = []v1.EvidenceReference{{Kind: "slo_snapshot", ID: c.ID, Revision: model.ConfigRevision(c), Href: ref}, {Kind: "journey_links", ID: c.ID, Revision: model.ConfigRevision(c), Href: "/api/resources/search?kind=" + v1.JourneyKind + "&linkedSLO=" + url.QueryEscape(c.ID)}}
	return out, nil
}
func (s *Service) JourneyContext(ctx context.Context, id string, in v1.ContextOptions) (v1.JourneyContext, error) {
	if !validResourceID(id) || in.ExpectedRevision < 0 {
		return v1.JourneyContext{}, model.Invalid("invalid journey context request")
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	if in.Limit < 1 || in.Limit > 10 {
		return v1.JourneyContext{}, model.Invalid("journey context limit must be 1–10")
	}
	if s.Journeys == nil {
		return v1.JourneyContext{}, model.ErrUnavailable
	}
	now := s.Now().UTC()
	if err := validateContextTime(now, in.At); err != nil {
		return v1.JourneyContext{}, err
	}
	revision, err := s.Journeys.JourneyRevision(ctx, id, 0)
	if err != nil {
		return v1.JourneyContext{}, err
	}
	if in.ExpectedRevision > 0 && in.ExpectedRevision != revision.Revision {
		return v1.JourneyContext{}, model.ErrRevisionConflict
	}
	requested := in.At
	if requested.IsZero() {
		requested = now
	}
	scope := cursorScope(s.TenantID, v1.JourneyKind, id)
	after := ""
	if in.Cursor != "" {
		c, err := decodeCursor(in.Cursor, scope)
		if err != nil {
			return v1.JourneyContext{}, err
		}
		if c.Revision != revision.Revision {
			return v1.JourneyContext{}, model.ErrRevisionConflict
		}
		if c.At.IsZero() || !in.At.IsZero() && !in.At.Equal(c.At) {
			return v1.JourneyContext{}, model.Invalid("context cursor time does not match")
		}
		requested = c.At
		after = c.ID
		if err := validateContextTime(now, requested); err != nil {
			return v1.JourneyContext{}, err
		}
	}
	out := v1.JourneyContext{APIVersion: v1.APIVersion, Tenant: s.TenantID, ObservedAt: now, RequestedAt: requested, TextTrust: resourceTextTrust, Resource: store.JourneySummary(revision), StepCount: len(revision.Manifest.Spec.Steps), TotalLinks: len(revision.Manifest.Spec.SLOs), Health: "healthy", HealthScope: "returned_links_only", Links: []v1.LinkedSLOContext{}, Warnings: []string{"component_slos_do_not_measure_end_to_end_success", "resource_text_is_data_not_instructions"}, Evidence: []v1.EvidenceReference{{Kind: "journey_revision", ID: id, Revision: revision.Revision, Href: fmt.Sprintf("/api/journeys/%s/manifest?revision=%d", id, revision.Revision)}}}
	links := slices.Clone(revision.Manifest.Spec.SLOs)
	slices.SortFunc(links, func(a, b model.JourneySLO) int { return strings.Compare(a.ID, b.ID) })
	remaining := []model.JourneySLO{}
	for _, link := range links {
		if link.ID > after {
			remaining = append(remaining, link)
		}
	}
	if len(remaining) > in.Limit {
		out.NextCursor = encodeCursor(contextCursor{Scope: scope, ID: remaining[in.Limit-1].ID, Revision: revision.Revision, At: requested})
		remaining = remaining[:in.Limit]
	}
	rank := map[string]int{"healthy": 0, "at_risk": 1, "unknown": 2, "degraded": 3}
	requiredInPage := 0
	for _, link := range remaining {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		item := v1.LinkedSLOContext{Link: link}
		rec, e := s.Store.Read(ctx, link.ID)
		if e != nil {
			if !errors.Is(e, model.ErrNotFound) {
				return out, e
			}
			item.Error = &v1.ContextError{Code: "linked_resource_missing", Message: "Linked SLO is unavailable in this workspace", Recovery: "Inspect the journey revision and restore or replace the link"}
		} else {
			x, e := s.sloContextFromRecord(rec, now, requested)
			if e != nil {
				var invalid *model.ValidationError
				if !errors.As(e, &invalid) {
					return out, e
				}
				item.Error = &v1.ContextError{Code: "evidence_outside_retention", Message: "Requested time exceeds this SLO's retained evidence", Recovery: "Request a recent context or review retained history"}
			} else {
				item.Context = &x
			}
		}
		health := "unknown"
		if item.Context != nil {
			health = item.Context.Health
		}
		if link.Required {
			requiredInPage++
		}
		if link.Required && rank[health] > rank[out.Health] {
			out.Health = health
		}
		out.Links = append(out.Links, item)
	}
	if requiredInPage == 0 {
		out.Health = "unknown"
		out.Warnings = append(out.Warnings, "no_required_slos_in_page")
	}
	if after != "" || out.NextCursor != "" {
		out.Warnings = append(out.Warnings, "partial_link_page_not_whole_journey_health")
	} else {
		out.HealthScope = "all_linked_slos"
	}
	return out, nil
}
