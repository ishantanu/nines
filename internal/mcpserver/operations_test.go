package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func rejected(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err == nil && !result.IsError {
		t.Fatalf("%s accepted invalid/unauthorized call: %+v", name, args)
	}
}
func dataInto(t *testing.T, result *mcp.CallToolResult, dst any) {
	t.Helper()
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var wrapper struct{ Data json.RawMessage }
	if err = json.Unmarshal(b, &wrapper); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(wrapper.Data, dst); err != nil {
		t.Fatal(err)
	}
}
func TestReadOperationsPlansAndResourceTemplates(t *testing.T) {
	var calls atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Nines-Tenant") != "commerce" || r.Header.Get("Authorization") != "Bearer agent" {
			t.Error("lost fixed identity")
		}
		switch r.URL.Path {
		case "/api/resources/plan":
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/yaml" || r.Header.Get("If-Match") != "" {
				t.Error("invalid plan request")
			}
			b, _ := io.ReadAll(r.Body)
			if _, err := model.ParseDefinition(b); err != nil {
				t.Error(err)
			}
			_, _ = fmt.Fprint(w, `{"action":"update","expectedRevision":2}`)
		case "/api/slos/api-availability/manifest":
			if r.Header.Get("Accept") != "application/yaml" {
				t.Error("YAML Accept missing")
			}
			_, _ = fmt.Fprint(w, examples.APIAvailability)
		case "/api/journeys/complete-checkout/manifest":
			if r.Header.Get("Accept") != "application/yaml" {
				t.Error("YAML Accept missing")
			}
			if q := r.URL.Query().Get("revision"); q != "" && q != "2" {
				t.Error("wrong revision", q)
			}
			_, _ = fmt.Fprint(w, examples.Checkout)
		case "/api/slos/api-availability/history":
			if r.URL.RawQuery != "" {
				if r.URL.Query().Get("rangeSeconds") != "3600" || r.URL.Query().Get("resolutionSeconds") != "300" {
					t.Error(r.URL.RawQuery)
				}
			}
			_, _ = fmt.Fprint(w, `{"configRevision":2,"points":[],"violations":[]}`)
		default:
			if r.Method != "GET" {
				t.Error("unexpected mutation", r.Method, r.URL.Path)
			}
			_, _ = fmt.Fprint(w, `{"safe":true}`)
		}
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "agent"})
	call(t, ctx, session, "list_connections", map[string]any{})
	call(t, ctx, session, "get_access", map[string]any{})
	call(t, ctx, session, "slo_history", map[string]any{"id": "api-availability", "rangeSeconds": 3600, "resolutionSeconds": 300})
	call(t, ctx, session, "plan_slo_rules", map[string]any{"id": "api-availability"})
	call(t, ctx, session, "plan_slo", map[string]any{"manifest": examples.APIAvailability})
	call(t, ctx, session, "plan_journey", map[string]any{"manifest": examples.Checkout})
	var exported struct{ Manifest string }
	dataInto(t, call(t, ctx, session, "export_slo", map[string]any{"id": "api-availability"}), &exported)
	if exported.Manifest != examples.APIAvailability {
		t.Fatal("SLO YAML changed")
	}
	dataInto(t, call(t, ctx, session, "export_journey", map[string]any{"id": "complete-checkout", "revision": 2}), &exported)
	if exported.Manifest != examples.Checkout {
		t.Fatal("journey YAML changed")
	}
	templates, err := session.ListResourceTemplates(ctx, nil)
	if err != nil || len(templates.ResourceTemplates) != 9 {
		t.Fatal(templates, err)
	}
	for _, entry := range []struct{ uri, media, content string }{
		{"nines://commerce/slos/api-availability/manifest", "application/yaml", examples.APIAvailability},
		{"nines://commerce/journeys/complete-checkout/manifest", "application/yaml", examples.Checkout},
		{"nines://commerce/slos/api-availability/history", "application/json", `"configRevision":2`},
		{"nines://commerce/slos/api-availability/status", "application/json", `"tenant":"commerce"`},
		{"nines://commerce/slos/api-availability/rules", "application/json", `"tenant":"commerce"`},
		{"nines://commerce/connections", "application/json", `"tenant":"commerce"`},
	} {
		got, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: entry.uri})
		if err != nil {
			t.Fatal(entry.uri, err)
		}
		if len(got.Contents) != 1 || got.Contents[0].MIMEType != entry.media || !strings.Contains(got.Contents[0].Text, entry.content) {
			t.Fatal(got)
		}
	}
	before := calls.Load()
	for _, uri := range []string{"nines://other/slos/api-availability/manifest", "nines://commerce/slos/a%2Fb/status", "nines://commerce/slos/../status", "nines://commerce/slos/api-availability/history?tenant=other"} {
		if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatal("accepted URI", uri)
		}
	}
	for _, args := range []map[string]any{
		{"id": "api-availability", "from": "2026-10-18T00:00:00Z", "rangeSeconds": 60},
		{"id": "api-availability", "rangeSeconds": 0},
		{"id": "api-availability", "rangeSeconds": -1},
		{"id": "api-availability", "rangeSeconds": 5356801},
		{"id": "api-availability", "resolutionSeconds": 0},
		{"id": "api-availability", "from": "yesterday"},
		{"id": "api-availability", "from": "2026-10-18T01:00:00Z", "to": "2026-10-18T00:00:00Z"},
		{"id": "api-availability", "tenant": "other"},
		{"id": "api-availability", "source": "untrusted"},
	} {
		rejected(t, ctx, session, "slo_history", args)
	}
	rejected(t, ctx, session, "export_journey", map[string]any{"id": "complete-checkout", "revision": 0})
	rejected(t, ctx, session, "plan_slo", map[string]any{"manifest": examples.Checkout})
	rejected(t, ctx, session, "plan_slo_rules", map[string]any{"id": "api-availability", "mimirOrgID": "other"})
	if calls.Load() != before {
		t.Fatal("invalid requests reached API")
	}
}
func TestHistoryPreservesExplicitTimestampPrecision(t *testing.T) {
	path, err := historyPath(historyInput{ID: "checkout", From: "2026-10-18T00:00:00.123Z", To: "2026-10-18T01:00:00Z"})
	if err != nil || !strings.Contains(path, "00.123Z") {
		t.Fatal(path, err)
	}
}

func TestMCPRealSLOUpdateRuleReviewAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Minute)
	commerce := service.New(store.NewMemory(), nil)
	operations := service.New(store.NewMemory(), nil)
	for i, svc := range []*service.Service{commerce, operations} {
		svc.Now = func() time.Time { return at }
		slo := model.SLO{ID: "availability", Name: "API", Project: "p", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(95)", TotalQuery: "vector(100)", Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 600, BurnRate: 2, MinCoverage: 1}}
		if err := svc.Create(ctx, slo); err != nil {
			t.Fatal(err)
		}
		points := []model.Point{}
		for n := 0; n < 120; n++ {
			points = append(points, model.Point{Time: at.Add(-time.Duration(n) * time.Minute), Good: []float64{95, 99}[i], Total: 100})
		}
		if err := svc.Ingest(ctx, slo.ID, points); err != nil {
			t.Fatal(err)
		}
	}
	manager := &rules.Manager{Tenant: "commerce", Targets: map[string]rules.Target{"primary": {Connection: "primary", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir()}}}
	handler, err := httpapi.NewMulti([]httpapi.Workspace{
		{ID: "commerce", Name: "Commerce", Service: commerce, Rules: manager, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}},
		{ID: "operations", Name: "Operations", Service: operations, Grants: []tenancy.Grant{{Token: "operations-owner", Role: "owner"}}},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(handler)
	defer api.Close()
	owner, ownerctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "owner", AllowWrites: true, AllowRuleDeployment: true})
	viewer, viewerctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "viewer"})
	agent, agentctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "agent", AllowWrites: true, AllowRuleDeployment: true})
	wrong, wrongctx := connect(t, Config{Server: api.URL, Tenant: "operations", Token: "owner"})
	rejected(t, wrongctx, wrong, "slo_history", map[string]any{"id": "availability"})
	var hist struct {
		ConfigRevision int64
		Points         []struct {
			Reliability    *float64
			TargetViolated bool
		}
	}
	dataInto(t, call(t, viewerctx, viewer, "slo_history", map[string]any{"id": "availability", "rangeSeconds": 1800, "resolutionSeconds": 300}), &hist)
	if hist.ConfigRevision != 1 || len(hist.Points) != 7 || hist.Points[6].Reliability == nil || *hist.Points[6].Reliability != .95 || !hist.Points[6].TargetViolated {
		t.Fatal(hist)
	}
	var plan rules.Plan
	dataInto(t, call(t, ownerctx, owner, "plan_slo_rules", map[string]any{"id": "availability"}), &plan)
	if plan.Target.Namespace != "nines" || plan.Target.Connection != "primary" {
		t.Fatal(plan)
	}
	rejected(t, agentctx, agent, "deploy_slo_rules", map[string]any{"id": "availability", "expectedDigest": plan.Digest})
	var deploy rules.Deployment
	dataInto(t, call(t, ownerctx, owner, "deploy_slo_rules", map[string]any{"id": "availability", "expectedDigest": plan.Digest}), &deploy)
	if deploy.State != "written" {
		t.Fatal(deploy)
	}
	rec, _ := commerce.Store.Read(ctx, "availability")
	definition := model.DefinitionFromSLO(rec.SLO)
	definition.Spec.WindowSeconds = 7200
	definition.Spec.Policy.LongWindowSeconds = 1200
	yaml, _ := yaml.Marshal(definition)
	call(t, viewerctx, viewer, "plan_slo", map[string]any{"manifest": string(yaml)})
	rejected(t, agentctx, agent, "apply_slo", map[string]any{"manifest": string(yaml), "expectedRevision": 1})
	rejected(t, ownerctx, owner, "apply_slo", map[string]any{"manifest": string(yaml)})
	call(t, ownerctx, owner, "apply_slo", map[string]any{"manifest": string(yaml), "expectedRevision": 1})
	rejected(t, ownerctx, owner, "apply_slo", map[string]any{"manifest": string(yaml), "expectedRevision": 1})
	rejected(t, ownerctx, owner, "deploy_slo_rules", map[string]any{"id": "availability", "expectedDigest": plan.Digest})
	var exported struct{ Manifest string }
	dataInto(t, call(t, viewerctx, viewer, "export_slo", map[string]any{"id": "availability"}), &exported)
	parsed, err := model.ParseSLODefinition([]byte(exported.Manifest))
	if err != nil || parsed.Spec.WindowSeconds != 7200 {
		t.Fatal(parsed, err)
	}
	var current struct {
		SLO    model.SLO
		Status model.Snapshot
	}
	dataInto(t, call(t, viewerctx, viewer, "get_slo", map[string]any{"id": "availability"}), &current)
	if current.SLO.Revision != 2 || current.SLO.WindowSeconds != 7200 || current.Status.Reliability == nil || *current.Status.Reliability != .95 {
		t.Fatal(current)
	}
	rejected(t, wrongctx, wrong, "get_slo", map[string]any{"id": "availability"})
	rec, _ = commerce.Store.Read(ctx, "availability")
	other, _ := operations.Store.Read(ctx, "availability")
	if rec.SLO.Revision != 2 || len(rec.Points) != 120 || model.ConfigRevision(other.SLO) != 1 || other.SLO.WindowSeconds != 3600 {
		t.Fatal("edit escaped tenant or destroyed samples")
	}
}

func TestRuleDeploymentRequiresBothFlagsAndReviewedDigest(t *testing.T) {
	if _, err := New(Config{Server: "http://127.0.0.1:1", Tenant: "commerce", AllowRuleDeployment: true}); err == nil {
		t.Fatal("deployment enabled without writes")
	}
	var calls atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/connections/production/test" || r.Method != "POST" || r.ContentLength != 0 {
			t.Error("wrong connection-test request")
		}
		_, _ = fmt.Fprint(w, `{"status":"reachable"}`)
	}))
	defer api.Close()
	ordinary, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", AllowWrites: true})
	tools, err := ordinary.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hasTool(tools.Tools, "deploy_slo_rules") {
		t.Fatal("deployment needs separate opt-in")
	}
	call(t, ctx, ordinary, "test_connection", map[string]any{"id": "production"})
	enabled, enabledctx := connect(t, Config{Server: api.URL, Tenant: "commerce", AllowWrites: true, AllowRuleDeployment: true})
	tools, err = enabled.ListTools(enabledctx, nil)
	if err != nil || len(tools.Tools) != 67 || !hasTool(tools.Tools, "deploy_slo_rules") {
		t.Fatal(tools, err)
	}
	if hasTool(tools.Tools, "approve_workflow") || hasTool(tools.Tools, "reject_workflow") {
		t.Fatal("workflow approval exposed")
	}
	before := calls.Load()
	for _, args := range []map[string]any{
		{"id": "availability"},
		{"id": "availability", "expectedDigest": ""},
		{"id": "availability", "expectedDigest": "unreviewed"},
		{"id": "availability", "expectedDigest": strings.Repeat("A", 64)},
		{"id": "availability", "expectedDigest": strings.Repeat("a", 64), "rulerURL": "https://other.invalid"},
		{"id": "availability", "expectedDigest": strings.Repeat("a", 64), "mimirOrgID": "other"},
	} {
		rejected(t, enabledctx, enabled, "deploy_slo_rules", args)
	}
	if calls.Load() != before {
		t.Fatal("invalid digest/target reached API")
	}
}
func TestYAMLExportsUseBoundedRedactedClient(t *testing.T) {
	var redirects atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirects.Add(1) }))
	defer target.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/slos/redirect/manifest":
			http.Redirect(w, r, target.URL, http.StatusFound)
		case "/api/slos/large/manifest":
			_, _ = fmt.Fprint(w, strings.Repeat("x", maxResponseBytes+1))
		case "/api/slos/bad/manifest":
			_, _ = fmt.Fprint(w, "upstream-private-token: hidden")
		case "/api/slos/other-kind/manifest":
			_, _ = fmt.Fprint(w, examples.Checkout)
		default:
			w.WriteHeader(500)
			_, _ = fmt.Fprint(w, "upstream-private-token")
		}
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "existing-secret"})
	for _, id := range []string{"redirect", "large", "bad", "other-kind", "error"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "export_slo", Arguments: map[string]any{"id": id}})
		if err == nil && !result.IsError {
			t.Fatal("invalid export accepted", id)
		}
		body, _ := json.Marshal(result)
		if strings.Contains(string(body), "private-token") || strings.Contains(string(body), "existing-secret") || strings.Contains(string(body), target.URL) {
			t.Fatal("export leaked sensitive errors")
		}
	}
	if redirects.Load() != 0 {
		t.Fatal("YAML export followed redirect")
	}
}
