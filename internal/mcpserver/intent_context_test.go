package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentContextMCPReadOnlyScopedContract(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(s, "viewer", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "default", Token: "viewer"})
	in := v1.IntentContextInput{Service: "api", Source: "primary", Project: "test"}
	result := call(t, ctx, session, "intent_context", in)
	b, _ := json.Marshal(result.StructuredContent)
	var response struct {
		Tenant string           `json:"tenant"`
		Data   v1.IntentContext `json:"data"`
	}
	if json.Unmarshal(b, &response) != nil || response.Tenant != "default" || response.Data.Tenant != "default" || response.Data.APIVersion != v1.APIVersion {
		t.Fatal(string(b))
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "intent_context" {
			found = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Fatal("context tool can write")
			}
		}
	}
	if !found {
		t.Fatal("missing tool")
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "intent_context", Arguments: map[string]any{"service": "api", "source": "primary", "project": "test", "tenant": "beta"}})
	if err == nil && !bad.IsError {
		t.Fatal("tenant override accepted")
	}
}

func TestIntentCandidatesMCPReadOnlyScope(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(s, "viewer", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "default", Token: "viewer"})
	in := v1.IntentCandidateInput{Service: "api", Project: "test", Source: "primary", Population: map[string]string{"service": "api"}, Metrics: []string{"http_requests_total"}}
	result := call(t, ctx, session, "discover_intent_candidates", in)
	b, _ := json.Marshal(result.StructuredContent)
	var out struct {
		Tenant string                      `json:"tenant"`
		Data   v1.IntentCandidateDiscovery `json:"data"`
	}
	if json.Unmarshal(b, &out) != nil || out.Tenant != "default" || out.Data.Tenant != "default" || !out.Data.HumanConfirmationRequired || len(out.Data.Candidates) != 1 {
		t.Fatal(string(b))
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "discover_intent_candidates" {
			found = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
				t.Fatal("incorrect annotations")
			}
		}
	}
	if !found {
		t.Fatal("missing candidate tool")
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "discover_intent_candidates", Arguments: map[string]any{"service": "api", "source": "primary", "project": "test", "population": map[string]string{"service": "api"}, "tenant": "beta"}})
	if err == nil && !bad.IsError {
		t.Fatal("tenant override accepted")
	}
}

func TestIntentEvidenceMCPReadOnlyAndPinnedScope(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(s, "viewer", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer api.Close()
	in, err := testtelemetry.EvidenceInput(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "default", Token: "viewer"})
	result := call(t, ctx, session, "prepare_intent_evidence", in)
	raw, _ := json.Marshal(result.StructuredContent)
	var out struct {
		Tenant string            `json:"tenant"`
		Data   v1.IntentEvidence `json:"data"`
	}
	if json.Unmarshal(raw, &out) != nil || out.Data.Tenant != "default" || out.Data.IntentDigest != in.IntentDigest || !out.Data.HumanConfirmationRequired || len(out.Data.Comparisons) != 1 {
		t.Fatal(string(raw))
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "prepare_intent_evidence" {
			found = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
				t.Fatal("incorrect evidence annotations")
			}
		}
	}
	if !found {
		t.Fatal("missing evidence tool")
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "prepare_intent_evidence", Arguments: map[string]any{"tenant": "beta"}})
	if err == nil && !bad.IsError {
		t.Fatal("tenant override accepted")
	}
}
