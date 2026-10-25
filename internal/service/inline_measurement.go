package service

import (
	"context"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// CompileMeasurement verifies the SLO-owned contract and returns one SLO.
// No catalog, binding resource, or definition write is involved.
func (s *Service) CompileMeasurement(ctx context.Context, manifest v1.ServiceLevelObjective) (v1.CompiledMeasurement, error) {
	out := v1.CompiledMeasurement{APIVersion: v1.APIVersion, Tenant: s.TenantID, CompilerVersion: "inline-measurement/1", Manifest: manifest}
	d := model.SLODefinition(manifest)
	if d.Spec.Measurement == nil {
		return out, model.Invalid("spec.measurement is required")
	}
	if err := d.Validate(); err != nil {
		return out, err
	}
	c := d.SLO()
	if err := s.ValidateSource(c); err != nil {
		return out, err
	}
	source := s.SourceFingerprint(c.Source)
	evidence, err := s.DiscoverTelemetry(ctx, sourceID(c.Source), c.Measurement.Metric, c.Measurement.Labels)
	if err != nil {
		return out, err
	}
	if err = verifyBindingEvidence(model.InlineBinding(c), evidence); err != nil {
		return out, err
	}
	if s.SourceFingerprint(c.Source) != source {
		return out, model.ErrRevisionConflict
	}
	out.MeasurementDigest = MeasurementDigest(c)
	out.Evidence = evidence
	out.Digest = model.DefinitionDigest(manifest)
	body, err := yaml.Marshal(manifest)
	out.YAML = string(body)
	out.Warnings = []string{"Review the observation point, eligible population, success criteria and retry semantics before applying.", "Recent telemetry metadata and series verify the measurement contract, not business correctness or full historical coverage.", "Compilation saves no resources and deploys no rules. The SLO and its measurement are reviewed and versioned together."}
	return out, err
}
func inlineIntentManifest(goal v1.SLOIntent) v1.ServiceLevelObjective {
	return v1.NewSLO(goal.Metadata.Name, goal.Metadata.Project, v1.SLOSpec{ErrorBudgetPolicy: goal.Spec.ErrorBudgetPolicy, Quality: goal.Spec.Quality, Lifecycle: goal.Spec.Lifecycle, Measurement: goal.Spec.Measurement, Intent: &goal.Spec, DisplayName: goal.Metadata.Name, Service: goal.Spec.Service, Source: goal.Spec.Source, Target: goal.Spec.Target, WindowSeconds: goal.Spec.WindowSeconds, StepSeconds: goal.Spec.StepSeconds, Policies: goal.Spec.Policies, Policy: goal.Spec.Policy})
}
