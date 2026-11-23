package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

// IncidentView is an incident with its impact, computed on read from stored
// samples with the same arithmetic as SLO status.
type IncidentView struct {
	model.Incident
	// DetectedBySLO is false when no SLO alert fired for the incident: Nines
	// could not see it coming, and an SLO that would have is worth defining.
	DetectedBySLO bool             `json:"detectedBySLO"`
	Impact        []IncidentImpact `json:"impact"`
	// PostmortemRequirement is set when an affected SLO's error budget policy
	// requires a postmortem above some budget share.
	PostmortemRequirement *PostmortemRequirement `json:"postmortemRequirement,omitempty"`
}

// PostmortemRequirement says whether the error budget policy requires a
// postmortem for an incident, and why. Required means some SLO consumed more
// than its policy's postmortemAbove; at exactly the threshold none is
// required. SLOs whose consumption is unknown are listed, never assumed.
type PostmortemRequirement struct {
	Required  bool               `json:"required"`
	Reasons   []PostmortemReason `json:"reasons"`
	Unknown   []string           `json:"unknown,omitempty"`
	Published bool               `json:"published"`
}

// PostmortemReason is one SLO whose budget consumption crossed its policy.
type PostmortemReason struct {
	SLOID     string  `json:"sloId"`
	Consumed  float64 `json:"consumed"`
	Threshold float64 `json:"threshold"`
}

// IncidentImpact is the error budget one incident consumed on one SLO.
// BudgetConsumedFraction is nil, with Reason, whenever it cannot be known:
// missing data stays unknown, never zero.
type IncidentImpact struct {
	SLOID string `json:"sloId"`
	// ImpactStartedAt is when users started to be affected: the start of the
	// run of intervals burning budget faster than the SLO allows that led to
	// the alert. It is usually before the alert fired.
	ImpactStartedAt *time.Time `json:"impactStartedAt,omitempty"`
	From            time.Time  `json:"from"`
	To              time.Time  `json:"to"`
	BadEvents       *float64   `json:"badEvents"`
	// BadEventsAtLeast marks BadEvents as a lower bound: some intervals have
	// no data, and they can only add bad events.
	BadEventsAtLeast       bool     `json:"badEventsAtLeast,omitempty"`
	BudgetConsumedFraction *float64 `json:"budgetConsumedFraction"`
	Coverage               float64  `json:"coverage"`
	Reason                 string   `json:"reason,omitempty"`
}

// incidentNow is the clock for incident records, in whole seconds like the
// rest of Nines' timestamps.
func (s *Service) incidentNow() time.Time { return s.Now().UTC().Truncate(time.Second) }

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
	now := s.incidentNow()
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
	now := s.incidentNow()
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
			if err := i.Transition(state, actor, note, s.incidentNow()); err != nil {
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
	view := IncidentView{Incident: i, DetectedBySLO: i.DetectedBySLO(), Impact: []IncidentImpact{}}
	for _, entry := range i.SLOs {
		view.Impact = append(view.Impact, s.incidentImpact(ctx, entry, i.OpenedAt, impactEnd(i, entry, s.incidentNow())))
	}
	view.PostmortemRequirement = s.postmortemRequirement(ctx, i, view.Impact)
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
	if began := impactStart(r, entry); began != nil {
		out.ImpactStartedAt = began
		if began.Before(start) {
			start = *began
		}
	}
	span := int64(at.Sub(start) / time.Second)
	if span < r.SLO.StepSeconds {
		span = r.SLO.StepSeconds
	}
	incident := engine.Evaluate(r.SLO, r.Points, at, span)
	window := engine.Evaluate(r.SLO, r.Points, at, r.SLO.WindowSeconds)
	out.From, out.To, out.Coverage = incident.From, incident.At, incident.Coverage
	bad := incident.Bad
	if incident.ObservedIntervals < incident.ExpectedIntervals {
		// Missing intervals can only add bad events, so the observed count is
		// a floor. The budget fraction is not: missing traffic also shrinks
		// the budget, so it stays unknown.
		if incident.ObservedIntervals > 0 {
			out.BadEvents, out.BadEventsAtLeast = &bad, true
		}
		out.Reason = fmt.Sprintf("%d of %d intervals during the incident have no data", incident.ExpectedIntervals-incident.ObservedIntervals, incident.ExpectedIntervals)
		return out
	}
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

// impactStart walks back from when the critical alert fired through
// consecutive intervals whose error ratio exceeded the SLO's allowance,
// at most the alert's long window. A gap in the data stops the walk, so the
// start is never earlier than the evidence shows.
func impactStart(r model.Record, entry model.IncidentSLO) *time.Time {
	if entry.FiringSince == nil || r.SLO.StepSeconds <= 0 {
		return nil
	}
	lookback := int64(0)
	for _, p := range r.SLO.AlertPolicies() {
		if p.Name == entry.Policy && p.LongWindowSeconds > lookback {
			lookback = p.LongWindowSeconds
		}
	}
	if lookback == 0 {
		return nil
	}
	allowed := 1 - r.SLO.Target
	step := r.SLO.StepSeconds
	end := entry.FiringSince.Unix() - entry.FiringSince.Unix()%step
	var earliest int64
	for t := end; t > end-lookback; t -= step {
		p, ok := r.Points[t]
		if !ok || p.Total <= 0 {
			break
		}
		good, total := p.Good, p.Total
		if r.SLO.BadInterval != nil {
			good, total = model.ClassifyInterval(good, total, *r.SLO.BadInterval)
		}
		if total <= 0 || (total-good)/total <= allowed {
			break
		}
		earliest = t
	}
	if earliest == 0 {
		return nil
	}
	// A point at t covers the interval ending at t.
	began := time.Unix(earliest-step, 0).UTC()
	return &began
}

// postmortemRequirement applies each affected SLO's error budget policy, at
// the definition revision in force during the incident, to its impact.
func (s *Service) postmortemRequirement(ctx context.Context, i model.Incident, impacts []IncidentImpact) *PostmortemRequirement {
	var req *PostmortemRequirement
	for n, entry := range i.SLOs {
		r, err := s.Store.Read(ctx, entry.ID)
		if err != nil {
			continue
		}
		policy := r.SLO.ErrorBudgetPolicy
		if policy == nil || policy.PostmortemAbove <= 0 {
			continue
		}
		if req == nil {
			req = &PostmortemRequirement{Reasons: []PostmortemReason{}}
			if i.Postmortem != nil {
				req.Published = i.Postmortem.Published
			}
		}
		consumed := impacts[n].BudgetConsumedFraction
		switch {
		// The definition, and perhaps its policy, changed after the incident:
		// whether a postmortem was required is unknown, not "not required".
		case model.ConfigRevision(r.SLO) != entry.ConfigRevision, consumed == nil:
			req.Unknown = append(req.Unknown, entry.ID)
		// A relative tolerance keeps "exactly at the threshold" from flipping
		// on floating-point error in the budget arithmetic.
		case *consumed > policy.PostmortemAbove*(1+1e-9):
			req.Required = true
			req.Reasons = append(req.Reasons, PostmortemReason{SLOID: entry.ID, Consumed: *consumed, Threshold: policy.PostmortemAbove})
		}
	}
	return req
}

// RecordPostmortem links an incident to the review holding its postmortem.
// Once published it stays published.
func (s *Service) RecordPostmortem(ctx context.Context, incident, review string, published bool) error {
	st, err := s.incidents()
	if err != nil {
		return err
	}
	return st.UpdateIncident(ctx, incident, func(i *model.Incident) error {
		was := i.Postmortem != nil && i.Postmortem.Published
		i.Postmortem = &model.IncidentPostmortem{ReviewID: review, Published: published || was, SavedAt: s.incidentNow()}
		if published && !was {
			i.Timeline = append(i.Timeline, model.IncidentEvent{Time: s.incidentNow(), Kind: "postmortem_published", Message: "The postmortem was published."})
		}
		i.Revision++
		return nil
	})
}

// RequiredPostmortems lists recent incidents on an SLO that need a published
// postmortem, for the SLO's budget decision.
func (s *Service) RequiredPostmortems(ctx context.Context, slo string) ([]v1.RequiredPostmortem, error) {
	st, err := s.incidents()
	if err != nil || !s.IncidentsEnabled {
		return nil, err
	}
	list, err := st.Incidents(ctx, 200)
	if err != nil {
		return nil, err
	}
	out := []v1.RequiredPostmortem{}
	for _, i := range list {
		if !i.HasSLO(slo) || i.Postmortem != nil && i.Postmortem.Published {
			continue
		}
		v := s.incidentView(ctx, i)
		if v.PostmortemRequirement == nil {
			continue
		}
		for _, r := range v.PostmortemRequirement.Reasons {
			if r.SLOID == slo {
				out = append(out, v1.RequiredPostmortem{IncidentID: i.ID, Title: i.Title, Consumed: r.Consumed, Threshold: r.Threshold})
			}
		}
	}
	return out, nil
}

// withRequiredPostmortems adds the incidents awaiting a required postmortem
// to an SLO's budget decision, when its policy sets postmortemAbove.
func (s *Service) withRequiredPostmortems(ctx context.Context, c model.SLO, d *v1.ErrorBudgetDecision) {
	if !s.IncidentsEnabled || c.ErrorBudgetPolicy == nil || c.ErrorBudgetPolicy.PostmortemAbove <= 0 {
		return
	}
	if required, err := s.RequiredPostmortems(ctx, c.ID); err == nil && len(required) > 0 {
		d.RequiredPostmortems = required
	}
}

// impactEnd is when an incident stopped affecting one SLO: the earliest of
// its alert clearing, the incident being marked mitigated or resolved, and
// now. An incident left open after recovery therefore does not keep
// collecting unrelated bad events, or lose its impact to later data gaps.
func impactEnd(i model.Incident, entry model.IncidentSLO, now time.Time) time.Time {
	end := now
	for _, t := range []*time.Time{i.MitigatedAt, i.ResolvedAt} {
		if t != nil && t.Before(end) {
			end = *t
		}
	}
	if !entry.Firing && entry.ClearedAt != nil && entry.ClearedAt.Before(end) {
		end = *entry.ClearedAt
	}
	return end
}
