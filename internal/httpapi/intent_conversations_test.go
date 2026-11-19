package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentConversationAPIRolesScopeAndStrictRequests(t *testing.T) {
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s, _, e := testtelemetry.New()
		if e != nil {
			t.Fatal(e)
		}
		s.TenantID = id
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, IntentConversations: intentflow.New(s, intentflow.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}})
	}
	h, e := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	body := `{"id":"intent-chat","title":"Checkout","text":"99.9% over 1 hour","scope":{"metadata":{"name":"new-slo","project":"test"},"service":"api","source":"primary","population":{"service":"api"},"noDataPolicy":"unknown"}}`
	if w := invRequest(h, "POST", "/api/intent-conversations", body, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := invRequest(h, "POST", "/api/intent-conversations", body, "agent", "alpha")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var c v1.IntentConversation
	if json.Unmarshal(w.Body.Bytes(), &c) != nil || c.Tenant != "alpha" {
		t.Fatal(w.Body.String())
	}
	if w = invRequest(h, "GET", "/api/intent-conversations/intent-chat", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant chat exposed", w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/selection", `{"expectedVersion":1,"confirm":true,"intentDigest":"forged","binding":{"name":"requests","expectedRevision":1}}`, "agent", "alpha"); w.Code != 403 {
		t.Fatal("agent selected binding", w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations", strings.TrimSuffix(body, "}")+`,"tenant":"beta"}`, "owner", "alpha"); w.Code != 400 {
		t.Fatal("tenant injection accepted", w.Code)
	}
	if w = invRequest(h, "GET", "/api/intent-conversations?cursor=x&tenant=beta", "", "viewer", "alpha"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "GET", "/api/intent-conversations/intent-chat", "", "viewer", "alpha"); w.Code != 200 || strings.Contains(w.Body.String(), "requestDigest") || strings.Contains(w.Body.String(), "native") {
		t.Fatal("private state leaked", w.Code, w.Body.String())
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/runs", `{"id":"one","expectedVersion":1,"mode":"guided"}`, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/runs", `{"id":"one","expectedVersion":1,"mode":"guided"}`, "agent", "alpha"); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/messages", `{"id":"late","expectedVersion":2,"text":"change source"}`, "owner", "alpha"); w.Code != 409 {
		t.Fatal("edited active conversation", w.Code, w.Body.String())
	}
	for _, suffix := range []string{"/change-set/approve", "/change-set/apply", "/change-set/reject", "/change-set/rollback", "/change-set/rollback/approve", "/change-set/rollback/apply", "/change-set/rollback/reject"} {
		if w := invRequest(h, "POST", "/api/intent-conversations/intent-chat"+suffix, `{"expectedVersion":2,"digest":"forged","confirm":true}`, "agent", "alpha"); w.Code != 403 {
			t.Fatal("agent reached approval/apply", suffix, w.Code)
		}
		if w := invRequest(h, "POST", "/api/intent-conversations/intent-chat"+suffix, `{}`, "viewer", "alpha"); w.Code != 403 {
			t.Fatal("viewer reached approval/apply", suffix, w.Code)
		}
	}
	for _, token := range []string{"viewer", "agent"} {
		if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/activation", "", token, "beta"); w.Code != 404 {
			t.Fatal("cross-tenant activation", w.Code)
		}
		if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/activation", "", token, "alpha"); w.Code != 400 {
			t.Fatal("unapplied activation should be rejected", w.Code)
		}
		if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/activation?source=other", "", token, "alpha"); w.Code != 400 {
			t.Fatal("activation source injection", w.Code)
		}
	}
	if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/change-set/verification", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant verification", w.Code)
	}
	if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/audit", "", "viewer", "alpha"); w.Code != 200 || !strings.Contains(w.Body.String(), `"outcome":"denied"`) {
		t.Fatal("authenticated denials missing from audit", w.Code, w.Body.String())
	}
	if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/audit", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant audit exposed", w.Code)
	}
	if _, e = workspaces[0].Service.Store.Read(context.Background(), "new-slo"); e == nil {
		t.Fatal("conversation applied definition")
	}
}

func TestIntentWorkflowStatusIsScopedReadOnly(t *testing.T) {
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s, _, err := testtelemetry.New()
		if err != nil {
			t.Fatal(err)
		}
		s.TenantID = id
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, IntentConversations: intentflow.New(s, intentflow.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}}})
	}
	h, err := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"status-chat","title":"API availability","text":"Review API success","scope":{"metadata":{"name":"new-slo","project":"test"},"service":"api","source":"primary","population":{"service":"api"},"noDataPolicy":"unknown"}}`
	if w := invRequest(h, "POST", "/api/intent-conversations", body, "owner", "alpha"); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := "/api/intent-conversations/status-chat/status"
	for range 2 {
		w := invRequest(h, "GET", path, "", "viewer", "alpha")
		var out v1.IntentWorkflowStatus
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Version != 1 || out.NextAction != "run" || out.Tenant != "alpha" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := invRequest(h, "GET", path, "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant status", w.Code)
	}
	if w := invRequest(h, "GET", path+"?tenant=beta", "", "viewer", "alpha"); w.Code != 400 {
		t.Fatal("status query accepted", w.Code)
	}
}

func TestIntentUsageIsAggregateTenantScopedAndReadOnly(t *testing.T) {
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s, _, e := testtelemetry.New()
		if e != nil {
			t.Fatal(e)
		}
		s.TenantID = id
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, IntentConversations: intentflow.New(s, intentflow.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}})
	}
	h, e := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	body := `{"id":"usage-chat","title":"Secret checkout title","text":"private goal text 99.9%","scope":{"metadata":{"name":"new-slo","project":"test"},"service":"api","source":"primary","population":{"service":"api"},"noDataPolicy":"unknown"}}`
	if w := invRequest(h, "POST", "/api/intent-conversations", body, "agent", "alpha"); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := invRequest(h, "GET", "/api/intent-conversations/usage", "", "viewer", "alpha")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var u v1.IntentUsage
	if json.Unmarshal(w.Body.Bytes(), &u) != nil || u.Tenant != "alpha" || u.Started != 1 || u.Proposed != 0 || u.WindowDays != 90 {
		t.Fatalf("usage = %s", w.Body.String())
	}
	for _, leaked := range []string{"Secret checkout", "private goal", "usage-chat", "actor", "agent"} {
		if strings.Contains(w.Body.String(), leaked) {
			t.Fatalf("usage exposed %q: %s", leaked, w.Body.String())
		}
	}
	if w = invRequest(h, "GET", "/api/intent-conversations/usage", "", "viewer", "beta"); w.Code != 200 || !strings.Contains(w.Body.String(), `"started":0`) {
		t.Fatal("cross-tenant usage counted", w.Code, w.Body.String())
	}
	for _, q := range []string{"?days=0x", "?days=400", "?days=-1", "?tenant=beta", "?days=7&days=8"} {
		if w = invRequest(h, "GET", "/api/intent-conversations/usage"+q, "", "viewer", "alpha"); w.Code != 400 {
			t.Fatalf("query %q accepted: %d", q, w.Code)
		}
	}
	if w = invRequest(h, "GET", "/api/intent-conversations/usage?days=7", "", "viewer", "alpha"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/usage", "{}", "owner", "alpha"); w.Code == 200 {
		t.Fatal("usage accepted a write")
	}
}

func TestDependencyCompositionIsReadOnlyBoundedAndLabelled(t *testing.T) {
	s, _, e := testtelemetry.New()
	if e != nil {
		t.Fatal(e)
	}
	s.TenantID = "alpha"
	h, e := NewMulti([]Workspace{{ID: "alpha", Name: "alpha", Service: s, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	w := invRequest(h, "GET", "/api/dependency-maps/services/composition", "", "viewer", "alpha")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out v1.DependencyComposition
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Tenant != "alpha" || out.ID != "services" || out.Checks == nil || len(out.Limitations) < 3 {
		t.Fatalf("composition = %s", w.Body.String())
	}
	for _, c := range out.Checks {
		if c.Verdict == "within_ceiling" && len(c.Unmeasured) > 0 {
			t.Fatalf("an incomplete ceiling was reported as confirmed: %+v", c)
		}
	}
	if w = invRequest(h, "GET", "/api/dependency-maps/services/composition?expectedRevision=1", "", "viewer", "alpha"); w.Code != 400 {
		t.Fatal("query fields accepted", w.Code)
	}
	if w = invRequest(h, "GET", "/api/dependency-maps/missing/composition", "", "viewer", "alpha"); w.Code != 404 {
		t.Fatal("missing map", w.Code)
	}
	if w = invRequest(h, "POST", "/api/dependency-maps/services/composition", "{}", "owner", "alpha"); w.Code == 200 {
		t.Fatal("composition accepted a write")
	}
}

func TestCustomerReportIsReadOnlyBoundedAndStatesUnsupportedProviders(t *testing.T) {
	s, _, e := testtelemetry.New()
	if e != nil {
		t.Fatal(e)
	}
	s.TenantID = "alpha"
	m := v1.NewSLO("per-customer", "test", v1.SLOSpec{DisplayName: "API availability", Service: "api", Source: "primary", Target: .999, WindowSeconds: 3600, StepSeconds: 60,
		Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "Completed API requests; excludes requests never reaching the server."},
		Customers:   &v1.CustomerPolicy{Label: "account", Customers: []v1.CustomerRef{{Value: "acme-01", Name: "Acme"}}}})
	if _, e = s.ApplyDefinition(context.Background(), model.Definition{SLO: (*model.SLODefinition)(&m)}, 0); e != nil {
		t.Fatal(e)
	}
	h, e := NewMulti([]Workspace{{ID: "alpha", Name: "alpha", Service: s, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	w := invRequest(h, "GET", "/api/slos/per-customer/customers", "", "viewer", "alpha")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out v1.CustomerReport
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Tenant != "alpha" || out.SLOID != "per-customer" || out.Label != "account" || out.State != "unsupported" || len(out.Items) != 0 || len(out.Limitations) < 3 {
		t.Fatalf("a provider that cannot group by customer must say so, not return an empty list: %s", w.Body.String())
	}
	if w = invRequest(h, "GET", "/api/slos/per-customer/customers?expectedRevision=1", "", "viewer", "alpha"); w.Code != 200 {
		t.Fatal("expectedRevision rejected", w.Code)
	}
	for _, q := range []string{"?expectedRevision=0", "?expectedRevision=x", "?tenant=beta", "?customer=acme-01", "?expectedRevision=1&expectedRevision=2"} {
		if w = invRequest(h, "GET", "/api/slos/per-customer/customers"+q, "", "viewer", "alpha"); w.Code != 400 {
			t.Errorf("query %q accepted: %d", q, w.Code)
		}
	}
	if w = invRequest(h, "GET", "/api/slos/missing/customers", "", "viewer", "alpha"); w.Code != 404 {
		t.Fatal("missing SLO", w.Code)
	}
	if w = invRequest(h, "POST", "/api/slos/per-customer/customers", "{}", "owner", "alpha"); w.Code == 200 {
		t.Fatal("customers accepted a write")
	}
}
