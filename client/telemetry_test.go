package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestTelemetrySDKVerticalSlice(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(s, "sdk-token", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "default", Token: "sdk-token"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inline := v1.NewSLO("inline-sdk", "test", v1.SLOSpec{DisplayName: "API success", Service: "api", Source: "primary", Target: .999, WindowSeconds: 3600, StepSeconds: 60, Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "API completions"}})
	body, _ := json.Marshal(inline)
	verified, e := c.CompileMeasurement(ctx, body)
	if e != nil || verified.Manifest.Spec.Measurement == nil || verified.MeasurementDigest == "" || strings.Contains(verified.YAML, "measurementBinding:") {
		t.Fatal(verified, e)
	}
	if _, e = s.Store.Read(ctx, "inline-sdk"); !errors.Is(e, model.ErrNotFound) {
		t.Fatal("verification saved resource", e)
	}
	if _, e = c.Plan(ctx, verified.Manifest); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Apply(ctx, verified.Manifest, 0); e != nil {
		t.Fatal(e)
	}
	if names, err := c.DiscoverMetricNames(ctx, "primary", "http_"); err != nil || len(names.Names) != 2 {
		t.Fatal(names, err)
	}
	if d, err := c.DiscoverTelemetry(ctx, "primary", "http_requests_total", map[string]string{"service": "api"}); err != nil || d.Fingerprint == "" {
		t.Fatal(d, err)
	}
	page, err := c.TopologyResources(ctx, "", "", 1)
	if err != nil || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	graph, err := c.DependencyMap(ctx, "services", 1)
	if err != nil || len(graph.Nodes) != 2 || len(graph.Nodes[1].PotentialImpact) != 1 {
		t.Fatal(graph, err)
	}
	compiled, err := c.CompileBinding(ctx, "requests", v1.CompileBindingInput{ExpectedRevision: 1, Name: "compiled", DisplayName: "Compiled", Target: .999, WindowSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Plan(ctx, compiled.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Apply(ctx, compiled.Manifest, 0); err != nil {
		t.Fatal(err)
	}
	context, err := c.SLOContext(ctx, "compiled", v1.ContextOptions{})
	if err != nil || context.Definition.MeasurementBinding == nil || context.Definition.MeasurementBinding.Revision != 1 {
		t.Fatal(context, err)
	}
}
