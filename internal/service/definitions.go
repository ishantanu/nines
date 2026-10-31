package service

import (
	"context"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

type ResourceRevision struct {
	Manifest any    `json:"manifest"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}
type ResourcePlan struct {
	MeasurementChanged bool     `json:"measurementChanged,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	APIVersion         string   `json:"apiVersion"`
	Kind               string   `json:"kind"`
	Action             string   `json:"action"`
	ExpectedRevision   int64    `json:"expectedRevision"`
	Digest             string   `json:"digest"`
	Manifest           any      `json:"manifest"`
	Previous           any      `json:"previous,omitempty"`
}

func (s *Service) PlanDefinition(ctx context.Context, d model.Definition) (ResourcePlan, error) {
	if d.Binding != nil || d.DependencyMap != nil {
		return s.planTopology(ctx, d)
	}
	p := ResourcePlan{APIVersion: model.APIVersion, Kind: d.Kind(), Manifest: d.Manifest(), Digest: model.DefinitionDigest(d.Manifest()), Action: "create"}
	if d.Journey != nil {
		plan, err := s.PlanJourney(ctx, *d.Journey)
		if err != nil {
			return p, err
		}
		p.Action = plan.Action
		p.ExpectedRevision = plan.ExpectedRevision
		if plan.Previous != nil {
			p.Previous = plan.Previous
		}
		return p, nil
	}
	if d.SLO == nil {
		return p, model.Invalid("resource definition is required")
	}
	if err := d.SLO.Validate(); err != nil {
		return p, err
	}
	if err := s.ValidateSource(d.SLO.SLO()); err != nil {
		return p, err
	}
	if err := s.validateBindingReference(ctx, d.SLO.SLO()); err != nil {
		return p, err
	}
	if !d.SLO.SLO().AlertsEnabled() {
		p.Warnings = append(p.Warnings, "Lifecycle "+string(d.SLO.Spec.Lifecycle)+": measurements and recording rules remain enabled; Nines and generated provider burn alerts are disabled.")
	}
	for _, policy := range d.SLO.SLO().AlertPolicies() {
		if policy.MinEvents > 0 {
			p.Warnings = append(p.Warnings, "minEvents gates both alert windows by observed traffic count; it does not establish statistical confidence.")
			break
		}
	}
	if s.RuleManager != nil {
		if target, ok := s.RuleManager.TargetFor(d.SLO.SLO()); ok && target.AutoDeploy {
			p.Warnings = append(p.Warnings, "Saving this definition queues automatic recording and alert rule deployment under the data source policy. Activation and runtime evaluation are reported separately.")
		}
	}
	old, err := s.Store.Read(ctx, d.SLO.Metadata.Name)
	if errors.Is(err, model.ErrNotFound) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if old.SLO.Retired() {
		return p, model.Invalid("SLO is archived; its retained definition cannot be edited")
	}
	if old.SLO.RuleDestination != "" && rules.ConnectionID(d.SLO.SLO()) != rules.ConnectionID(old.SLO) {
		return p, model.Invalid("withdraw owned rules from the original connection before changing the SLO source")
	}
	if !old.SLO.AlertsEnabled() && d.SLO.SLO().AlertsEnabled() {
		p.Warnings = append(p.Warnings, "Promoting this SLO to committed enables its configured burn alerts. Review the thresholds, traffic guard and rule destination before applying.")
	}
	manifest := model.DefinitionFromSLO(old.SLO)
	p.ExpectedRevision = model.ConfigRevision(old.SLO)
	p.Previous = ResourceRevision{manifest, p.ExpectedRevision, model.DefinitionDigest(manifest)}
	if model.DefinitionDigest(manifest) != p.Digest {
		p.MeasurementChanged = MeasurementDigest(old.SLO) != MeasurementDigest(d.SLO.SLO())
		if p.MeasurementChanged {
			p.Warnings = append(p.Warnings, "This edit changes the measurement. The previous definition and retained evidence will be archived; active samples, coverage and measurement health start fresh. Review the rule destination and its automatic or manual delivery policy. Operator destination changes still require withdrawal from the original route.")
		}
		p.Action = "update"
		return p, nil
	}
	p.Action = "unchanged"
	return p, nil
}
func (s *Service) ApplyDefinition(ctx context.Context, d model.Definition, expected int64) (ResourceRevision, error) {
	if d.SLO != nil {
		s.DefinitionMu.Lock()
		defer s.DefinitionMu.Unlock()
	}
	if expected < 0 {
		return ResourceRevision{}, model.Invalid("expected revision must be nonnegative")
	}
	if d.Binding != nil || d.DependencyMap != nil {
		return s.applyTopology(ctx, d, expected)
	}
	if d.Journey != nil {
		r, err := s.ApplyJourney(ctx, *d.Journey, expected)
		return ResourceRevision{r.Manifest, r.Revision, r.Digest}, err
	}
	if d.SLO == nil {
		return ResourceRevision{}, model.Invalid("resource definition is required")
	}
	if err := d.SLO.Validate(); err != nil {
		return ResourceRevision{}, err
	}
	if err := s.validateBindingReference(ctx, d.SLO.SLO()); err != nil {
		return ResourceRevision{}, err
	}
	digest := model.DefinitionDigest(*d.SLO)
	if err := s.ValidateSource(d.SLO.SLO()); err != nil {
		return ResourceRevision{}, err
	}
	old, err := s.Store.Read(ctx, d.SLO.Metadata.Name)
	if err == nil {
		var result ResourceRevision
		err = s.Store.Update(ctx, d.SLO.Metadata.Name, func(r *model.Record) error {
			if r.SLO.Retired() {
				return model.Invalid("SLO is archived; its retained definition cannot be edited")
			}
			revision := model.ConfigRevision(r.SLO)
			if expected != revision {
				return model.ErrRevisionConflict
			}
			manifest := model.DefinitionFromSLO(r.SLO)
			if model.DefinitionDigest(manifest) == digest {
				result = ResourceRevision{manifest, revision, digest}
				return nil
			}
			next := d.SLO.SLO()
			if r.SLO.RuleDestination != "" && rules.ConnectionID(next) != rules.ConnectionID(r.SLO) {
				return model.Invalid("withdraw owned rules from the original connection before changing the SLO source")
			}
			next.RuleDestination = r.SLO.RuleDestination
			measurementChanged := MeasurementDigest(r.SLO) != MeasurementDigest(next)
			next.CreatedAt = r.SLO.CreatedAt
			next.Revision = revision + 1
			// A definition update ends an existing firing episode explicitly, then
			// starts evaluation of the new policy with a fresh pending timer.
			if len(r.Alert.Policies) > 0 {
				for name, state := range r.Alert.Policies {
					if state.Phase == "firing" {
						r.Events = append(r.Events, model.AlertEvent{ID: model.NewID(), SLOID: r.SLO.ID, Kind: "resolved", Reason: "configuration_changed", ConfigRevision: revision, Time: s.Now().UTC().Truncate(time.Duration(r.SLO.StepSeconds) * time.Second), Policy: name, Severity: state.Severity})
					}
				}
			} else if r.Alert.Phase == "firing" {
				r.Events = append(r.Events, model.AlertEvent{ID: model.NewID(), SLOID: r.SLO.ID, Kind: "resolved", Reason: "configuration_changed", ConfigRevision: revision, Time: s.Now().UTC().Truncate(time.Duration(r.SLO.StepSeconds) * time.Second)})
			}
			r.SLO = next
			// Store.Update archives the previous definition and observations atomically.
			// New measurement identities cannot reuse old interval counts or health.
			if measurementChanged {
				r.Points = map[int64]model.Point{}
			}
			if measurementChanged || next.ProviderWindow != nil {
				r.Windows = nil
				var measurement *v1.BindingHealth
				if previous := r.Collection.Measurement; previous != nil && !measurementChanged {
					copy := *previous
					copy.SLORevision = next.Revision
					measurement = &copy
				}
				r.Collection = v1.CollectionHealth{State: "not_observed", Measurement: measurement}
			}
			r.Alert = model.AlertState{Phase: "inactive"}
			result = ResourceRevision{*d.SLO, next.Revision, digest}
			return nil
		})
		return result, err
	}
	_ = old

	if !errors.Is(err, model.ErrNotFound) {
		return ResourceRevision{}, err
	}
	if expected != 0 {
		return ResourceRevision{}, model.ErrRevisionConflict
	}
	if err = s.Create(ctx, d.SLO.SLO()); err != nil {
		if errors.Is(err, model.ErrConflict) {
			err = model.ErrRevisionConflict
		}
		return ResourceRevision{}, err
	}
	return ResourceRevision{*d.SLO, 1, digest}, nil
}

func sourceID(name string) string {
	if name == "" {
		return "primary"
	}
	return name
}

// MeasurementDigest is shared by the API and optional Kubernetes controller so
// measurement edits can archive and reset observations without reinterpreting history.
func MeasurementDigest(s model.SLO) string {
	language := s.QueryLanguage
	if language == "promql" {
		language = ""
	}
	return model.DefinitionDigest(struct {
		Measurement                            *v1.MeasurementSpec
		Window                                 *v1.ProviderWindow
		Binding                                *v1.BindingReference
		Service, Source, Good, Total, Language string
		Step                                   int64
	}{s.Measurement, s.ProviderWindow, s.MeasurementBinding, s.Service, sourceID(s.Source), s.GoodQuery, s.TotalQuery, language, s.StepSeconds})
}
