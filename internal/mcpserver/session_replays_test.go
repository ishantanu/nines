package mcpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReplayMCPExposesMetadataOnly(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || (r.URL.Path != "/api/session-replays" && r.URL.Path != "/api/session-replays/session-one") {
			t.Error("unexpected replay access")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","items":[]}`)
	}))
	defer server.Close()
	session, ctx := connect(t, Config{Server: server.URL, Tenant: "alpha", Token: "viewer"})
	call(t, ctx, session, "list_session_replays", map[string]any{})
	call(t, ctx, session, "get_session_replay", map[string]any{"id": "session-one"})
	for name, args := range map[string]map[string]any{"get_session_replay": {"id": "../other"}, "list_session_replays": {"tenant": "beta"}, "get_session_replay_events": {"id": "session-one"}, "create_session_replay": {}} {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err == nil && !r.IsError {
			t.Fatal("unsafe replay capability accepted", name)
		}
	}
	if calls != 2 {
		t.Fatal("invalid input reached API", calls)
	}
}
