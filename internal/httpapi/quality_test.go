package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestQualityAndBudgetReadsEnforceTenantAndRevision(t *testing.T) {
	s := service.New(store.NewMemory(), nil)
	if err := s.Create(context.Background(), model.SLO{ID: "api", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}}, {ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "other", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"quality", "budget"} {
		for _, tc := range []struct {
			tenant, token, query string
			want                 int
		}{{"alpha", "viewer", "", 200}, {"alpha", "agent", "", 200}, {"beta", "viewer", "", 403}, {"beta", "other", "", 404}, {"alpha", "viewer", "?expectedRevision=2", 409}, {"alpha", "viewer", "?expectedRevision=-1", 400}} {
			req := httptest.NewRequest("GET", "/api/slos/api/"+suffix+tc.query, nil)
			req.Header.Set("X-Nines-Tenant", tc.tenant)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("%s %s: %d %s", suffix, tc.tenant, w.Code, w.Body.String())
			}
		}
	}
}
