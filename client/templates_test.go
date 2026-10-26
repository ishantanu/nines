package client_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestTemplateClientRendersWithoutSourceOrMutation(t *testing.T) {
	s := service.New(store.NewMemory(), nil)
	server := httptest.NewServer(httpapi.New(s, "template-client", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "default", Token: "template-client"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	catalog, err := c.SLOTemplates(ctx, "inference")
	if err != nil || len(catalog.Templates) != 3 {
		t.Fatal(catalog, err)
	}
	template, err := c.SLOTemplate(ctx, "inference-ttft")
	if err != nil || template.Template.SLI != "time-to-first-token" {
		t.Fatal(template, err)
	}
	draft, err := c.RenderSLOTemplate(ctx, template.Template.ID, v1.TemplateValues{Name: "serving-ttft"})
	if err != nil || draft.TelemetryVerified || draft.Manifest.Metadata.Name != "serving-ttft" {
		t.Fatal(draft, err)
	}
	if _, err = c.SLOTemplate(ctx, "../private"); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if _, err = c.RenderSLOTemplate(ctx, "../private", v1.TemplateValues{}); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if rows, err := s.Store.List(ctx); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
