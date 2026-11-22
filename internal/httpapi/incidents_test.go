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
