package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

func TestTenantAuthorizationIsolationAndAgentBoundaries(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	alpha := service.New(store.NewMemory(), nil)
	beta := service.New(store.NewMemory(), nil)
	alpha.Now = func() time.Time { return now }
	beta.Now = alpha.Now
	aw := workflow.New(alpha, workflow.NewMemory())
	bw := workflow.New(beta, workflow.NewMemory())
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: alpha, Workflows: aw, Grants: []tenancy.Grant{{Token: "alpha-owner", Role: "owner"}, {Token: "shared-owner", Role: "owner"}, {Token: "alpha-agent", Role: "agent"}, {Token: "alpha-viewer", Role: "viewer"}}}, {ID: "beta", Name: "Beta", Service: beta, Workflows: bw, Grants: []tenancy.Grant{{Token: "beta-owner", Role: "owner"}, {Token: "shared-owner", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, tenant, method, path string, body any, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if tenant != "" {
			r.Header.Set(tenancy.Header, tenant)
		}
		r.Header.Set("X-Scope-OrgID", "other-forged-org")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s tenant=%q token=%q: %d %s", method, path, tenant, token, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	request("", "", "GET", "/api/tenants", nil, 401)
	request("alpha-owner", "beta", "GET", "/api/slos", nil, 403)
	var tenants []tenantDescriptor
	_ = json.Unmarshal(request("alpha-owner", "", "GET", "/api/tenants", nil, 200), &tenants)
	if len(tenants) != 1 || tenants[0].ID != "alpha" {
		t.Fatal("tenant enumeration leak", tenants)
	}
	request("shared-owner", "", "GET", "/api/slos", nil, 400)
	request("alpha-owner", "", "GET", "/api/slos", nil, 200)
	for _, tc := range []struct {
		token, tenant string
		good          float64
	}{{"alpha-owner", "alpha", 99}, {"beta-owner", "beta", 90}} {
		for _, id := range []string{"api-availability", "background-jobs"} {
			s := model.SLO{ID: id, Name: tc.tenant + " objective", Project: "same-project", Service: "api", Target: .99, WindowSeconds: 60, StepSeconds: 60}
			request(tc.token, tc.tenant, "POST", "/api/slos", s, 201)
			request(tc.token, tc.tenant, "POST", "/api/slos/"+id+"/samples", map[string]any{"samples": []model.Point{{Time: now, Good: tc.good, Total: 100}}}, 200)
		}
		request(tc.token, tc.tenant, "POST", "/api/journeys/apply", map[string]any{"manifest": examples.AgentResolution, "expectedRevision": 0}, 200)
	}
	var a, b model.Snapshot
	_ = json.Unmarshal(request("alpha-owner", "alpha", "GET", "/api/slos/api-availability/status", nil, 200), &a)
	_ = json.Unmarshal(request("beta-owner", "beta", "GET", "/api/slos/api-availability/status", nil, 200), &b)
	if a.Good != 99 || b.Good != 90 {
		t.Fatal("sample collision", a, b)
	}
	alphaEvents, _ := alpha.Store.Deliveries(ctx)
	betaEvents, _ := beta.Store.Deliveries(ctx)
	if len(alphaEvents) != 0 || len(betaEvents) != 0 {
		t.Fatal("unexpected events")
	}
	request("alpha-agent", "alpha", "POST", "/api/slos", model.SLO{}, 403)
	request("alpha-viewer", "alpha", "POST", "/api/journeys/plan", map[string]any{"manifest": examples.AgentResolution}, 200)
	request("alpha-viewer", "alpha", "POST", "/api/workflows", map[string]any{}, 403)
	request("alpha-agent", "alpha", "POST", "/api/journeys/apply", map[string]any{"manifest": examples.AgentResolution, "expectedRevision": 1}, 403)
	var j model.Journey
	j, _ = model.ParseJourney([]byte(examples.AgentResolution))
	j.Spec.Description = "Proposal from agent"
	proposal := map[string]any{"id": "same-workflow", "kind": "propose_journey", "manifest": j}
	var w workflow.Workflow
	_ = json.Unmarshal(request("alpha-agent", "alpha", "POST", "/api/workflows", proposal, 201), &w)
	if w.State != "awaiting_approval" {
		t.Fatal(w)
	}
	decision := map[string]any{"expectedVersion": w.Version, "digest": w.Proposal.Digest, "reason": "Reviewed tenant Alpha proposal"}
	request("alpha-agent", "alpha", "POST", "/api/workflows/same-workflow/approve", decision, 403)
	request("alpha-owner", "alpha", "POST", "/api/workflows/same-workflow/approve", decision, 200)
	// Beta sees neither Alpha's workflow nor its applied revision.
	request("beta-owner", "beta", "GET", "/api/workflows/same-workflow", nil, 404)
	rev, err := beta.Journeys.JourneyRevision(ctx, "assisted-resolution", 0)
	if err != nil || rev.Revision != 1 || rev.Manifest.Spec.Description == j.Spec.Description {
		t.Fatal("journey crossed tenant boundary", rev, err)
	}
	request("beta-owner", "beta", "POST", "/api/workflows", map[string]any{"id": "same-workflow", "kind": "investigate"}, 201)
	// Workflow, evidence and skill APIs inherit the exact same boundary.
	request("alpha-agent", "beta", "GET", "/api/skills", nil, 403)
	request("alpha-agent", "alpha", "GET", "/api/skills", nil, 200)
	e := model.RunEvidence{ID: "same-run", JourneyRevision: 1, StartedAt: now.Add(-time.Minute), FinishedAt: now, Outcome: "unknown"}
	request("alpha-agent", "alpha", "POST", "/api/journeys/assisted-resolution/runs", e, 200)
	runs, _ := beta.Journeys.Runs(ctx, "assisted-resolution")
	if len(runs) != 0 {
		t.Fatal("run evidence leaked", runs)
	}
	// Two tenant selectors and forged origins are rejected before routing.
	r := httptest.NewRequest("GET", "/api/slos", nil)
	r.Header.Set("Authorization", "Bearer shared-owner")
	r.Header.Add(tenancy.Header, "alpha")
	r.Header.Add(tenancy.Header, "beta")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
	r = httptest.NewRequest("GET", "/api/tenants", nil)
	r.Header.Set("Authorization", "Bearer alpha-owner")
	r.Header.Set("Origin", "https://foreign.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatal(rec.Code)
	}
	workspace := request("alpha-agent", "alpha", "GET", "/api/workspace", nil, 200)
	if !strings.Contains(string(workspace), `"role":"agent"`) || strings.Contains(string(workspace), "alpha-agent") {
		t.Fatal(string(workspace))
	}
}
