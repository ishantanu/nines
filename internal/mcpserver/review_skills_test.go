package mcpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExecutableSkillDiscoveryRemainsReadOnly(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("unscoped mutation")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","items":[]}`)
	}))
	defer server.Close()
	session, ctx := connect(t, Config{Server: server.URL, Tenant: "alpha", Token: "viewer"})
	call(t, ctx, session, "list_review_skills", map[string]any{})
	call(t, ctx, session, "get_review_skill", map[string]any{"id": "measurement-review", "version": "1.0.0"})
	before := calls
	for _, args := range []map[string]any{{"id": "../other", "version": "1.0.0"}, {"id": "measurement-review", "version": "latest"}, {"id": "measurement-review", "version": "1.0.0", "tenant": "beta"}} {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_review_skill", Arguments: args})
		if err == nil && !r.IsError {
			t.Fatal("unsafe discovery accepted")
		}
	}
	if calls != before {
		t.Fatal("unsafe discovery reached server")
	}
}
