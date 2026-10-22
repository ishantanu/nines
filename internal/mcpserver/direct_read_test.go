package mcpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetSLOBypassesOversizedInventory(t *testing.T) {
	var points, lists atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer restricted" {
			t.Error("lost fixed tenant identity")
		}
		switch r.URL.Path {
		case "/api/slos":
			lists.Add(1)
			_, _ = fmt.Fprint(w, strings.Repeat("x", maxResponseBytes+1))
		case "/api/slos/checkout":
			points.Add(1)
			_, _ = fmt.Fprint(w, `{"slo":{"id":"checkout","revision":2},"status":{"reliability":0.99}}`)
		default:
			w.WriteHeader(404)
			_, _ = fmt.Fprint(w, `{"error":"missing"}`)
		}
	}))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "alpha", Token: "restricted"})
	var view struct {
		SLO struct {
			ID       string
			Revision int64
		}
	}
	dataInto(t, call(t, ctx, session, "get_slo", map[string]any{"id": "checkout"}), &view)
	if view.SLO.ID != "checkout" || view.SLO.Revision != 2 || points.Load() != 1 || lists.Load() != 0 {
		t.Fatal("get_slo did not use one point request", view)
	}
	got, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "nines://alpha/slos/checkout"})
	if err != nil || len(got.Contents) != 1 || !strings.Contains(got.Contents[0].Text, `"revision":2`) {
		t.Fatal(got, err)
	}
	if points.Load() != 2 || lists.Load() != 0 {
		t.Fatal("resource scanned inventory")
	}
	rejected(t, ctx, session, "get_slo", map[string]any{"id": "missing"})
	rejected(t, ctx, session, "get_slo", map[string]any{"id": "../beta"})
	rejected(t, ctx, session, "get_slo", map[string]any{"id": "checkout", "tenant": "beta"})
	if lists.Load() != 0 {
		t.Fatal("missing/invalid lookup fell back to full inventory")
	}
}
