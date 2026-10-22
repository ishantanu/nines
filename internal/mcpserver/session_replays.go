package mcpserver

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addSessionReplays(s *mcp.Server, a *apiClient) {
	register(s, a, "list_session_replays", "Read bounded tenant recording metadata and manual CUJ/SLO/violation links. Does not fetch recorded DOM, record a browser or change SLO measurements.", true, func(ctx context.Context, _ emptyInput) (any, error) {
		return a.request(ctx, http.MethodGet, "/api/session-replays", nil)
	})
	register(s, a, "get_session_replay", "Read one tenant recording's metadata, retention and exact sanitized event digest. Does not fetch recorded DOM or imply a causal relationship.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/session-replays/", ""))
}
