package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestIncidentRoutesRolesAndGate(t *testing.T) {
	for _, gated := range []bool{true, false} {
		gates := features.All()
		if !gated {
			gates, _ = features.Parse("none")
		}
		svc := service.New(store.NewMemory(), nil)
		svc.IncidentsEnabled = gated
		grants := []tenancy.Grant{{Token: "editor", Role: "editor"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}
		h, err := NewMulti([]Workspace{{Features: gates, ID: "alpha", Name: "Alpha", Service: svc, Grants: grants}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		call := func(token, method, path, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		if !gated {
			if w := call("editor", "GET", "/api/incidents", ""); w.Code != 404 || !strings.Contains(w.Body.String(), "feature_disabled") {
				t.Fatal("incidents served with the gate off", w.Code)
			}
			continue
		}
		for _, token := range []string{"agent", "viewer"} {
			if w := call(token, "POST", "/api/incidents", `{"title":"x"}`); w.Code != 403 {
				t.Fatalf("%s declared an incident: %d", token, w.Code)
			}
		}
		w := call("editor", "POST", "/api/incidents", `{"title":"Checkout errors"}`)
		var declared service.IncidentView
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &declared) != nil || declared.DeclaredBy == "" {
			t.Fatal(w.Code, w.Body.String())
		}
		if w := call("agent", "POST", "/api/incidents/"+declared.ID+"/transition", `{"state":"resolved","expectedRevision":1}`); w.Code != 403 {
			t.Fatal("an agent resolved an incident", w.Code)
		}
		if w := call("editor", "POST", "/api/incidents/"+declared.ID+"/transition", `{"state":"resolved"}`); w.Code != 400 {
			t.Fatal("transition without expectedRevision accepted", w.Code)
		}
		if w := call("editor", "POST", "/api/incidents/"+declared.ID+"/transition", `{"state":"mitigated","expectedRevision":1}`); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if w := call("editor", "POST", "/api/incidents/"+declared.ID+"/transition", `{"state":"resolved","expectedRevision":1}`); w.Code != 409 {
			t.Fatal("stale transition accepted", w.Code)
		}
		if w := call("viewer", "GET", "/api/incidents/"+declared.ID, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"mitigated"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if w := call("viewer", "GET", "/api/incidents?limit=0", ""); w.Code != 400 {
			t.Fatal("bad limit accepted", w.Code)
		}
	}
}

func TestIncidentAlertIntakeAcceptsAlertmanager(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	svc.IncidentsEnabled = true
	grants := []tenancy.Grant{{Token: "alertmanager", Role: "editor"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}
	h, err := NewMulti([]Workspace{{Features: features.All(), ID: "alpha", Name: "Alpha", Service: svc, Grants: grants}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	post := func(token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/incidents/alerts", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	webhook := `{"version":"4","groupKey":"{}:{alertname=\"PostgresDown\"}","status":"firing","receiver":"nines","groupLabels":{"alertname":"PostgresDown"},"commonLabels":{"alertname":"PostgresDown","severity":"critical","nines_journey":"purchase"},"commonAnnotations":{"summary":"Postgres primary is down"},"externalURL":"https://alertmanager.example","alerts":[{"status":"firing","labels":{},"annotations":{},"startsAt":"2026-11-22T12:00:00Z","generatorURL":"https://prometheus.example/graph"}]}`
	for _, token := range []string{"agent", "viewer"} {
		if w := post(token, webhook); w.Code != 403 {
			t.Fatalf("%s sent alerts: %d", token, w.Code)
		}
	}
	w := post("alertmanager", webhook)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"outcome":"opened"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	list, err := svc.ListIncidents(t.Context(), 10)
	if err != nil || len(list) != 1 || list[0].Title != "Postgres primary is down" || list[0].Signals[0].URL != "https://alertmanager.example" || list[0].DetectedBySLO {
		t.Fatal(list, err)
	}
	generic := `{"signals":[{"source":"pagerduty","key":"P123","status":"firing","title":"Payments degraded"}]}`
	if w := post("alertmanager", generic); w.Code != 200 || !strings.Contains(w.Body.String(), `"outcome":"opened"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, bad := range []string{`not json`, `{"signals":[{"source":"x","key":"k","status":"maybe"}]}`, `{"signals":[{"source":"x","key":"k","status":"firing","extra":1}]}`, `{"version":"4","alerts":[]}`} {
		if w := post("alertmanager", bad); w.Code != 400 {
			t.Fatalf("%s accepted: %d", bad, w.Code)
		}
	}
}
