package testtelemetry

import (
	"context"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
)

// EvidenceInput supplies only synthetic fixture instrumentation and provenance.
func EvidenceInput(ctx context.Context, s *service.Service) (v1.IntentEvidenceInput, error) {
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "evidence-preview", Project: "test"}, Spec: v1.IntentSpec{Description: "Completed synthetic requests", Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, Outcome: "availability", NoDataPolicy: "unknown", Target: .99, WindowSeconds: 3600, StepSeconds: 60, Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "Synthetic completed requests"}}}
	plan, err := s.PlanIntent(ctx, v1.PlanIntentInput{Intent: goal})
	if err != nil {
		return v1.IntentEvidenceInput{}, err
	}
	var registry *slotemplates.Registry
	catalog, err := registry.List("")
	if err != nil {
		return v1.IntentEvidenceInput{}, err
	}
	return v1.IntentEvidenceInput{Plan: v1.PlanIntentInput{Intent: goal}, IntentDigest: plan.IntentDigest, MeasurementDigest: plan.Compiled.MeasurementDigest, SourceDigest: model.DefinitionDigest(struct{ Tenant, Source, Route string }{s.TenantID, "primary", s.SourceFingerprint("primary")}), CatalogDigest: catalog.Digest, ExpectedRevision: plan.ExpectedRevision}, nil
}
