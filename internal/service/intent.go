package service

import (
	"context"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// PlanIntent verifies and compiles an inline measurement without resource writes.
// Explicit legacy catalog selection remains supported. Neither path establishes
// live traffic or business correctness; conversation confirmation is separate.
func (s *Service) PlanIntent(ctx context.Context, in v1.PlanIntentInput) (v1.IntentPlan, error) {
	return s.planIntent(ctx, in, true)
}

func (s *Service) planIntent(ctx context.Context, in v1.PlanIntentInput, includeJourney bool) (v1.IntentPlan, error) {
	out := v1.IntentPlan{SourceDigest: s.SourceFingerprint(in.Intent.Spec.Source), APIVersion: v1.APIVersion, Tenant: s.TenantID, Intent: in.Intent, IntentDigest: model.DefinitionDigest(in.Intent), Candidates: []v1.IntentCandidate{}, Clarifications: []string{}, Warnings: []string{"Description is explanatory text, not an executable condition. Review the measurement's success labels, observation point, retries and denominator.", "Telemetry validation checks recent metadata and series. It is not a traffic backtest or proof of end-to-end journey success.", "Planning writes no resources or recording/alert rules. An approved intent change set can apply the SLO and CUJ link together; rule deployment requires its separate destination review."}}
	if err := model.ValidateIntent(in.Intent); err != nil {
		return out, err
	}
	if _, err := s.telemetrySource(in.Intent.Spec.Source); err != nil {
		return out, err
	}
	if in.Intent.Spec.Outcome != "availability" && in.Intent.Spec.Outcome != "latency" {
		out.Status = "unsupported_measurement"
		out.Clarifications = append(out.Clarifications, "This outcome needs an implemented measurement adapter and instrumentation contract. Availability and classic-histogram latency are supported.")
		return out, nil
	}
	var journey *v1.IntentJourneyPlan
	if includeJourney {
		var err error
		journey, err = s.intentJourney(ctx, in.Intent)
		if err != nil {
			return out, err
		}
	}
	var compiled v1.CompiledBinding
	var err error
	if in.Intent.Spec.Measurement != nil {
		if in.Binding != nil {
			return out, model.Invalid("an inline intent does not use a separate measurement binding")
		}
		inline, e := s.CompileMeasurement(ctx, inlineIntentManifest(in.Intent))
		if e != nil {
			return out, e
		}
		compiled = v1.CompiledBinding{APIVersion: inline.APIVersion, Tenant: inline.Tenant, CompilerVersion: inline.CompilerVersion, MeasurementDigest: inline.MeasurementDigest, Evidence: inline.Evidence, Manifest: inline.Manifest, YAML: inline.YAML, Digest: inline.Digest, Warnings: inline.Warnings}
	} else {
		if in.Binding == nil {
			st, err := s.topologyStore()
			if err != nil {
				return out, err
			}
			rows, err := st.ListTopology(ctx, v1.BindingKind, "", 101)
			if err != nil {
				return out, err
			}
			if len(rows) > 100 {
				out.Truncated = true
				rows = rows[:100]
			}
			for _, r := range rows {
				var b model.MeasurementBinding
				if err := json.Unmarshal(r.Manifest, &b); err != nil {
					return out, err
				}
				if model.IntentMatchesBinding(in.Intent, b) {
					if len(out.Candidates) == 20 {
						out.Truncated = true
						continue
					}
					out.Candidates = append(out.Candidates, v1.IntentCandidate{Binding: v1.BindingReference{Name: r.ID, Revision: r.Revision, Digest: r.Digest}, Definition: v1.MeasurementBinding(b)})
				}
			}
			out.Status = "needs_selection"
			out.Clarifications = append(out.Clarifications, "Select a binding and its reviewed revision after confirming population and success semantics. Even a single candidate requires an explicit choice.")
			if len(out.Candidates) == 0 {
				out.Status = "needs_instrumentation"
				out.Clarifications = []string{"No exact binding matches the project, service, source, entire population and threshold. Define the measurement inside this SLO; do not broaden the population or substitute a percentile."}
			}
			if out.Truncated {
				if len(out.Candidates) == 0 {
					out.Status = "catalog_incomplete"
				}
				out.Clarifications = append(out.Clarifications, "Catalog results are bounded to 100 scanned bindings and 20 matching candidates. Absence is inconclusive; use the paginated binding catalog and select a matching binding explicitly.")
			}
			return out, nil
		}
		if !validResourceID(in.Binding.Name) || in.Binding.ExpectedRevision < 1 {
			return out, model.Invalid("binding requires a name and positive expectedRevision")
		}
		row, err := s.ReadTopology(ctx, v1.BindingKind, in.Binding.Name, 0)
		if err != nil {
			return out, err
		}
		if row.Revision != in.Binding.ExpectedRevision {
			return out, model.ErrRevisionConflict
		}
		var b model.MeasurementBinding
		if err = json.Unmarshal(row.Manifest, &b); err != nil {
			return out, err
		}
		if !model.IntentMatchesBinding(in.Intent, b) {
			return out, model.Invalid("selected binding does not match the intent's exact population, outcome, threshold, service, source and project")
		}
		compiled, err = s.CompileBinding(ctx, row.ID, v1.CompileBindingInput{ExpectedRevision: row.Revision, Name: in.Intent.Metadata.Name, DisplayName: in.Intent.Metadata.Name, Target: in.Intent.Spec.Target, WindowSeconds: in.Intent.Spec.WindowSeconds, Policies: in.Intent.Spec.Policies, Policy: in.Intent.Spec.Policy})
		if err != nil {
			return out, err
		}

	}
	compiled.Manifest.Spec.Intent = &in.Intent.Spec
	def := model.SLODefinition(compiled.Manifest)
	if err = def.Validate(); err != nil {
		return out, err
	}
	// Validate existing identity compatibility as well as the generated artifact.
	resourcePlan, err := s.PlanDefinition(ctx, model.Definition{SLO: &def})
	if err != nil {
		return out, err
	}
	encoded, err := yaml.Marshal(compiled.Manifest)
	if err != nil {
		return out, err
	}
	compiled.YAML = string(encoded)
	compiled.Digest = model.DefinitionDigest(compiled.Manifest)
	out.Warnings = append(out.Warnings, resourcePlan.Warnings...)
	out.ExpectedRevision = resourcePlan.ExpectedRevision
	out.Compiled = &compiled
	out.Status = "ready"
	out.Journey = journey
	// Provider discovery can race a CUJ edit. Reject instead of returning stale links.
	if ref := in.Intent.Spec.Journey; ref != nil && includeJourney {
		latest, err := s.Journeys.JourneyRevision(ctx, ref.Name, 0)
		if err != nil {
			return out, err
		}
		if latest.Revision != ref.ExpectedRevision {
			return out, model.ErrRevisionConflict
		}
	}
	if s.SourceFingerprint(in.Intent.Spec.Source) != out.SourceDigest {
		return out, model.ErrRevisionConflict
	}
	return out, nil
}

func (s *Service) intentJourney(ctx context.Context, in v1.SLOIntent) (*v1.IntentJourneyPlan, error) {
	ref := in.Spec.Journey
	if ref == nil {
		return nil, nil
	}
	if s.Journeys == nil {
		return nil, model.ErrUnavailable
	}
	r, err := s.Journeys.JourneyRevision(ctx, ref.Name, 0)
	if err != nil {
		return nil, err
	}
	if r.Revision != ref.ExpectedRevision {
		return nil, model.ErrRevisionConflict
	}
	j := r.Manifest
	if j.Metadata.Project != in.Metadata.Project {
		return nil, model.Invalid("intent and CUJ must belong to the same project")
	}
	found := false
	for k := range j.Spec.Steps {
		if j.Spec.Steps[k].ID == ref.Step {
			found = true
			present := false
			for _, id := range j.Spec.Steps[k].SLORefs {
				present = present || id == in.Metadata.Name
			}
			if !present {
				j.Spec.Steps[k].SLORefs = append(j.Spec.Steps[k].SLORefs, in.Metadata.Name)
			}
		}
	}
	if !found {
		return nil, model.Invalid("selected CUJ step does not exist")
	}
	found = false
	for _, l := range j.Spec.SLOs {
		if l.ID == in.Metadata.Name {
			found = true
			if l.Dimension != in.Spec.Outcome || l.Required != ref.Required {
				return nil, model.Invalid("existing CUJ link has different semantics; review it explicitly")
			}
		}
	}
	if !found {
		j.Spec.SLOs = append(j.Spec.SLOs, v1.JourneySLO{ID: in.Metadata.Name, Dimension: in.Spec.Outcome, Required: ref.Required})
	}
	if err = j.Validate(); err != nil {
		return nil, err
	}
	b, err := yaml.Marshal(j)
	if err != nil {
		return nil, err
	}
	return &v1.IntentJourneyPlan{ExpectedRevision: r.Revision, Manifest: v1.CriticalUserJourney(j), YAML: string(b), Digest: model.DefinitionDigest(j)}, nil
}

func (s *Service) SourceFingerprint(source string) string {
	c := s.CollectorFor(model.SLO{Source: source})
	if described, ok := c.(interface{ SourceFingerprint() string }); ok {
		return described.SourceFingerprint()
	}
	// Custom in-process adapters may have no endpoint. Production Prometheus and
	// Mimir clients provide an explicit configured route fingerprint above.
	return model.DefinitionDigest(struct{ Source, Type string }{sourceID(source), fmt.Sprintf("%T", c)})
}
