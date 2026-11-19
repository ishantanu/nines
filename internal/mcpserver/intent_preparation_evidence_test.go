package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIntentEvidenceAttachmentMCPWriteGateAndScope(t *testing.T) {
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/intent-conversations/chat/evidence" {
			t.Error("unexpected evidence route")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tenant": "commerce", "preparationEvidence": map[string]any{"capturedVersion": 7}})
	}))
	defer api.Close()
	readonly, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce"})
	tools, err := readonly.ListTools(ctx, nil)
	if err != nil || hasTool(tools.Tools, "record_intent_evidence") {
		t.Fatal("write tool exposed without opt-in", err)
	}
	enabled, ctx := connect(t, Config{Server: api.URL, Tenant: "commerce", AllowWrites: true})
	tools, err = enabled.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "record_intent_evidence" {
			found = true
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
				t.Fatal("provider write incorrectly annotated")
			}
		}
	}
	if !found {
		t.Fatal("missing evidence tool")
	}
	args := map[string]any{"conversationId": "chat", "input": map[string]any{"expectedVersion": 7, "intentDigest": strings.Repeat("a", 64), "readTelemetry": true}}
	call(t, ctx, enabled, "record_intent_evidence", args)
	if calls != 1 {
		t.Fatal("unexpected external reads", calls)
	}
	args["tenant"] = "other"
	result, err := enabled.CallTool(ctx, &mcp.CallToolParams{Name: "record_intent_evidence", Arguments: args})
	if err == nil && !result.IsError {
		t.Fatal("tenant override accepted")
	}
	if calls != 1 {
		t.Fatal("invalid scope reached provider")
	}
}
