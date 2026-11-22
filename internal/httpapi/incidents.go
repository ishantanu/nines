package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

func incidentActor(r *http.Request) string {
	p, ok := tenancy.FromContext(r.Context())
	if !ok {
		return ""
	}
	if p.Name != "" {
		return p.Name
	}
	return p.Subject
}

func (a *Server) incidentList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			a.fail(w, r, model.Invalid("limit must be 1 to 200"))
			return
		}
		limit = n
	}
	list, err := a.Service.ListIncidents(r.Context(), limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"incidents": list})
}

func (a *Server) incidentGet(w http.ResponseWriter, r *http.Request) {
	v, err := a.Service.Incident(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

func (a *Server) incidentDeclare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string   `json:"title"`
		SLOs  []string `json:"slos"`
	}
	if err := decode(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	i, err := a.Service.DeclareIncident(r.Context(), incidentActor(r), req.Title, req.SLOs)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Service.Incident(r.Context(), i.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

func (a *Server) incidentTransition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State            string `json:"state"`
		Note             string `json:"note"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decode(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.ExpectedRevision == nil {
		a.fail(w, r, model.Invalid("expectedRevision is required: use the incident's current revision"))
		return
	}
	i, err := a.Service.TransitionIncident(r.Context(), r.PathValue("id"), *req.ExpectedRevision, req.State, incidentActor(r), req.Note)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Service.Incident(r.Context(), i.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

// alertmanagerPayload is Alertmanager's webhook body (version 4).
type alertmanagerPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []struct {
		Status       string            `json:"status"`
		Labels       map[string]string `json:"labels"`
		Annotations  map[string]string `json:"annotations"`
		StartsAt     time.Time         `json:"startsAt"`
		GeneratorURL string            `json:"generatorURL"`
	} `json:"alerts"`
}

// genericSignal is the format for any other tool or script.
type genericSignal struct {
	Source    string    `json:"source"`
	Key       string    `json:"key"`
	Status    string    `json:"status"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	URL       string    `json:"url"`
	StartedAt time.Time `json:"startedAt"`
	SLOs      []string  `json:"slos"`
	Journeys  []string  `json:"journeys"`
}

func splitLabel(v string) []string {
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" && len(out) < 20 {
			out = append(out, part)
		}
	}
	return out
}

func signalKey(key string) string {
	if len(key) <= 512 {
		return key
	}
	sum := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// parseSignals accepts an Alertmanager webhook as is, or {"signals": [...]}.
func parseSignals(body []byte) ([]service.Signal, error) {
	var probe struct {
		Version string          `json:"version"`
		Signals json.RawMessage `json:"signals"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, model.Invalid("send JSON: an Alertmanager webhook, or {\"signals\": [...]}")
	}
	if probe.Version != "" {
		var p alertmanagerPayload
		if err := json.Unmarshal(body, &p); err != nil || p.GroupKey == "" || len(p.Alerts) == 0 || len(p.Alerts) > 1000 {
			return nil, model.Invalid("not a valid Alertmanager webhook")
		}
		sig := service.Signal{Source: "alertmanager", Key: signalKey(p.GroupKey), Firing: p.Status == "firing", Severity: p.CommonLabels["severity"], URL: p.ExternalURL}
		sig.Title = p.CommonAnnotations["summary"]
		if sig.Title == "" {
			sig.Title = p.CommonLabels["alertname"]
		}
		for _, a := range p.Alerts {
			if sig.StartedAt.IsZero() || (!a.StartsAt.IsZero() && a.StartsAt.Before(sig.StartedAt)) {
				sig.StartedAt = a.StartsAt
			}
			if sig.URL == "" {
				sig.URL = a.GeneratorURL
			}
		}
		sig.SLOs = splitLabel(p.CommonLabels["nines_slo"])
		sig.Journeys = splitLabel(p.CommonLabels["nines_journey"])
		return []service.Signal{sig}, nil
	}
	var g struct {
		Signals []genericSignal `json:"signals"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&g); err != nil || len(g.Signals) == 0 {
		return nil, model.Invalid("send {\"signals\": [{\"source\", \"key\", \"status\": \"firing\" or \"resolved\", \"title\", ...}]}")
	}
	out := []service.Signal{}
	for _, s := range g.Signals {
		if s.Status != "firing" && s.Status != "resolved" {
			return nil, model.Invalid("status must be firing or resolved")
		}
		if s.Source == "" || len(s.Source) > 64 {
			return nil, model.Invalid("source must name the sending tool in at most 64 bytes")
		}
		out = append(out, service.Signal{Source: s.Source, Key: signalKey(s.Key), Firing: s.Status == "firing", Title: s.Title, Severity: s.Severity, URL: s.URL, StartedAt: s.StartedAt, SLOs: s.SLOs, Journeys: s.Journeys})
	}
	return out, nil
}

// incidentAlerts receives alerts from other tools. It needs a token with the
// editor role; give each sending tool its own.
func (a *Server) incidentAlerts(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.fail(w, r, model.Invalid("send at most 1 MiB"))
		return
	}
	signals, err := parseSignals(body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	results, err := a.Service.IngestSignals(r.Context(), incidentActor(r), signals)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"results": results})
}
