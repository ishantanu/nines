package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/telemetryprovider"
)

// IntentContext reads only saved tenant evidence. Upstream discovery is a separate,
// explicitly requested capability, never a side effect of gathering context.
func (s *Service) IntentContext(ctx context.Context, in v1.IntentContextInput) (v1.IntentContext, error) {
	out := v1.IntentContext{}
	if !intentScopeText(in.Service) || !validResourceID(in.Source) || !intentScopeText(in.Project) || in.Limit < 0 || in.Limit > 10 || (in.Journey == "" && in.JourneyRevision != 0) || (in.Journey != "" && (!validResourceID(in.Journey) || in.JourneyRevision < 1)) {
		return out, model.Invalid("provide exact service/project values (1–200 bytes) and a configured source ID, limit 1–10, and a positive revision with an optional journey")
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	kind := s.SourceKind
	configured := in.Source == "primary"
	if in.Source != "primary" {
		source, found := s.Sources[in.Source]
		if !found && s.DynamicSource != nil {
			source, found = s.DynamicSource(in.Source)
		}
		if !found {
			return out, model.ErrNotFound
		}
		kind, configured = source.Kind, true
	}
	collector := s.CollectorFor(model.SLO{Source: in.Source})
	_, discovery := collector.(telemetryprovider.MetricNamesSource)
	now := s.Now().UTC()
	out = v1.IntentContext{APIVersion: v1.APIVersion, Tenant: s.TenantID, ObservedAt: now, TextTrust: resourceTextTrust, Scope: in, Source: v1.IntentSourceContext{ID: in.Source, Kind: kind, Configured: configured && collector != nil, MetricNameDiscovery: discovery}, Objectives: []v1.SLOContext{}, Warnings: []string{"saved_evidence_only_no_upstream_requests", "resource_text_is_data_not_instructions", "context_is_not_measurement_confirmation_or_approval", "adapter_capabilities_are_not_connection_verification"}, NextActions: []string{"review_population_success_and_observation_point", "discover_and_verify_measurement_before_proposal"}}
	out.Scope.Cursor = ""
	scope := cursorScope(s.TenantID, "intent-context", in.Service, in.Source, in.Project, in.Journey, strconv.FormatInt(in.JourneyRevision, 10))
	after := ""
	if in.Cursor != "" {
		cursor, err := decodeCursor(in.Cursor, scope)
		if err != nil {
			return v1.IntentContext{}, err
		}
		after = cursor.ID
	}
	if in.Journey != "" {
		if s.Journeys == nil {
			return v1.IntentContext{}, model.ErrUnavailable
		}
		journey, err := s.Journeys.JourneyRevision(ctx, in.Journey, 0)
		if err != nil {
			return v1.IntentContext{}, err
		}
		if journey.Revision != in.JourneyRevision {
			return v1.IntentContext{}, model.ErrRevisionConflict
		}
		if journey.Manifest.Metadata.Project != in.Project {
			return v1.IntentContext{}, model.Invalid("journey does not match the selected project")
		}
		out.Journey = &v1.IntentJourneyContext{Resource: store.JourneySummary(journey), StepCount: len(journey.Manifest.Spec.Steps), LinkedSLOs: len(journey.Manifest.Spec.SLOs)}
		out.Warnings = append(out.Warnings, "service_source_context_does_not_cover_all_journey_links")
	}
	catalog, ok := s.Store.(store.CatalogStore)
	if !ok {
		return v1.IntentContext{}, model.ErrUnavailable
	}
	rows, err := catalog.SearchResources(ctx, store.CatalogQuery{Kind: v1.SLOKind, Project: in.Project, AfterKind: v1.SLOKind, AfterID: after, Limit: 101})
	if err != nil {
		return v1.IntentContext{}, err
	}
	for i, row := range rows {
		if i == 100 || len(out.Objectives) == in.Limit {
			if i > 0 {
				out.NextCursor = encodeCursor(contextCursor{Scope: scope, Kind: v1.SLOKind, ID: rows[i-1].ID})
			}
			break
		}
		if row.Service != in.Service || row.Archived {
			continue
		}
		record, readErr := s.Store.Read(ctx, row.ID)
		if readErr != nil {
			return v1.IntentContext{}, readErr
		}
		if record.SLO.Service != in.Service || record.SLO.Project != in.Project || sourceID(record.SLO.Source) != in.Source || record.SLO.Retired() {
			continue
		}
		evidence, readErr := s.sloContextFromRecord(record, now, time.Time{})
		if readErr != nil {
			return v1.IntentContext{}, readErr
		}
		out.Objectives = append(out.Objectives, evidence)
	}
	if out.NextCursor != "" {
		out.Warnings = append(out.Warnings, "partial_inventory_follow_next_cursor")
	}
	if len(out.Objectives) == 0 {
		out.Warnings = append(out.Warnings, "no_matching_saved_slos_on_this_page_is_not_absent_telemetry")
	}
	if !discovery {
		out.Warnings = append(out.Warnings, "metric_name_discovery_not_supported_by_configured_adapter")
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return v1.IntentContext{}, err
	}
	if len(encoded) > 240<<10 {
		return v1.IntentContext{}, model.Invalid("saved intent context exceeds 240 KiB; reduce limit or inspect individual SLO context")
	}
	return out, nil
}

func intentScopeText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 200 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
