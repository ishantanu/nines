package mcpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestTemplateToolsStayReadOnlyAndTenantBound(t *testing.T) {
	s := service.New(store.NewMemory(), nil)
	handler, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "template-agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(handler)
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "alpha", Token: "template-agent"})
	call(t, ctx, session, "list_slo_templates", map[string]any{"category": "agent"})
	call(t, ctx, session, "render_slo_template", map[string]any{"id": "agent-evaluation-pass", "values": map[string]any{"name": "reviewed-evaluation"}})
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "render_slo_template", Arguments: map[string]any{"id": "../private", "values": map[string]any{}}})
	if err == nil && !result.IsError {
		t.Fatal("invalid ID accepted")
	}
	rows, err := s.Store.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("template tool wrote resources", err)
	}
	foreign, foreignCtx := connect(t, Config{Server: api.URL, Tenant: "other", Token: "template-agent"})
	result, err = foreign.CallTool(foreignCtx, &mcp.CallToolParams{Name: "list_slo_templates", Arguments: map[string]any{}})
	if err == nil && !result.IsError {
		t.Fatal("ungranted tenant accepted")
	}
}

func TestExtensionMCPUsesPinnedTypedRecipeWithoutWrites(t *testing.T) {
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
	if _, err = slotemplates.Install(lock, data, slotemplates.SHA256(data), false); err != nil {
		t.Fatal(err)
	}
	registry, err := slotemplates.Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	s := service.New(store.NewMemory(), nil)
	handler, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Templates: registry, Service: s, Grants: []tenancy.Grant{{Token: "agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(handler)
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "alpha", Token: "agent"})
	template, _ := registry.Get("example-agent--evaluation-pass")
	call(t, ctx, session, "list_slo_templates", map[string]any{"category": "agent"})
	values := map[string]any{"expectedTemplateDigest": template.Digest, "inputs": map[string]any{"metric": "agent_evaluations_total", "service": "support-agent", "population": map[string]string{"service": "support-agent", "rubric": "support-v1", "evaluator": "rules-v1"}}}
	call(t, ctx, session, "render_slo_template", map[string]any{"id": template.ID, "values": values})
	delete(values, "expectedTemplateDigest")
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "render_slo_template", Arguments: map[string]any{"id": template.ID, "values": values}})
	if err == nil && !result.IsError {
		t.Fatal("extension render omitted pin")
	}
	rows, err := s.Store.List(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatal("MCP recipe wrote resources", err)
	}
}
