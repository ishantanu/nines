package client_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestIntentContextClientAndAuthorization(t *testing.T) {
	s := service.New(store.NewMemory(), nil)
	if err := s.Create(context.Background(), model.SLO{ID: "api", Project: "p", Service: "api", Name: "Synthetic", Target: .99, WindowSeconds: 600, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewMulti([]httpapi.Workspace{{Features: features.All(), ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	in := v1.IntentContextInput{Service: "api", Source: "primary", Project: "p"}
	out, err := c.IntentContext(context.Background(), in)
	if err != nil || out.Tenant != "alpha" || len(out.Objectives) != 1 {
		t.Fatal(out, err)
	}
	for _, query := range []string{"&tenant=beta", "&source=other", "&limit=11", "&journeyRevision=0"} {
		path, _ := client.IntentContextPath(in)
		r := httptest.NewRequest("GET", path+query, nil)
		r.Header.Set("X-Nines-Tenant", "alpha")
		r.Header.Set("Authorization", "Bearer viewer")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(query, w.Code, w.Body.String())
		}
	}
	path, _ := client.IntentContextPath(in)
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("X-Nines-Tenant", "beta")
	r.Header.Set("Authorization", "Bearer viewer")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 && w.Code != 404 {
		t.Fatal("tenant escape", w.Code)
	}
	r = httptest.NewRequest("GET", path, nil)
	r.Header.Set("X-Nines-Tenant", "alpha")
	r.Header.Set("Authorization", "Bearer viewer")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable context")
	}
	var response v1.IntentContext
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Source.MetricNameDiscovery {
		t.Fatal("manual source advertised discovery")
	}
}

func TestIntentContextPathPreservesLiteralScope(t *testing.T) {
	path, err := client.IntentContextPath(v1.IntentContextInput{Service: "checkout.api/v2", Source: "primary", Project: "Production Europe"})
	if err != nil || !strings.Contains(path, "service=checkout.api%2Fv2") || !strings.Contains(path, "project=Production+Europe") {
		t.Fatal(path, err)
	}
}
