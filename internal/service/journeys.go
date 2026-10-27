package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type JourneyLinkStatus struct {
	Link   model.JourneySLO `json:"link"`
	SLO    model.SLO        `json:"slo"`
	Status model.Snapshot   `json:"status"`
	Health string           `json:"health"`
}
type JourneyView struct {
	Resource model.JourneyRevision `json:"resource"`
	Health   string                `json:"health"`
	Links    []JourneyLinkStatus   `json:"links"`
	Reasons  []string              `json:"reasons"`
}
type JourneyPlan struct {
	Action           string                 `json:"action"`
	ExpectedRevision int64                  `json:"expectedRevision"`
	Digest           string                 `json:"digest"`
	Manifest         model.Journey          `json:"manifest"`
	Previous         *model.JourneyRevision `json:"previous,omitempty"`
}

func (s *Service) ValidateJourney(ctx context.Context, j model.Journey) error {
	if err := j.Validate(); err != nil {
		return err
	}
	if s.Journeys == nil {
		return model.ErrUnavailable
	}
	for _, link := range j.Spec.SLOs {
		record, err := s.Store.Read(ctx, link.ID)
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				return model.Invalid("linked SLO %s does not exist; create it before applying the journey", link.ID)
			}
			return err
		}
		if record.SLO.Retired() {
			return model.Invalid("linked SLO %s is archived", link.ID)
		}
	}
	return nil
}
func (s *Service) PlanJourney(ctx context.Context, j model.Journey) (JourneyPlan, error) {
	p := JourneyPlan{Action: "create", Digest: model.JourneyDigest(j), Manifest: j}
	if err := s.ValidateJourney(ctx, j); err != nil {
		return p, err
	}
	old, err := s.Journeys.JourneyRevision(ctx, j.Metadata.Name, 0)
	if errors.Is(err, model.ErrJourneyNotFound) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.Previous = &old
	p.ExpectedRevision = old.Revision
	p.Action = "update"
	if old.Digest == p.Digest {
		p.Action = "unchanged"
	}
	return p, nil
}
func (s *Service) ApplyJourney(ctx context.Context, j model.Journey, expected int64) (model.JourneyRevision, error) {
	if expected < 0 {
		return model.JourneyRevision{}, model.Invalid("expectedRevision must be nonnegative")
	}
	if err := s.ValidateJourney(ctx, j); err != nil {
		return model.JourneyRevision{}, err
	}
	return s.Journeys.ApplyJourney(ctx, model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: s.Now().UTC()}, expected)
}
func SLOHealth(x model.Snapshot) string {
	if x.Alert.Phase == "firing" || (x.BudgetRemainingFraction != nil && *x.BudgetRemainingFraction <= 0) {
		return "degraded"
	}
	if x.Reliability == nil || x.Coverage < 1 {
		return "unknown"
	}
	if x.Alert.Phase == "pending" || (x.BurnRate != nil && *x.BurnRate > 1) {
		return "at_risk"
	}
	return "healthy"
}
func (s *Service) JourneyView(ctx context.Context, r model.JourneyRevision) (JourneyView, error) {
	v := JourneyView{Resource: r, Health: "healthy", Links: []JourneyLinkStatus{}, Reasons: []string{}}
	rank := map[string]int{"healthy": 0, "at_risk": 1, "unknown": 2, "degraded": 3}
	// The same requested wall-clock time is used for all linked objectives. Each
	// objective retains its own collection delay and interval alignment.
	for _, link := range r.Manifest.Spec.SLOs {
		rec, err := s.Store.Read(ctx, link.ID)
		if err != nil {
			return v, err
		}
		status, err := s.statusFromRecord(rec, time.Time{})
		if err != nil {
			return v, err
		}
		health := SLOHealth(status)
		v.Links = append(v.Links, JourneyLinkStatus{link, rec.SLO, status, health})
		if link.Required && health != "healthy" {
			v.Reasons = append(v.Reasons, fmt.Sprintf("%s: %s", link.ID, health))
			if rank[health] > rank[v.Health] {
				v.Health = health
			}
		}
	}
	return v, nil
}
func (s *Service) ListJourneys(ctx context.Context) ([]JourneyView, error) {
	if s.Journeys == nil {
		return nil, model.ErrUnavailable
	}
	resources, err := s.Journeys.Journeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]JourneyView, 0, len(resources))
	for _, r := range resources {
		v, err := s.JourneyView(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) RecordRun(ctx context.Context, id string, e model.RunEvidence) (model.JourneyRun, error) {
	if s.Journeys == nil {
		return model.JourneyRun{}, model.ErrUnavailable
	}
	if e.JourneyRevision < 1 {
		return model.JourneyRun{}, model.Invalid("journeyRevision must be explicit and positive")
	}
	rev, err := s.Journeys.JourneyRevision(ctx, id, e.JourneyRevision)
	if err != nil {
		return model.JourneyRun{}, err
	}
	e.StartedAt = e.StartedAt.UTC()
	e.FinishedAt = e.FinishedAt.UTC()
	r, err := model.EvaluateRun(id, rev, e, s.Now())
	if err != nil {
		return r, err
	}
	return s.Journeys.PutRun(ctx, r)
}
