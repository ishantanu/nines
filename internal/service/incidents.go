package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

// IncidentView is an incident with its impact, computed on read from stored
// samples with the same arithmetic as SLO status.
type IncidentView struct {
	model.Incident
	Impact []IncidentImpact `json:"impact"`
}

// IncidentImpact is the error budget one incident consumed on one SLO.
// BudgetConsumedFraction is nil, with Reason, whenever it cannot be known:
// missing data stays unknown, never zero.
type IncidentImpact struct {
	SLOID                  string    `json:"sloId"`
	From                   time.Time `json:"from"`
	To                     time.Time `json:"to"`
	BadEvents              *float64  `json:"badEvents"`
	BudgetConsumedFraction *float64  `json:"budgetConsumedFraction"`
	Coverage               float64   `json:"coverage"`
	Reason                 string    `json:"reason,omitempty"`
}

func (s *Service) incidents() (store.IncidentStore, error) {
	st, ok := s.Store.(store.IncidentStore)
	if !ok {
		return nil, model.ErrUnavailable
	}
	return st, nil
}

// criticalFiring reports whether an SLO's critical alert fires, since when,
// and under which policy. A single unnamed policy pages, so it counts.
func criticalFiring(r model.Record) (bool, *time.Time, string) {
	if len(r.SLO.Policies) == 0 {
		if r.SLO.Policy != nil && r.Alert.Phase == "firing" && (r.SLO.Policy.Severity == "" || r.SLO.Policy.Severity == "critical") {
			return true, r.Alert.FiringSince, r.SLO.Policy.Name
		}
		return false, nil, ""
	}
	var since *time.Time
	policy, firing := "", false
	for _, p := range r.SLO.Policies {
		state := r.Alert.Policies[p.Name]
		if p.Severity != "critical" || state.Phase != "firing" {
			continue
		}
		if !firing || state.FiringSince != nil && (since == nil || state.FiringSince.Before(*since)) {
			since, policy = state.FiringSince, p.Name
		}
		firing = true
	}
	return firing, since, policy
}

// journeysFor returns the journeys that link an SLO, at their current
// revisions.
func (s *Service) journeysFor(ctx context.Context, id string) ([]model.IncidentJourney, error) {
	out := []model.IncidentJourney{}
	if s.Journeys == nil {
		return out, nil
	}
	revisions, err := s.Journeys.Journeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, j := range revisions {
		for _, link := range j.Manifest.Spec.SLOs {
			if link.ID == id {
				out = append(out, model.IncidentJourney{ID: j.Manifest.Metadata.Name, Revision: j.Revision})
				break
			}
		}
	}
	return out, nil
}

func addJourneys(i *model.Incident, journeys []model.IncidentJourney) {
	for _, j := range journeys {
		if !slices.ContainsFunc(i.Journeys, func(x model.IncidentJourney) bool { return x.ID == j.ID }) {
			i.Journeys = append(i.Journeys, j)
		}
	}
}

func sharesJourney(i model.Incident, journeys []model.IncidentJourney) bool {
	for _, j := range journeys {
		if slices.ContainsFunc(i.Journeys, func(x model.IncidentJourney) bool { return x.ID == j.ID }) {
			return true
		}
	}
	return false
}

// ReconcileIncidents brings incidents in line with one SLO's critical alert.
// It is level-triggered, so it is safe to repeat and catches up after a
// restart: a firing alert joins the open incident that already has the SLO,
// or one sharing a critical user journey, or opens a new incident; a cleared
// alert is recorded, and when nothing fires any more the incident moves to
// monitoring. A resolved incident is never reopened; a later burn opens a
// new one.
func (s *Service) ReconcileIncidents(ctx context.Context, id string) error {
	st, err := s.incidents()
	if err != nil {
		return err
	}
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return err
	}
	firing, since, policy := criticalFiring(r)
	if r.SLO.Retired() {
		firing = false
	}
	journeys, err := s.journeysFor(ctx, id)
	if err != nil {
		return err
	}
	now := s.Now().UTC()
	return st.UpdateIncidents(ctx, func(open []model.Incident) ([]model.Incident, error) {
		for _, i := range open {
			if !i.HasSLO(id) {
				continue
			}
			for n := range i.SLOs {
				entry := &i.SLOs[n]
				if entry.ID != id || entry.Firing == firing {
					continue
				}
				entry.Firing = firing
				if firing {
					entry.FiringSince, entry.ClearedAt, entry.Policy = since, nil, policy
					i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "alert_fired", SLOID: id, Message: fmt.Sprintf("The critical burn alert on %s fired again.", id)})
					if i.State == model.IncidentMonitoring {
						i.State = model.IncidentDetected
						i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "state_changed", Message: "Back to detected because an alert fired again while monitoring."})
					}
				} else {
					entry.ClearedAt = &now
					i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "alert_cleared", SLOID: id, Message: fmt.Sprintf("The critical burn alert on %s cleared.", id)})
					if !i.Firing() && (i.State == model.IncidentDetected || i.State == model.IncidentMitigated) {
						i.State = model.IncidentMonitoring
						i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "state_changed", Message: "Monitoring because every critical burn alert has cleared. Resolve the incident once the service is stable."})
					}
				}
				i.Revision++
				return []model.Incident{i}, nil
			}
			return nil, nil
		}
		if !firing {
			return nil, nil
		}
		opened := now
		if since != nil {
			opened = *since
		}
		entry := model.IncidentSLO{ID: id, ConfigRevision: model.ConfigRevision(r.SLO), Policy: policy, FiringSince: since, Firing: true}
		for _, i := range open {
			if sharesJourney(i, journeys) {
				i.SLOs = append(i.SLOs, entry)
				addJourneys(&i, journeys)
				i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "slo_joined", SLOID: id, Message: fmt.Sprintf("The critical burn alert on %s fired; it shares a critical user journey with this incident.", id)})
				if i.State == model.IncidentMonitoring {
					i.State = model.IncidentDetected
				}
				i.Revision++
				return []model.Incident{i}, nil
			}
		}
		name := r.SLO.Name
		if name == "" {
			name = id
		}
		i := model.Incident{ID: model.NewID(), Revision: 1, Title: name + " is burning its error budget", Source: "alert", State: model.IncidentDetected, OpenedAt: opened, SLOs: []model.IncidentSLO{entry}, Journeys: journeys}
		i.Timeline = []model.IncidentEvent{{Time: opened, Kind: "opened", SLOID: id, Message: fmt.Sprintf("Opened by the critical burn alert on %s.", id)}}
		return []model.Incident{i}, nil
	})
}

// DeclareIncident records an incident a person declares, optionally for some
// SLOs. Declaring early carries no penalty: alerts on those SLOs join it.
func (s *Service) DeclareIncident(ctx context.Context, actor, title string, sloIDs []string) (model.Incident, error) {
	st, err := s.incidents()
	if err != nil {
		return model.Incident{}, err
	}
	if err = model.ValidateIncidentTitle(title); err != nil {
		return model.Incident{}, err
	}
	if actor == "" || len(sloIDs) > 50 {
		return model.Incident{}, model.Invalid("declare an incident as a known person, for at most 50 SLOs")
	}
	now := s.Now().UTC()
	i := model.Incident{ID: model.NewID(), Revision: 1, Title: title, Source: "manual", State: model.IncidentDetected, DeclaredBy: actor, OpenedAt: now, SLOs: []model.IncidentSLO{}, Journeys: []model.IncidentJourney{}}
	for _, id := range sloIDs {
		if i.HasSLO(id) {
			continue
		}
		r, err := s.Store.Read(ctx, id)
		if err != nil {
			return model.Incident{}, err
		}
		firing, since, policy := criticalFiring(r)
		i.SLOs = append(i.SLOs, model.IncidentSLO{ID: id, ConfigRevision: model.ConfigRevision(r.SLO), Policy: policy, FiringSince: since, Firing: firing})
		journeys, err := s.journeysFor(ctx, id)
		if err != nil {
			return model.Incident{}, err
		}
		addJourneys(&i, journeys)
	}
	i.Timeline = []model.IncidentEvent{{Time: now, Kind: "opened", Actor: actor, Message: "Declared by " + actor + "."}}
	err = st.UpdateIncidents(ctx, func([]model.Incident) ([]model.Incident, error) { return []model.Incident{i}, nil })
	return i, err
}

// TransitionIncident moves an incident through the lifecycle at an expected
// revision, so two responders cannot overwrite each other's decision.
func (s *Service) TransitionIncident(ctx context.Context, id string, expected int64, state, actor, note string) (model.Incident, error) {
	st, err := s.incidents()
	if err != nil {
		return model.Incident{}, err
	}
	var out model.Incident
	found := false
	err = st.UpdateIncidents(ctx, func(open []model.Incident) ([]model.Incident, error) {
		for _, i := range open {
			if i.ID != id {
				continue
			}
			if i.Revision != expected {
				return nil, model.ErrRevisionConflict
			}
			if err := i.Transition(state, actor, note, s.Now().UTC()); err != nil {
				return nil, err
			}
			i.Revision++
			out, found = i, true
			return []model.Incident{i}, nil
		}
		return nil, nil
	})
	if err != nil || found {
		return out, err
	}
	// Not open: tell a resolved incident from a missing one, outside the
	// store's transaction.
	if _, err = st.Incident(ctx, id); err == nil {
		return out, model.Invalid("the incident is resolved; declare a new one")
	}
	return out, err
}

// Incident returns one incident with its impact.
func (s *Service) Incident(ctx context.Context, id string) (IncidentView, error) {
	st, err := s.incidents()
	if err != nil {
		return IncidentView{}, err
	}
	i, err := st.Incident(ctx, id)
	if err != nil {
		return IncidentView{}, err
	}
	return s.incidentView(ctx, i), nil
}

// ListIncidents returns the most recent incidents, newest first.
func (s *Service) ListIncidents(ctx context.Context, limit int) ([]IncidentView, error) {
	st, err := s.incidents()
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	list, err := st.Incidents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]IncidentView, 0, len(list))
	for _, i := range list {
		out = append(out, s.incidentView(ctx, i))
	}
	return out, nil
}

func (s *Service) incidentView(ctx context.Context, i model.Incident) IncidentView {
	end := s.Now().UTC()
	if i.ResolvedAt != nil {
		end = *i.ResolvedAt
	}
	view := IncidentView{Incident: i, Impact: []IncidentImpact{}}
	for _, entry := range i.SLOs {
		view.Impact = append(view.Impact, s.incidentImpact(ctx, entry, i.OpenedAt, end))
	}
	return view
}

// incidentImpact is the budget consumed on one SLO between from and to: the
// bad events in that span over the budget the SLO's window allowed at its
// end, both from model.Snapshot as status computes them. It is unknown when
// any interval in the span has no data, or the definition has changed.
func (s *Service) incidentImpact(ctx context.Context, entry model.IncidentSLO, from, to time.Time) IncidentImpact {
	out := IncidentImpact{SLOID: entry.ID, From: from, To: to}
	r, err := s.Store.Read(ctx, entry.ID)
	switch {
	case err != nil:
		out.Reason = "the SLO is no longer readable"
		return out
	case model.ConfigRevision(r.SLO) != entry.ConfigRevision:
		out.Reason = "the SLO's definition changed after the incident, so its samples are not comparable"
		return out
	case r.SLO.ProviderWindow != nil:
		out.Reason = "the provider evaluates this SLO's windows, so per-interval samples are not stored"
		return out
	}
	step := time.Duration(r.SLO.StepSeconds) * time.Second
	at := to.Truncate(step)
	start := from.Truncate(step)
	span := int64(at.Sub(start) / time.Second)
	if span < r.SLO.StepSeconds {
		span = r.SLO.StepSeconds
	}
	incident := engine.Evaluate(r.SLO, r.Points, at, span)
	window := engine.Evaluate(r.SLO, r.Points, at, r.SLO.WindowSeconds)
	out.From, out.To, out.Coverage = incident.From, incident.At, incident.Coverage
	if incident.ObservedIntervals < incident.ExpectedIntervals {
		out.Reason = fmt.Sprintf("%d of %d intervals during the incident have no data", incident.ExpectedIntervals-incident.ObservedIntervals, incident.ExpectedIntervals)
		return out
	}
	bad := incident.Bad
	out.BadEvents = &bad
	if window.BudgetAllowed == nil || *window.BudgetAllowed <= 0 {
		out.Reason = "the SLO's window had no traffic, so it allowed no error budget"
		return out
	}
	consumed := bad / *window.BudgetAllowed
	out.BudgetConsumedFraction = &consumed
	return out
}

// incidentAfterEvaluation keeps incidents level with an SLO's alerts after
// each evaluation. It is best effort: a failure is retried by the next
// evaluation, and never fails the evaluation itself.
func (s *Service) incidentAfterEvaluation(ctx context.Context, id string) {
	if !s.IncidentsEnabled {
		return
	}
	_ = s.ReconcileIncidents(ctx, id)
}
