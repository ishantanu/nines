package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestIntegrationReadIsTenantScopedAndDoesNotInferRuleDeployment(t *testing.T) {
	svc := service.New(store.NewMemory(), metadataOnlyCollector{})
	now := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return now }
	slo := model.SLO{ID: "checkout", Project: "test", Service: "api", Name: "Checkout", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if err := svc.Create(context.Background(), slo); err != nil {
		t.Fatal(err)
	}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	get := func(tenant string, want int) v1.IntegrationStatus {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/slos/checkout/integration", nil)
		req.Header.Set("Authorization", "Bearer viewer")
		req.Header.Set("X-Nines-Tenant", tenant)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var out v1.IntegrationStatus
		if want == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	out := get("alpha", 200)
	if out.Tenant != "alpha" || out.Definition.State != "saved" || out.Collection.State != "not_collected" {
		t.Fatal(out)
	}
	if err := svc.Store.Update(context.Background(), "checkout", func(r *model.Record) error {
		r.Collection = v1.CollectionHealth{State: "succeeded", SLORevision: 1, CompletedAt: now, To: now, Points: 2}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out = get("alpha", 200)
	if out.Collection.State != "collected" || out.Deployment.State != "not_configured" || out.Evaluation.State != "unknown" {
		t.Fatal(out)
	}
	now = now.Add(5 * time.Minute)
	if get("alpha", 200).Collection.State != "stale" {
		t.Fatal("old collection presented as current")
	}
	get("beta", 403)
}
