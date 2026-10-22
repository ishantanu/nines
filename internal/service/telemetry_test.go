package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestBindingCompilationAndLineage(t *testing.T) {
	ctx := context.Background()
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	input := v1.CompileBindingInput{ExpectedRevision: 1, Name: "compiled", DisplayName: "Compiled", Target: .999, WindowSeconds: 3600}
	compiled, err := s.CompileBinding(ctx, "requests", input)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Manifest.Spec.ProviderWindow == nil || compiled.Manifest.Spec.ProviderWindow.GoodSelector != `http_requests_total{service="api",status="200"}` || compiled.Manifest.Spec.ProviderWindow.TotalSelector != `http_requests_total{service="api"}` || compiled.Definition.Spec.ObservationPoint == "" || compiled.Binding.Revision != 1 || compiled.Evidence.Fingerprint == "" {
		t.Fatal(compiled)
	}
	if _, err = s.Store.Read(ctx, "compiled"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("preview wrote an SLO", err)
	}
	d, err := model.ParseDefinition([]byte(compiled.YAML))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyDefinition(ctx, d, 0); err != nil {
		t.Fatal(err)
	}
	d.SLO.Spec.MeasurementBinding.Digest = strings.Repeat("a", 64)
	if _, err = s.PlanDefinition(ctx, d); err == nil {
		t.Fatal("forged provenance accepted")
	}
	d, err = model.ParseDefinition([]byte(compiled.YAML))
	if err != nil {
		t.Fatal(err)
	}
	d.SLO.Spec.GoodQuery = "vector(1)"
	if _, err = s.ApplyDefinition(ctx, d, 1); err == nil {
		t.Fatal("query changed under provenance")
	}
	source.Truncated = true
	if _, err = s.CompileBinding(ctx, "requests", input); err == nil {
		t.Fatal("truncated discovery compiled")
	}
	source.Truncated = false
	source.Fail = true
	if _, err = s.CompileBinding(ctx, "requests", input); !errors.Is(err, service.ErrTelemetry) || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	source.Fail = false
	b := testtelemetry.Binding()
	b.Spec.DisplayName = "New display"
	if _, err = s.ApplyDefinition(ctx, model.Definition{Binding: &b}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompileBinding(ctx, "requests", input); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale revision", err)
	}
	// An existing SLO remains linked to the immutable historical binding revision.
	record, err := s.Store.Read(ctx, "compiled")
	if err != nil || record.SLO.MeasurementBinding.Revision != 1 {
		t.Fatal(record, err)
	}
}
func TestLatencyCompilationRequiresExactBucketsForEveryPopulation(t *testing.T) {
	ctx := context.Background()
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	b := testtelemetry.Binding()
	threshold := .3
	b.Metadata.Name = "latency"
	b.Spec.Metric = "http_duration_seconds"
	b.Spec.Measurement = "latency"
	b.Spec.SuccessLabels = nil
	b.Spec.ThresholdSeconds = &threshold
	if _, err = s.ApplyDefinition(ctx, model.Definition{Binding: &b}, 0); err != nil {
		t.Fatal(err)
	}
	in := v1.CompileBindingInput{ExpectedRevision: 1, Name: "latency-slo", DisplayName: "Latency", Target: .99, WindowSeconds: 3600}
	c, err := s.CompileBinding(ctx, "latency", in)
	if err != nil || c.Manifest.Spec.ProviderWindow == nil || !strings.Contains(c.Manifest.Spec.ProviderWindow.GoodSelector, `le="0.3"`) || !strings.Contains(c.Manifest.Spec.ProviderWindow.TotalSelector, "_count") {
		t.Fatal(c, err)
	}
	source.MissingBucket = true
	if _, err = s.CompileBinding(ctx, "latency", in); err == nil {
		t.Fatal("missing exact boundary accepted")
	}
}
func TestDependencyEvidenceAndUnknownHealth(t *testing.T) {
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	g, err := s.DependencyView(ctx, "services", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 2 || g.Edges[0].Provenance != "declared" || g.Edges[1].Provenance != "observed" || g.Nodes[0].Health != "unknown" || len(g.Nodes[0].Journeys) != 1 || g.Source != "primary" || g.From == nil {
		t.Fatal(g)
	}
	source.Fail = true
	g, err = s.DependencyView(ctx, "services", 1)
	if err != nil || g.ObservationState != "unavailable" || len(g.Edges) != 1 {
		t.Fatal(g, err)
	}
	b, _ := json.Marshal(g)
	if strings.Contains(string(b), "secret") {
		t.Fatal("provider error leaked")
	}
	if _, err = s.DependencyView(ctx, "services", 2); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal(err)
	}
	bad := testtelemetry.Graph(false)
	bad.Spec.Services[0].Name = "wrong"
	bad.Spec.Dependencies[0].Caller = "wrong"
	if _, err = s.ApplyDefinition(ctx, model.Definition{DependencyMap: &bad}, 1); err == nil {
		t.Fatal("wrong SLO/service assignment accepted")
	}
	first, err := s.ListTopology(ctx, "", "", 1)
	if err != nil || first.NextCursor == "" || len(first.Items) != 1 {
		t.Fatal(first, err)
	}
	next, err := s.ListTopology(ctx, "", first.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == first.Items[0].ID {
		t.Fatal(next, err)
	}
	s.TenantID = "other"
	if _, err = s.ListTopology(ctx, "", first.NextCursor, 1); err == nil {
		t.Fatal("cross-tenant cursor accepted")
	}
}

type eventHistogramSource struct{ *testtelemetry.Source }

func (c eventHistogramSource) Discover(_ context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	return v1.TelemetryDiscovery{Metric: metric, From: at.Add(-15 * time.Minute), To: at, Metadata: []v1.TelemetryMetadata{{Type: "histogram"}}, Series: []map[string]string{{"__name__": metric + "_count", "service": "api", "status": "200"}, {"__name__": metric + "_count", "service": "api", "status": "500"}, {"__name__": metric + "_bucket", "service": "api", "status": "200", "le": "0.3"}}}, nil
}
func TestAvailabilityCompilesExplicitHistogramEventCounts(t *testing.T) {
	s, source, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	s.Collector = eventHistogramSource{source}
	b := testtelemetry.Binding()
	b.Metadata.Name = "histogram-events"
	b.Spec.Metric = "http_duration_seconds"
	b.Spec.CountFrom = "histogram"
	if _, err = s.ApplyDefinition(context.Background(), model.Definition{Binding: &b}, 0); err != nil {
		t.Fatal(err)
	}
	out, err := s.CompileBinding(context.Background(), b.Metadata.Name, v1.CompileBindingInput{ExpectedRevision: 1, Name: "http-availability", DisplayName: "HTTP availability", Target: .99, WindowSeconds: 120})
	if err != nil {
		t.Fatal(err)
	}
	if out.Manifest.Spec.ProviderWindow == nil || out.Manifest.Spec.ProviderWindow.GoodSelector != `http_duration_seconds_count{service="api",status="200"}` || out.Manifest.Spec.ProviderWindow.TotalSelector != `http_duration_seconds_count{service="api"}` {
		t.Fatal(out.Manifest.Spec)
	}
	b.Spec.CountFrom = "counter"
	if _, err = s.ApplyDefinition(context.Background(), model.Definition{Binding: &b}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompileBinding(context.Background(), b.Metadata.Name, v1.CompileBindingInput{ExpectedRevision: 2, Name: "bad", DisplayName: "Bad", Target: .99, WindowSeconds: 120}); err == nil {
		t.Fatal("histogram silently treated as a counter")
	}
}
