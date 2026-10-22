package rules

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestInspectionSeparatesDeploymentFromRuntimeEvidence(t *testing.T) {
	now := time.Now().UTC()
	s := fixture()
	var groups []runtimeGroup
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/rules" {
			t.Errorf("unexpected mutation or endpoint: %s %s", r.Method, r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"groups": groups}})
	}))
	defer backend.Close()
	m := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Kind: "prometheus", Connection: "production", QueryURL: backend.URL, Directory: t.TempDir(), Namespace: "nines"}}}
	p, err := m.Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	check := func(wantDeployment, wantEvaluation string) {
		t.Helper()
		d, e := m.Inspect(context.Background(), s, now)
		if d.State != wantDeployment || e.State != wantEvaluation {
			t.Fatalf("got %+v %+v, want %s %s", d, e, wantDeployment, wantEvaluation)
		}
	}
	check("not_deployed", "not_loaded")
	if _, err = m.Apply(context.Background(), s, p.Digest); err != nil {
		t.Fatal(err)
	}
	check("deployed", "not_loaded")
	g := runtimeGroup{Name: p.Group.Name, Interval: float64(s.StepSeconds)}
	for _, r := range p.Group.Rules {
		name, kind := r.Record, "recording"
		if r.Alert != "" {
			name, kind = r.Alert, "alerting"
		}
		seconds, _ := DurationSeconds(r.For)
		g.Rules = append(g.Rules, runtimeRule{Name: name, Type: kind, Query: r.Expr, Labels: r.Labels, Annotations: r.Annotations, Duration: float64(seconds), Health: "ok", LastEvaluation: now.Add(-time.Second)})
	}
	groups = []runtimeGroup{g}
	check("deployed", "evaluating")
	groups[0].Rules[0].Query = "vector(1)"
	check("deployed", "out_of_date")
	groups[0].Rules[0].Query = p.Group.Rules[0].Expr
	groups[0].Rules[0].LastError = "credential error with private content"
	check("deployed", "failed")
	_, e := m.Inspect(context.Background(), s, now)
	if e.Message == groups[0].Rules[0].LastError {
		t.Fatal("provider error leaked")
	}
	groups[0].Rules[0].LastError = ""
	groups[0].Rules[0].LastEvaluation = now.Add(-10 * time.Minute)
	check("deployed", "stale")
	groups[0].Rules[0].LastEvaluation = time.Time{}
	check("deployed", "pending")
	groups = append(groups, groups[0])
	check("deployed", "out_of_date")
}

func TestMimirInspectionPinsNamespaceCredentialsAndTenant(t *testing.T) {
	s := fixture()
	var group Group
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("X-Scope-OrgID") != "upstream" || r.Header.Get("Authorization") != "Bearer rules-secret" {
			t.Error("unscoped inspection")
		}
		switch r.URL.Path {
		case "/prometheus/config/v1/rules/nines/" + group.Name:
			_ = yaml.NewEncoder(w).Encode(group)
		case "/prometheus/api/v1/rules":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"groups": []any{}}})
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer backend.Close()
	m := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", RulerURL: backend.URL + "/prometheus", MimirOrgID: "upstream", Namespace: "nines", BearerToken: "rules-secret"}}}
	p, err := m.Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	group = p.Group
	d, e := m.Inspect(context.Background(), s, time.Now())
	if d.State != "deployed" || e.State != "not_loaded" {
		t.Fatal(d, e)
	}
}

func TestInspectionDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	m := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "prometheus", Directory: t.TempDir(), QueryURL: redirect.URL}}}
	_, e := m.Inspect(context.Background(), fixture(), time.Now())
	if calls != 0 || e.State != "unknown" {
		t.Fatal(calls, e)
	}
}
