package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

// Platform and sign-in requests are labelled with their exact route, so they
// can be graphed and alerted on separately, never as /api/* or /auth/*.
func TestPlatformAndSignInRequestsCarryExactRoutes(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	public := "http://" + server.Listener.Addr().String()
	config, err := identity.ParseConfig([]byte(fmt.Sprintf("publicURL: %s\nproviders:\n- id: local\n  name: Local\n  type: local\n", public)), func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(context.Background(), config, identity.NewMemory(), map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil)}}, slog.New(slog.NewJSONHandler(&logs, nil)), auth)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = h
	server.Start()
	defer server.Close()
	for _, r := range []struct{ method, path string }{
		{"GET", "/auth/cli/capabilities"},
		{"POST", "/auth/cli/device/token"},
		{"GET", "/auth/session"},
		{"GET", "/api/platform/overview"},
		{"POST", "/api/platform/tenants/create"},
		{"GET", "/api/platform/tenants/alpha/system-one"},
	} {
		req, _ := http.NewRequest(r.method, server.URL+r.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
	}
	routes := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "HTTP request completed" {
			routes[fmt.Sprint(entry["http.route"])] = true
		}
	}
	for _, want := range []string{"/auth/cli/capabilities", "/auth/cli/device/token", "/auth/session", "/api/platform/overview", "/api/platform/tenants/create", "/api/platform/tenants/{tenant}/system-one"} {
		if !routes[want] {
			t.Errorf("route %s not labelled; got %v", want, routes)
		}
	}
	if routes["/api/*"] || routes["/auth/*"] {
		t.Errorf("coarse route labels remain: %v", routes)
	}
}
