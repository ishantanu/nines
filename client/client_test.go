package client_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestPublicClientActualTenantAPICreatePlanCASAndJourney(t *testing.T) {
	h, err := httpapi.NewMulti([]httpapi.Workspace{
		{ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "alpha-owner", Role: "owner"}, {Token: "alpha-viewer", Role: "viewer"}}},
		{ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "beta-owner", Role: "owner"}}},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "alpha-owner"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	slo := v1.NewSLO("checkout", "commerce", v1.SLOSpec{DisplayName: "Checkout success", Service: "payments", Target: .999, WindowSeconds: 86400, StepSeconds: 60})
	v, err := c.Validate(ctx, slo)
	if err != nil || v.Status != "valid" || v.Kind != v1.SLOKind {
		t.Fatal(v, err)
	}
	p, err := c.Plan(ctx, slo)
	if err != nil || p.Action != "create" || p.ExpectedRevision != 0 {
		t.Fatal(p, err)
	}
	r, err := c.CreateSLO(ctx, slo)
	if err != nil || r.Revision != 1 || r.Digest != p.Digest {
		t.Fatal(r, err)
	}
	_, err = c.CreateSLO(ctx, slo)
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || !apiErr.Conflict() {
		t.Fatal("missing conflict", err)
	}
	p, err = c.Plan(ctx, slo)
	if err != nil || p.Action != "unchanged" || p.ExpectedRevision != 1 {
		t.Fatal(p, err)
	}
	if _, err = c.Apply(ctx, slo, p.ExpectedRevision); err != nil {
		t.Fatal(err)
	}
	journey := v1.NewJourney("checkout-journey", "commerce", v1.JourneySpec{DisplayName: "Complete checkout", Owner: "commerce", Criticality: "critical", SLOs: []v1.JourneySLO{{ID: "checkout", Dimension: "availability", Required: true}}, Steps: []v1.JourneyStep{{ID: "submit", Name: "Submit checkout", Actor: "user", SLORefs: []string{"checkout"}}}})
	if _, err = c.Apply(ctx, journey, 0); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tenant, token string
		status        int
	}{{"alpha", "alpha-viewer", 403}, {"beta", "alpha-owner", 403}} {
		restricted, _ := client.New(client.Config{URL: server.URL, Tenant: tc.tenant, Token: tc.token})
		_, err := restricted.CreateSLO(ctx, slo)
		if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status {
			t.Fatal("access boundary failed", err)
		}
	}
}
func TestClientBoundHeadersRedirectsAndErrorRedaction(t *testing.T) {
	var reached atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "commerce" || r.Header.Get("Authorization") != "Bearer secret-token" || r.Header.Get("Content-Type") != "application/yaml" || r.Header.Get("If-Match") != `"0"` {
			t.Error("incorrect tenant/auth/YAML/CAS boundary")
		}
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "commerce", Token: "secret-token", HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ApplyYAML(context.Background(), []byte("kind: ServiceLevelObjective"), 0)
	var e *client.APIError
	if !errors.As(err, &e) || e.StatusCode != 307 || reached.Load() != 0 {
		t.Fatal("redirect forwarded credentials", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = fmt.Fprint(w, "upstream-secret-password")
	}))
	defer bad.Close()
	c, _ = client.New(client.Config{URL: bad.URL, Tenant: "commerce"})
	_, err = c.PlanYAML(context.Background(), []byte("a: b"))
	if err == nil || strings.Contains(err.Error(), "upstream-secret") {
		t.Fatal(err)
	}
	for _, url := range []string{"http://metrics.example.com", "https://user:password@example.com", "https://example.com/api", "https://example.com?token=secret"} {
		if _, err := client.New(client.Config{URL: url, Tenant: "alpha"}); err == nil {
			t.Fatal("unsafe origin accepted", url)
		}
	}
	if _, err := c.ApplyYAML(context.Background(), []byte("a: b"), -1); err == nil {
		t.Fatal("negative revision accepted")
	}
	if _, err := c.PlanYAML(context.Background(), make([]byte, (1<<20)+1)); err == nil {
		t.Fatal("oversized manifest accepted")
	}
}
func TestClientCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	c, _ := client.New(client.Config{URL: s.URL, Tenant: "alpha"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.PlanYAML(ctx, []byte("a: b"))
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
