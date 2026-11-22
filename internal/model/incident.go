package model

import (
	"strings"
	"time"
)

// Incident states follow the response lifecycle in Site Reliability
// Engineering (2nd edition, chapters 10-12).
const (
	IncidentDetected   = "detected"
	IncidentMitigated  = "mitigated"
	IncidentMonitoring = "monitoring"
	IncidentResolved   = "resolved"
)

// Incident is a record of user-visible reliability loss. Nines fills in what
// it already knows (alerts, affected SLOs and journeys, revisions in force);
// responders move it through the lifecycle. Impact is computed on read from
// stored samples, never stored, so it always uses the current arithmetic.
type Incident struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Title    string `json:"title"`
	// Source is "alert" when a critical burn alert opened it, "manual" when a
	// person declared it.
	Source      string            `json:"source"`
	State       string            `json:"state"`
	DeclaredBy  string            `json:"declaredBy,omitempty"`
	OpenedAt    time.Time         `json:"openedAt"`
	MitigatedAt *time.Time        `json:"mitigatedAt,omitempty"`
	ResolvedAt  *time.Time        `json:"resolvedAt,omitempty"`
	SLOs        []IncidentSLO     `json:"slos"`
	Journeys    []IncidentJourney `json:"journeys"`
	Timeline    []IncidentEvent   `json:"timeline"`
}

// IncidentSLO is an affected SLO with the definition revision in force when
// it joined the incident, and its critical alert's firing span.
type IncidentSLO struct {
	ID             string     `json:"id"`
	ConfigRevision int64      `json:"configRevision"`
	Policy         string     `json:"policy,omitempty"`
	FiringSince    *time.Time `json:"firingSince,omitempty"`
	ClearedAt      *time.Time `json:"clearedAt,omitempty"`
	Firing         bool       `json:"firing"`
}

// IncidentJourney is an affected critical user journey at its revision then.
type IncidentJourney struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

// IncidentEvent is one timeline entry. Actor is empty for Nines itself.
type IncidentEvent struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Actor   string    `json:"actor,omitempty"`
	SLOID   string    `json:"sloId,omitempty"`
	Message string    `json:"message"`
}

// Open reports whether the incident still accepts alerts and transitions.
func (i Incident) Open() bool { return i.State != IncidentResolved }

// HasSLO reports whether an SLO is already part of the incident.
func (i Incident) HasSLO(id string) bool {
	for _, s := range i.SLOs {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Firing reports whether any affected SLO's critical alert is firing.
func (i Incident) Firing() bool {
	for _, s := range i.SLOs {
		if s.Firing {
			return true
		}
	}
	return false
}

// incidentTransitions are the states a person may move an incident to.
// Monitoring is also entered automatically when every alert clears, and
// detected again when one fires.
var incidentTransitions = map[string][]string{
	IncidentDetected:   {IncidentMitigated, IncidentMonitoring, IncidentResolved},
	IncidentMitigated:  {IncidentDetected, IncidentMonitoring, IncidentResolved},
	IncidentMonitoring: {IncidentDetected, IncidentMitigated, IncidentResolved},
}

// Transition moves an incident to a new state as a person's decision.
func (i *Incident) Transition(state, actor, note string, at time.Time) error {
	allowed := false
	for _, s := range incidentTransitions[i.State] {
		allowed = allowed || s == state
	}
	if !allowed {
		return Invalid("an incident in state %s cannot move to %s", i.State, state)
	}
	if strings.TrimSpace(actor) == "" {
		return Invalid("a state change needs an actor")
	}
	if len(note) > 2000 {
		return Invalid("note must be at most 2000 characters")
	}
	message := "State changed from " + i.State + " to " + state + "."
	if note = strings.TrimSpace(note); note != "" {
		message += " " + note
	}
	i.State = state
	switch state {
	case IncidentMitigated:
		if i.MitigatedAt == nil {
			i.MitigatedAt = &at
		}
	case IncidentResolved:
		i.ResolvedAt = &at
	}
	i.Timeline = append(i.Timeline, IncidentEvent{Time: at, Kind: "state_changed", Actor: actor, Message: message})
	return nil
}

// ValidateIncidentTitle bounds a declared incident's title.
func ValidateIncidentTitle(title string) error {
	if n := len(strings.TrimSpace(title)); n == 0 || n > 200 {
		return Invalid("title must be 1 to 200 characters")
	}
	return nil
}
