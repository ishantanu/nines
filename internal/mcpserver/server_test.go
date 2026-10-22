package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/buildinfo"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

func connect(t *testing.T, c Config) (*mcp.ClientSession, context.Context) {
	t.Helper()
	s, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "nines-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

func call(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if r.IsError {
		t.Fatalf("%s returned error: %+v", name, r.Content)
	}
	return r
}

func hasTool(tools []*mcp.Tool, name string) bool {
	for _, x := range tools {
		if x.Name == name {
			return true
		}
	}
	return false
}

func TestReadOnlyProtocolTenantBinding(t *testing.T) {
	var calls atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "commerce" || r.Header.Get("Authorization") != "Bearer restricted-token" {
			t.Errorf("invalid API boundary: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/slos/checkout":
			_, _ = fmt.Fprint(w, `{"slo":{"id":"checkout","name":"Checkout"},"status":{"coverage":0.5}}`)
		case "/api/slos/checkout/status":
			_, _ = fmt.Fprint(w, `{"sloId":"checkout","coverage":0.5,"dataStatus":"partial"}`)
		case "/api/skills/reliability-investigation":
			_, _ = fmt.Fprint(w, `{"id":"reliability-investigation","version":"1.0.0","digest":"test-digest","instructions":"Inspect coverage."}`)
		default:
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "restricted-token"})
	info := session.InitializeResult().ServerInfo
	if info.Name != "nines" || info.Version != buildinfo.Version {
		t.Fatalf("MCP handshake must identify the installed build: %+v", info)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 43 {
		t.Fatalf("unexpected read tools: %d", len(tools.Tools))
	}
	for _, x := range tools.Tools {
		if x.Annotations == nil || !x.Annotations.ReadOnlyHint {
			t.Fatalf("not marked read only: %s", x.Name)
		}
	}
	for _, name := range []string{"create_slo", "apply_slo", "apply_journey", "start_workflow", "record_run", "test_connection", "deploy_slo_rules", "approve_workflow", "reject_workflow"} {
		if hasTool(tools.Tools, name) {
			t.Fatalf("forbidden tool %s", name)
		}
		before := calls.Load()
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err == nil && !r.IsError {
			t.Fatalf("forbidden tool %s succeeded", name)
		}
		if calls.Load() != before {
			t.Fatal("forbidden tool reached API")
		}
	}
	r := call(t, ctx, session, "get_slo", map[string]any{"id": "checkout"})
	b, _ := json.Marshal(r.StructuredContent)
	if !strings.Contains(string(b), `"tenant":"commerce"`) || !strings.Contains(string(b), `"Checkout"`) {
		t.Fatalf("wrong structured output: %s", b)
	}
	call(t, ctx, session, "slo_status", map[string]any{"id": "checkout", "at": "2026-10-18T00:00:00Z"})
	before := calls.Load()
	for _, args := range []map[string]any{{"id": "checkout", "tenant": "other"}, {"id": "../other"}, {"id": "checkout?tenant=other"}} {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_slo", Arguments: args})
		if err == nil && !r.IsError {
			t.Fatalf("invalid input accepted: %+v", args)
		}
	}
	if calls.Load() != before {
		t.Fatal("invalid inputs reached API")
	}
	resources, err := session.ListResources(ctx, nil)
	if err != nil || len(resources.Resources) != 8 {
		t.Fatalf("resource list: %+v %v", resources, err)
	}
	for _, item := range resources.Resources {
		if !strings.HasPrefix(item.URI, "nines://commerce/") {
			t.Fatalf("unscoped resource: %s", item.URI)
		}
	}
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "nines://other/skills"}); err == nil {
		t.Fatal("cross-tenant resource accepted")
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "nines://commerce/skills/reliability-investigation"})
	if err != nil || !strings.Contains(resource.Contents[0].Text, "test-digest") {
		t.Fatalf("skill resource: %+v %v", resource, err)
	}
	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil || len(prompts.Prompts) != 2 {
		t.Fatalf("prompts: %+v %v", prompts, err)
	}
	prompt, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "investigate_reliability", Arguments: map[string]string{"journeyId": "checkout"}})
	if err != nil {
		t.Fatal(err)
	}
	text := prompt.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "test-digest") || !strings.Contains(text, "human must approve") {
		t.Fatalf("prompt lacks skill/review boundary: %s", text)
	}
}

func TestWriteToolsManifestCASAndWorkflowProposal(t *testing.T) {
	var mu sync.Mutex
	type request struct{ path, body, media, match string }
	var requests []request
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "commerce" || r.Header.Get("Authorization") != "Bearer owner-token" {
			t.Error("tenant/auth missing")
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, request{r.URL.Path, string(body), r.Header.Get("Content-Type"), r.Header.Get("If-Match")})
		mu.Unlock()
		_, _ = fmt.Fprint(w, `{"state":"awaiting_approval","version":1}`)
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "owner-token", AllowWrites: true})
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 54 || hasTool(tools.Tools, "approve_workflow") || hasTool(tools.Tools, "reject_workflow") {
		t.Fatal("invalid write tool surface")
	}
	call(t, ctx, session, "plan_journey", map[string]any{"manifest": examples.Checkout})
	call(t, ctx, session, "apply_journey", map[string]any{"manifest": examples.Checkout, "expectedRevision": 3})
	call(t, ctx, session, "create_slo", map[string]any{"manifest": examples.APIAvailability})
	call(t, ctx, session, "start_workflow", map[string]any{"id": "proposal-1", "kind": "propose_journey", "manifest": examples.Checkout, "note": "Review this"})
	// An SLO tool cannot be used to apply a different kind.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_slo", Arguments: map[string]any{"manifest": examples.Checkout}})
	if err == nil && !result.IsError {
		t.Fatal("wrong manifest kind accepted")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("request count %d", len(requests))
	}
	for i, kind := range []string{model.JourneyKind, model.JourneyKind, model.SLOKind} {
		r := requests[i]
		if r.media != "application/yaml" {
			t.Fatal("YAML media type lost", r)
		}
		d, err := model.ParseDefinition([]byte(r.body))
		if err != nil || d.Kind() != kind {
			t.Fatal("invalid YAML request", r, err)
		}
	}
	if requests[0].path != "/api/resources/plan" || requests[1].path != "/api/resources/apply" || requests[1].match != `"3"` || requests[2].match != `"0"` {
		t.Fatal("YAML endpoint or CAS lost", requests)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(requests[3].body), &body); err != nil {
		t.Fatal(err)
	}
	if requests[3].path != "/api/workflows" || body["kind"] != "propose_journey" {
		t.Fatal("wrong workflow endpoint")
	}
	manifest, ok := body["manifest"].(map[string]any)
	if !ok || manifest["kind"] != model.JourneyKind {
		t.Fatal("workflow requires parsed manifest object")
	}
}

func TestActualStdioSDKClient(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "commerce" || r.Header.Get("Authorization") != "Bearer stdio-token" {
			t.Error("stdio API identity missing")
		}
		_, _ = fmt.Fprint(w, `{"tenant":{"id":"commerce"},"role":"agent"}`)
	}))
	defer api.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestStdioHelper$")
	cmd.Env = append(os.Environ(), "NINES_MCP_TEST_HELPER=1", "NINES_MCP_TEST_SERVER="+api.URL)
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-integration", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = session.Close()
	}()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || !hasTool(tools.Tools, "workspace") {
		t.Fatalf("stdio list tools: %v", err)
	}
	r := call(t, ctx, session, "workspace", map[string]any{})
	b, _ := json.Marshal(r.StructuredContent)
	if !strings.Contains(string(b), `"role":"agent"`) {
		t.Fatalf("stdio call response: %s", b)
	}
}

// Spawned through the official SDK CommandTransport, exercising its actual
// initialize, initialized, tools/list and tools/call messages over stdio.
func TestStdioHelper(t *testing.T) {
	if os.Getenv("NINES_MCP_TEST_HELPER") != "1" {
		return
	}
	s, err := New(Config{Server: os.Getenv("NINES_MCP_TEST_SERVER"), Tenant: "commerce", Token: "stdio-token"})
	if err != nil {
		os.Exit(2)
	}
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestClientConfigurationAndRedaction(t *testing.T) {
	for _, c := range []Config{
		{Server: "http://remote.example", Tenant: "commerce"},
		{Server: "https://user:secret@api.example", Tenant: "commerce"},
		{Server: "https://api.example/subpath", Tenant: "commerce"},
		{Server: "https://api.example?secret=token", Tenant: "commerce"},
		{Server: "https://api.example", Tenant: "other/tenant"},
		{Server: "https://api.example", Tenant: "commerce", Token: "bad\r\nvalue"},
	} {
		if _, err := New(c); err == nil {
			t.Fatalf("unsafe config accepted: %+v", c)
		}
	}
	for _, c := range []Config{{Server: "https://api.example", Tenant: "commerce"}, {Server: "http://[::1]:8080", Tenant: "commerce"}} {
		if _, err := New(c); err != nil {
			t.Fatal(err)
		}
	}
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, target.URL+"?secret=upstream-token", http.StatusFound)
		case "/error":
			w.WriteHeader(500)
			_, _ = fmt.Fprint(w, "upstream-secret bearer token")
		case "/large":
			_, _ = fmt.Fprint(w, strings.Repeat("x", maxResponseBytes+1))
		case "/invalid":
			_, _ = fmt.Fprint(w, `{} {}`)
		default:
			_, _ = fmt.Fprint(w, `{"safe":true}`)
		}
	}))
	defer api.Close()
	a, err := newAPI(Config{Server: api.URL, Tenant: "commerce", Token: "private-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/redirect", "/error", "/large", "/invalid"} {
		_, err := a.request(context.Background(), "GET", path, nil)
		if err == nil {
			t.Fatalf("accepted %s", path)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), target.URL) {
			t.Fatalf("sensitive error: %s", err)
		}
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect forwarded credentials")
	}
	got, err := a.request(context.Background(), "GET", "/valid", nil)
	if err != nil || !reflect.DeepEqual(got, map[string]any{"safe": true}) {
		t.Fatalf("valid response: %+v %v", got, err)
	}
}

func TestMCPRealTenantAPIProposalBoundary(t *testing.T) {
	ctx := context.Background()
	commerce := service.New(store.NewMemory(), nil)
	operations := service.New(store.NewMemory(), nil)
	manifest, err := model.ParseJourney([]byte(examples.Checkout))
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range manifest.Spec.SLOs {
		for _, platform := range []*service.Service{commerce, operations} {
			if err := platform.Create(ctx, model.SLO{ID: link.ID, Project: "demo", Service: "checkout", Name: "Checkout", Target: 0.999, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
				t.Fatal(err)
			}
		}
	}
	handler, err := httpapi.NewMulti([]httpapi.Workspace{
		{ID: "commerce", Name: "Commerce", Service: commerce, Grants: []tenancy.Grant{{Token: "commerce-agent", Role: "agent"}, {Token: "commerce-owner", Role: "owner"}}, Workflows: workflow.New(commerce, workflow.NewMemory())},
		{ID: "operations", Name: "Operations", Service: operations, Grants: []tenancy.Grant{{Token: "operations-agent", Role: "agent"}}, Workflows: workflow.New(operations, workflow.NewMemory())},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(handler)
	defer api.Close()
	session, mcpctx := connect(t, Config{Server: api.URL, Tenant: "commerce", Token: "commerce-agent", AllowWrites: true})
	call(t, mcpctx, session, "start_workflow", map[string]any{"id": "review-checkout", "kind": "propose_journey", "manifest": examples.Checkout})
	result := call(t, mcpctx, session, "get_workflow", map[string]any{"id": "review-checkout"})
	b, _ := json.Marshal(result.StructuredContent)
	var out struct {
		Data workflow.Workflow `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.State != "awaiting_approval" || out.Data.Proposal == nil {
		t.Fatalf("proposal not reviewable: %s", b)
	}
	if _, err := commerce.Journeys.JourneyRevision(ctx, manifest.Metadata.Name, 0); err == nil {
		t.Fatal("model proposal applied before review")
	}
	denied, err := session.CallTool(mcpctx, &mcp.CallToolParams{Name: "apply_journey", Arguments: map[string]any{"manifest": examples.Checkout, "expectedRevision": 0}})
	if err == nil && !denied.IsError {
		t.Fatal("agent token directly applied manifest")
	}
	wrongTenant, wrongctx := connect(t, Config{Server: api.URL, Tenant: "operations", Token: "commerce-agent"})
	denied, err = wrongTenant.CallTool(wrongctx, &mcp.CallToolParams{Name: "list_slos", Arguments: map[string]any{}})
	if err == nil && !denied.IsError {
		t.Fatal("agent token crossed tenant boundary")
	}
	// Approval is an explicit authenticated API action, never an MCP tool.
	body, _ := json.Marshal(map[string]any{"expectedVersion": out.Data.Version, "digest": out.Data.Proposal.Digest, "reason": "Reviewed the checkout links"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.URL+"/api/workflows/review-checkout/approve", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Nines-Tenant", "commerce")
	req.Header.Set("Authorization", "Bearer commerce-owner")
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("human approval: HTTP %d %s", response.StatusCode, body)
	}
	call(t, mcpctx, session, "get_journey", map[string]any{"id": manifest.Metadata.Name})
	if _, err := operations.Journeys.JourneyRevision(ctx, manifest.Metadata.Name, 0); err == nil {
		t.Fatal("approved proposal escaped tenant")
	}
}
