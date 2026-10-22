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
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestPublicReadClientBoundedSearchContextAndCAS(t *testing.T) {
	s := service.New(store.NewMemory(), nil)
	ctx := context.Background()
	for _, id := range []string{"checkout", "checkout-two"} {
		if err := s.Create(ctx, model.SLO{ID: id, Project: "commerce", Service: "api", Name: "Checkout", Target: .99, WindowSeconds: 600, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
	}
	h, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.SearchResources(ctx, v1.SearchOptions{Limit: 1, Query: "checkout"})
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	next, err := c.SearchResources(ctx, v1.SearchOptions{Limit: 1, Query: "checkout", Cursor: first.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == first.Items[0].ID {
		t.Fatal(next, err)
	}
	out, err := c.SLOContext(ctx, "checkout", v1.ContextOptions{ExpectedRevision: 1})
	if err != nil || out.Tenant != "alpha" || out.Health != "unknown" {
		t.Fatal(out, err)
	}
	_, err = c.SLOContext(ctx, "checkout", v1.ContextOptions{ExpectedRevision: 2})
	var conflict *client.APIError
	if !errors.As(err, &conflict) || !conflict.Conflict() {
		t.Fatal(err)
	}
	for _, id := range []string{"../beta", "checkout?tenant=beta"} {
		if _, err = c.SLOContext(ctx, id, v1.ContextOptions{}); err == nil {
			t.Fatal("unsafe id accepted")
		}
	}
}
func TestPublicContextRejectsWrongScopeAndOversizedResponse(t *testing.T) {
	for _, body := range []string{`{"tenant":"beta","apiVersion":"nines.dev/v1alpha1","resource":{"id":"private-beta"}}`, strings.Repeat("x", (256<<10)+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) }))
		c, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha"})
		_, err := c.SLOContext(context.Background(), "checkout", v1.ContextOptions{})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "private-beta") {
			t.Fatal(err)
		}
	}
}
