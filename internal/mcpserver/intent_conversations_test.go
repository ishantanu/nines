package mcpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIntentConversationMCPDoesNotConfirmOrApply(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("unscoped intent access")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","items":[]}`)
	}))
	defer server.Close()
	readonly, ctx := connect(t, Config{Server: server.URL, Tenant: "alpha"})
	call(t, ctx, readonly, "list_intent_conversations", map[string]any{})
	for _, name := range []string{"start_intent_preparation", "confirm_intent_binding", "apply_intent_proposal"} {
		r, e := readonly.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if e == nil && !r.IsError {
			t.Fatal("forbidden capability", name)
		}
	}
	if calls != 1 {
		t.Fatal("rejected call reached API")
	}
	write, ctx := connect(t, Config{Server: server.URL, Tenant: "alpha", AllowWrites: true})
	tools, e := write.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if hasTool(tools.Tools, "confirm_intent_binding") || hasTool(tools.Tools, "apply_intent_proposal") {
		t.Fatal("human confirmation or apply exposed")
	}
	for _, tool := range tools.Tools {
		if tool.Name == "start_intent_preparation" && (tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint) {
			t.Fatal("model transmission was not declared")
		}
	}
	r, e := write.CallTool(ctx, &mcp.CallToolParams{Name: "start_intent_preparation", Arguments: map[string]any{"conversationId": "../other", "input": map[string]any{"id": "one", "expectedVersion": 1, "mode": "guided", "modelConsent": false}}})
	if e == nil && !r.IsError {
		t.Fatal("path injection accepted")
	}
	if calls != 1 {
		t.Fatal("unsafe path reached API")
	}
}
