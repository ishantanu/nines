package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestTelemetryTenantRolesAndBoundedAPI(t *testing.T) {
	a, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: a, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}, {Token: "editor", Role: "editor"}, {Token: "agent", Role: "agent"}}}, {ID: "beta", Name: "Beta", Service: b, Grants: []tenancy.Grant{{Token: "beta", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, tenant, token, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Nines-Tenant", tenant)
		r.Header.Set("Authorization", "Bearer "+token)
		if strings.HasPrefix(path, "/api/resources/") {
			r.Header.Set("Content-Type", "application/yaml")
			r.Header.Set("If-Match", `"0"`)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	first := request("GET", "/api/topology/resources?limit=1", "alpha", "viewer", "", 200)
	var page v1.TopologyCatalog
	if json.Unmarshal(first, &page) != nil || page.Tenant != "alpha" || page.NextCursor == "" {
		t.Fatal(string(first))
	}
	request("GET", "/api/topology/resources?cursor="+page.NextCursor, "beta", "beta", "", 400)
	request("GET", "/api/dependency-maps/services/view", "beta", "viewer", "", 403)
	for _, path := range []string{"/api/topology/resources?limit=101", "/api/connections/primary/telemetry?metric=up&orgID=other", "/api/dependency-maps/services/view?source=other", "/api/topology/resources?kind=unsupported", "/api/dependency-maps/services/view?expectedRevision=0"} {
		request("GET", path, "alpha", "viewer", "", 400)
	}
	request("GET", "/api/connections/primary/telemetry?metric=http_requests_total", "alpha", "viewer", "", 200)
	body := `{"expectedRevision":1,"name":"compiled","displayName":"Compiled","target":0.999,"windowSeconds":3600}`
	request("POST", "/api/bindings/requests/compile", "alpha", "viewer", body, 200)
	request("POST", "/api/bindings/requests/compile", "alpha", "viewer", strings.Replace(body, `"expectedRevision":1`, `"expectedRevision":2`, 1), 409)
	if _, err = a.Store.Read(context.Background(), "compiled"); err == nil {
		t.Fatal("compile applied SLO")
	}
	request("POST", "/api/resources/apply", "alpha", "viewer", "", 403)
	request("POST", "/api/resources/apply", "alpha", "agent", "", 403)
	raw := request("GET", "/api/dependency-maps/services/view", "alpha", "viewer", "", 200)
	var graph v1.DependencyView
	if json.Unmarshal(raw, &graph) != nil || graph.Nodes[1].PotentialImpact[0].ID != "checkout" || graph.Nodes[1].Health != "unknown" {
		t.Fatal(string(raw))
	}
	request("GET", "/api/dependency-maps/services/manifest?revision=1", "alpha", "viewer", "", 200)
}
