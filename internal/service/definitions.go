package service

import (
	"context"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type ResourceRevision struct {
	Manifest any    `json:"manifest"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}
type ResourcePlan struct {
	APIVersion       string `json:"apiVersion"`
	Kind             string `json:"kind"`
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Digest           string `json:"digest"`
	Manifest         any    `json:"manifest"`
	Previous         any    `json:"previous,omitempty"`
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
	old, err := s.Store.Read(ctx, d.SLO.Metadata.Name)
	if errors.Is(err, model.ErrNotFound) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	manifest := model.DefinitionFromSLO(old.SLO)
	p.ExpectedRevision = model.ConfigRevision(old.SLO)
	p.Previous = ResourceRevision{manifest, p.ExpectedRevision, model.DefinitionDigest(manifest)}
	if model.DefinitionDigest(manifest) != p.Digest {
		if err := sloMeasurementCompatible(old.SLO, d.SLO.SLO()); err != nil {
			return p, err
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
			if err := sloMeasurementCompatible(r.SLO, next); err != nil {
				return err
			}
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
			// Observations are revision scoped; a target/policy/window edit starts fresh.
			if next.ProviderWindow != nil {
				r.Windows = nil
				var measurement *v1.BindingHealth
				if previous := r.Collection.Measurement; previous != nil {
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

// Existing interval counts must not be reinterpreted as different measurements.
func sloMeasurementCompatible(old, next model.SLO) error {
	if MeasurementDigest(old) != MeasurementDigest(next) {
		return model.Invalid("service, binding reference, stepSeconds, queries and source identify the measurement and cannot change; create a new metadata.name")
	}
	return nil
}

func sourceID(name string) string {
	if name == "" {
		return "primary"
	}
	return name
}

// MeasurementDigest is shared by the API and optional Kubernetes controller so
// definition edits cannot reinterpret an existing interval measurement history.
func MeasurementDigest(s model.SLO) string {
	return model.DefinitionDigest(struct {
		Window                       *v1.ProviderWindow
		Binding                      *v1.BindingReference
		Service, Source, Good, Total string
		Step                         int64
	}{s.ProviderWindow, s.MeasurementBinding, s.Service, sourceID(s.Source), s.GoodQuery, s.TotalQuery, s.StepSeconds})
}
