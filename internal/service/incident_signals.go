package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ishantanu/nines/internal/model"
)

// Signal is one alert from another tool, normalized from its webhook.
type Signal struct {
	Source string
	// Key identifies the alert across repeated notifications.
	Key       string
	Firing    bool
	Title     string
	URL       string
	StartedAt time.Time
	// Severity from the sender; warning, info and none never open incidents.
	Severity string
	SLOs     []string
	Journeys []string
}

// SignalResult says what each signal did, so a sender can be debugged.
type SignalResult struct {
	Key      string `json:"key"`
	Outcome  string `json:"outcome"`
	Incident string `json:"incident,omitempty"`
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	for !utf8.ValidString(s[:n]) {
		n--
	}
	return s[:n]
}

// SafeLink keeps a link only if it is http(s) without embedded credentials.
func SafeLink(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(raw) > 2048 {
		return ""
	}
	return u.String()
}

var toolNames = map[string]string{"alertmanager": "Alertmanager", "pagerduty": "PagerDuty", "incident.io": "incident.io", "rootly": "Rootly", "opsgenie": "Opsgenie", "grafana": "Grafana"}

// toolName is how a sending tool is named on the timeline.
func toolName(source string) string {
	if name, ok := toolNames[strings.ToLower(source)]; ok {
		return name
	}
	return source
}

var quietSeverities = map[string]bool{"warning": true, "info": true, "none": true}

// IngestSignals applies alerts from other tools to incidents. A firing signal
// joins the open incident that already has it, or shares a named SLO or
// journey, or opens a new incident; repeated notifications change nothing.
// A resolved signal is recorded, and when nothing fires any more the
// incident moves to monitoring, as with SLO alerts.
func (s *Service) IngestSignals(ctx context.Context, actor string, signals []Signal) ([]SignalResult, error) {
	st, err := s.incidents()
	if err != nil {
		return nil, err
	}
	if len(signals) == 0 || len(signals) > 100 {
		return nil, model.Invalid("send 1 to 100 alerts")
	}
	now := s.incidentNow()
	// Resolve SLOs and journeys before the incident transaction.
	type resolved struct {
		slos     []model.IncidentSLO
		journeys []model.IncidentJourney
	}
	lookups := make([]resolved, len(signals))
	for n, sig := range signals {
		if sig.Key == "" || len(sig.Key) > 512 {
			return nil, model.Invalid("each alert needs a key of at most 512 bytes")
		}
		var r resolved
		for _, id := range sig.SLOs {
			rec, err := s.Store.Read(ctx, id)
			if err != nil {
				continue // An unknown SLO label must not block the alert.
			}
			firing, since, policy := criticalFiring(rec)
			r.slos = append(r.slos, model.IncidentSLO{ID: id, ConfigRevision: model.ConfigRevision(rec.SLO), Policy: policy, FiringSince: since, Firing: firing})
			journeys, err := s.journeysFor(ctx, id)
			if err != nil {
				return nil, err
			}
			r.journeys = append(r.journeys, journeys...)
		}
		if len(sig.Journeys) > 0 && s.Journeys != nil {
			all, err := s.Journeys.Journeys(ctx)
			if err != nil {
				return nil, err
			}
			for _, j := range all {
				for _, want := range sig.Journeys {
					if j.Manifest.Metadata.Name == want {
						r.journeys = append(r.journeys, model.IncidentJourney{ID: want, Revision: j.Revision})
					}
				}
			}
		}
		lookups[n] = r
	}
	results := make([]SignalResult, len(signals))
	err = st.UpdateIncidents(ctx, func(open []model.Incident) ([]model.Incident, error) {
		changed := map[string]bool{}
		order := []string{}
		touch := func(n int) {
			if !changed[open[n].ID] {
				changed[open[n].ID] = true
				order = append(order, open[n].ID)
			}
		}
		for n, sig := range signals {
			r := lookups[n]
			sig.Title = clip(sig.Title, 200)
			if sig.Title == "" {
				sig.Title = clip(sig.Key, 200)
			}
			sig.URL = SafeLink(sig.URL)
			started := sig.StartedAt.UTC().Truncate(time.Second)
			if started.IsZero() || started.After(now) || started.Before(now.Add(-7*24*time.Hour)) {
				started = now
			}
			results[n] = SignalResult{Key: sig.Key}
			// The incident already holding this signal.
			found := -1
			for k := range open {
				for _, x := range open[k].Signals {
					if x.Source == sig.Source && x.Key == sig.Key {
						found = k
					}
				}
			}
			if found >= 0 {
				i := &open[found]
				results[n].Incident = i.ID
				for m := range i.Signals {
					x := &i.Signals[m]
					if x.Source != sig.Source || x.Key != sig.Key || x.Firing == sig.Firing {
						continue
					}
					x.Firing = sig.Firing
					if sig.Firing {
						x.FiringSince, x.ClearedAt = &started, nil
						i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "signal_fired", Actor: actor, Message: fmt.Sprintf("The %s alert %q fired again.", toolName(sig.Source), x.Title)})
						if i.State == model.IncidentMonitoring {
							i.State = model.IncidentDetected
							i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "state_changed", Message: "Back to detected because an alert fired again while monitoring."})
						}
					} else {
						x.ClearedAt = &now
						i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "signal_cleared", Actor: actor, Message: fmt.Sprintf("The %s alert %q resolved.", toolName(sig.Source), x.Title)})
						if !i.Firing() && (i.State == model.IncidentDetected || i.State == model.IncidentMitigated) {
							i.State = model.IncidentMonitoring
							i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "state_changed", Message: "Monitoring because every alert has cleared. Resolve the incident once the service is stable."})
						}
					}
					touch(found)
				}
				results[n].Outcome = "updated"
				if !changed[i.ID] {
					results[n].Outcome = "unchanged"
				}
				continue
			}
			if !sig.Firing {
				results[n].Outcome = "ignored: resolved alert with no open incident"
				continue
			}
			if quietSeverities[strings.ToLower(sig.Severity)] {
				results[n].Outcome = "ignored: severity " + sig.Severity + " does not open incidents"
				continue
			}
			entry := model.IncidentSignal{Source: sig.Source, Key: sig.Key, Title: sig.Title, URL: sig.URL, FiringSince: &started, Firing: true}
			joined := -1
			for k := range open {
				shares := sharesJourney(open[k], r.journeys)
				for _, slo := range r.slos {
					shares = shares || open[k].HasSLO(slo.ID)
				}
				if shares {
					joined = k
					break
				}
			}
			if joined >= 0 {
				i := &open[joined]
				i.Signals = append(i.Signals, entry)
				for _, slo := range r.slos {
					if !i.HasSLO(slo.ID) {
						i.SLOs = append(i.SLOs, slo)
					}
				}
				addJourneys(i, r.journeys)
				i.Timeline = append(i.Timeline, model.IncidentEvent{Time: now, Kind: "signal_joined", Actor: actor, Message: fmt.Sprintf("The %s alert %q joined; it names an SLO or journey in this incident.", toolName(sig.Source), sig.Title)})
				if i.State == model.IncidentMonitoring {
					i.State = model.IncidentDetected
				}
				touch(joined)
				results[n] = SignalResult{Key: sig.Key, Outcome: "joined", Incident: i.ID}
				continue
			}
			i := model.Incident{ID: model.NewID(), Title: sig.Title, Source: "external", State: model.IncidentDetected, DeclaredBy: actor, OpenedAt: started, SLOs: r.slos, Journeys: r.journeys, Signals: []model.IncidentSignal{entry}}
			if i.SLOs == nil {
				i.SLOs = []model.IncidentSLO{}
			}
			if i.Journeys == nil {
				i.Journeys = []model.IncidentJourney{}
			}
			i.Timeline = []model.IncidentEvent{{Time: started, Kind: "opened", Actor: actor, Message: fmt.Sprintf("Opened by the %s alert %q.", toolName(sig.Source), sig.Title)}}
			open = append(open, i)
			touch(len(open) - 1)
			results[n] = SignalResult{Key: sig.Key, Outcome: "opened", Incident: i.ID}
		}
		writes := []model.Incident{}
		for _, id := range order {
			for _, i := range open {
				if i.ID == id {
					i.Revision++
					writes = append(writes, i)
				}
			}
		}
		return writes, nil
	})
	return results, err
}
