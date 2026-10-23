package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
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
	if out.Collection.State != "collected" || out.Deployment.State != "not_configured" || out.Evaluation.State != "unknown" || out.Deployment.Code != "destination_not_configured" || out.Deployment.NextAction == "" || out.Evaluation.NextAction == "" {
		t.Fatal(out)
	}
	now = now.Add(5 * time.Minute)
	if get("alpha", 200).Collection.State != "stale" {
		t.Fatal("old collection presented as current")
	}
	get("beta", 403)
}

type outputCollector struct {
	metadataOnlyCollector
	identity model.RuleOutputIdentity
}

func (c *outputCollector) RuleOutput(_ context.Context, identity model.RuleOutputIdentity, _ time.Time) v1.IntegrationStage {
	c.identity = identity
	return v1.IntegrationStage{State: "not_observed", Code: "rule_output_missing", Message: "No marker in query dataset", NextAction: "Check the query route"}
}

func TestIntegrationReturnsIndependentScopedQueryOutput(t *testing.T) {
	collector := &outputCollector{}
	svc := service.New(store.NewMemory(), collector)
	svc.Sources = map[string]service.MetricSource{"production": {Kind: "prometheus", Collector: collector}}
	slo := model.SLO{ID: "checkout", Source: "production", Project: "test", Service: "api", Name: "Checkout", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if err := svc.Create(context.Background(), slo); err != nil {
		t.Fatal(err)
	}
	// A valid destination permits checking output, even if the runtime API is
	// unavailable. Neither inspection path may trigger deployment.
	backend := httptest.NewServer(http.NotFoundHandler())
	defer backend.Close()
	manager := &rules.Manager{Tenant: "alpha", Targets: map[string]rules.Target{"production": {Connection: "production", Kind: "prometheus", Directory: t.TempDir(), Namespace: "nines", QueryURL: backend.URL}}}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Rules: manager, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/slos/checkout/integration", nil)
	req.Header.Set("Authorization", "Bearer viewer")
	req.Header.Set("X-Nines-Tenant", "alpha")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out v1.IntegrationStatus
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
		t.Fatal(w.Code, w.Body, err)
	}
	if out.RuleOutput == nil || out.RuleOutput.Code != "rule_output_missing" || out.Deployment.State != "not_deployed" || out.Evaluation.State != "unknown" {
		t.Fatal(out)
	}
	if collector.identity.Tenant != "alpha" || collector.identity.Source != "production" || collector.identity.SLO != "checkout" || collector.identity.Revision != 1 || collector.identity.DefinitionDigest == "" {
		t.Fatal("query was not scoped to saved tenant/definition", collector.identity)
	}
	entries, err := os.ReadDir(manager.Targets["production"].Directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("read-only integration inspection wrote rules", err)
	}
}
