package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestNativeReadsRejectTenantAndRouteOverrides(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	h, e := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Rules: &rules.Manager{Tenant: "alpha", Targets: map[string]rules.Target{}}, Grants: []tenancy.Grant{{Token: "read", Role: "viewer"}, {Token: "agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{"read", "agent"} {
		for _, tc := range []struct {
			path, tenant string
			status       int
		}{
			{"/api/connections/nr/native-slos?account=other", "alpha", 400},
			{"/api/connections/nr/native-slos?%xx=bad", "alpha", 400},
			{"/api/connections/nr/native-slos", "beta", 403},
			{"/api/connections/nr/native-slos", "alpha", 404},
			{"/api/connections/nr/native-slo-draft?nativeID=one&name=test&project=prod&service=api&entity=foreign", "alpha", 400},
			{"/api/connections/nr/native-slo-draft?nativeID=one&name=test&project=prod&service=api&name=other", "alpha", 400},
			{"/api/slos/missing/native-results?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z&endpoint=http://elsewhere", "alpha", 400},
			{"/api/slos/missing/native-results?from=2026-01-01T00:00:00Z&to=2026-03-02T00:00:00Z", "alpha", 400},
			{"/api/slos/missing/native-results?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z", "alpha", 404},
		} {
			req := httptest.NewRequest("GET", tc.path, nil).WithContext(context.Background())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Nines-Tenant", tc.tenant)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("%s %s: %d %s", token, tc.path, w.Code, w.Body)
			}
		}
	}
}
