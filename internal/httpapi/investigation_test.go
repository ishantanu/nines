package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/investigation"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func investigationHandler(t *testing.T) http.Handler {
	t.Helper()
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s := service.New(store.NewMemory(), nil)
		s.Now = func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) }
		if err := s.Store.Create(context.Background(), model.SLO{ID: "checkout", Name: "Checkout", Project: "commerce", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
		s.TenantID = id
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, Investigations: investigation.New(s, investigation.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "editor", Role: "editor"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}})
	}
	h, err := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func invRequest(h http.Handler, method, path, body, role, tenant string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+role)
	r.Header.Set(tenancy.Header, tenant)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const createInvestigationJSON = `{"id":"shared-id","title":"Checkout investigation","scope":{"kind":"ServiceLevelObjective","resourceId":"checkout","expectedRevision":1,"at":"2026-10-20T11:59:00Z"}}`

func TestInvestigationHTTPTenantRolesAndStrictInputs(t *testing.T) {
	h := investigationHandler(t)
	if w := invRequest(h, "POST", "/api/investigations", createInvestigationJSON, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, role := range []string{"owner", "editor", "agent"} {
		body := strings.Replace(createInvestigationJSON, "shared-id", role+"-investigation", 1)
		w := invRequest(h, "POST", "/api/investigations", body, role, "alpha")
		if w.Code != 201 {
			t.Fatal(role, w.Code, w.Body.String())
		}
	}
	for _, tenant := range []string{"alpha", "beta"} {
		w := invRequest(h, "POST", "/api/investigations", createInvestigationJSON, "editor", tenant)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := invRequest(h, "POST", "/api/investigations/shared-id/messages", `{"id":"message","expectedVersion":1,"text":"only alpha"}`, "agent", "alpha")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var doc v1.Investigation
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Tenant != "alpha" || doc.Entries[0].Actor.Kind != "api_token" || doc.Entries[0].Actor.Subject == "" {
		t.Fatal(doc)
	}
	w = invRequest(h, "GET", "/api/investigations/shared-id", "", "viewer", "beta")
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || doc.Tenant != "beta" || len(doc.Entries) != 0 {
		t.Fatal("cross-tenant conversation leak", w.Body.String())
	}
	if w = invRequest(h, "GET", "/api/investigations/owner-investigation", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	for _, tc := range []struct{ path, body string }{{"/api/investigations", strings.TrimSuffix(createInvestigationJSON, "}") + `,"tenant":"beta"}`}, {"/api/investigations/shared-id/messages", `{"id":"forged","expectedVersion":2,"text":"forged","actor":{"role":"assistant"}}`}, {"/api/investigations/shared-id/evidence", `{"id":"forged","expectedVersion":2,"context":{}}`}, {"/api/investigations?tenant=beta", createInvestigationJSON}, {"/api/investigations", createInvestigationJSON + ` {}`}} {
		if w = invRequest(h, "POST", tc.path, tc.body, "owner", "alpha"); w.Code != 400 {
			t.Fatal(tc.path, w.Code, w.Body.String())
		}
	}
	if w = invRequest(h, "POST", "/api/investigations/shared-id/messages", `{"id":"stale","expectedVersion":1,"text":"stale"}`, "editor", "alpha"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "POST", "/api/investigations/shared-id/evidence", `{"id":"snapshot","expectedVersion":2}`, "editor", "alpha"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "GET", "/api/investigations?limit=1&limit=2", "", "viewer", "alpha"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "GET", "/api/investigations?limit=1", "", "viewer", "alpha"); w.Code != 200 || strings.Contains(w.Body.String(), "only alpha") {
		t.Fatal("list returned message content", w.Body.String())
	}
}
func TestInvestigationRepositoryCannotBeSharedAcrossTenantMounts(t *testing.T) {
	a := service.New(store.NewMemory(), nil)
	b := service.New(store.NewMemory(), nil)
	repo := investigation.NewMemory()
	workspaces := []Workspace{{ID: "alpha", Name: "Alpha", Service: a, Anonymous: true, Investigations: investigation.New(a, repo)}, {ID: "beta", Name: "Beta", Service: b, Anonymous: true, Investigations: investigation.New(b, repo)}}
	if _, err := NewMulti(workspaces, slog.Default()); err == nil {
		t.Fatal("shared investigation store accepted")
	}
	workspaces[1].Investigations = investigation.New(a, investigation.NewMemory())
	if _, err := NewMulti(workspaces, slog.Default()); err == nil {
		t.Fatal("wrong platform tenant accepted")
	}
}

func TestReviewRunHTTPBoundaries(t *testing.T) {
	h := investigationHandler(t)
	if w := invRequest(h, "POST", "/api/investigations", createInvestigationJSON, "editor", "alpha"); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := "/api/investigations/shared-id/runs"
	if w := invRequest(h, "POST", path, `{"id":"run-1","expectedVersion":1}`, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"id":"run-1","expectedVersion":1,"tool":"apply"}`, `{"id":"run-1","expectedVersion":1,"actor":{"subject":"owner"}}`, `{"id":"run-1","expectedVersion":1} {}`} {
		if w := invRequest(h, "POST", path, body, "agent", "alpha"); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := invRequest(h, "POST", path, `{"id":"run-1","expectedVersion":1}`, "agent", "alpha")
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "lease") || strings.Contains(w.Body.String(), "Bearer") {
		t.Fatal("private worker state leaked")
	}
	if w = invRequest(h, "GET", path+"/run-1", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("another tenant read run", w.Code)
	}
	if w = invRequest(h, "POST", path+"/run-1/cancel", `{"expectedVersion":1}`, "editor", "alpha"); w.Code != 403 {
		t.Fatal("another editor cancelled job", w.Code)
	}
	if w = invRequest(h, "POST", path+"/run-1/cancel", `{"expectedVersion":1}`, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "POST", path+"/run-1/cancel", `{"expectedVersion":1}`, "owner", "alpha"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "GET", path, "", "viewer", "alpha"); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "GET", path+"?tenant=beta", "", "viewer", "alpha"); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestReviewRunStreamViewerScopeAndCursor(t *testing.T) {
	h := investigationHandler(t)
	invRequest(h, "POST", "/api/investigations", createInvestigationJSON, "editor", "alpha")
	path := "/api/investigations/shared-id/runs/run-1"
	if w := invRequest(h, "POST", "/api/investigations/shared-id/runs", `{"id":"run-1","expectedVersion":1}`, "agent", "alpha"); w.Code != 202 {
		t.Fatal(w.Code)
	}
	invRequest(h, "POST", path+"/cancel", `{"expectedVersion":1}`, "owner", "alpha")
	w := invRequest(h, "GET", path+"/events", "", "viewer", "alpha")
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(w.Body.String(), "event: run") || !strings.Contains(w.Body.String(), "event: done") || strings.Contains(w.Body.String(), "private_state") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "GET", path+"/events", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant stream", w.Code)
	}
	r := httptest.NewRequest("GET", path+"/events", nil)
	r.Header.Set("Authorization", "Bearer viewer")
	r.Header.Set(tenancy.Header, "alpha")
	r.Header.Set("Last-Event-ID", "999999")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("invalid resume cursor", w.Code)
	}
}

func TestReviewSkillsHTTPUsesTenantAndExactVersion(t *testing.T) {
	h := investigationHandler(t)
	for _, tenant := range []string{"alpha", "beta"} {
		w := invRequest(h, "GET", "/api/review-skills", "", "viewer", tenant)
		var list v1.ReviewSkillList
		if json.Unmarshal(w.Body.Bytes(), &list) != nil || w.Code != 200 || list.Tenant != tenant || len(list.Items) != 3 {
			t.Fatal(w.Code, w.Body.String())
		}
		skill := list.Items[0]
		path := "/api/review-skills/" + skill.Metadata.Name + "/versions/" + skill.Metadata.Version
		w = invRequest(h, "GET", path, "", "viewer", tenant)
		var doc v1.ReviewSkillDocument
		if json.Unmarshal(w.Body.Bytes(), &doc) != nil || w.Code != 200 || doc.Tenant != tenant || doc.Item.Digest != skill.Digest {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/api/review-skills?tenant=beta", 400}, {"/api/review-skills/evidence-check/versions/999.0.0", 404}} {
		if w := invRequest(h, "GET", tc.path, "", "viewer", "alpha"); w.Code != tc.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := invRequest(h, "GET", "/api/review-skills", "", "viewer", "ungranted"); w.Code == 200 {
		t.Fatal("ungranted tenant accepted")
	}
	if w := invRequest(h, "POST", "/api/review-skills", "{}", "owner", "alpha"); w.Code != 405 {
		t.Fatal("caller-owned executable contract accepted", w.Code)
	}
}
