package client_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentSDKPlanApplyAndExport(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(s, "sdk", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "default", Token: "sdk"})
	if err != nil {
		t.Fatal(err)
	}
	in := v1.PlanIntentInput{Intent: v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "intent-api", Project: "test"}, Spec: v1.IntentSpec{Description: "Successful API attempts", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}}
	ctx := context.Background()
	p, err := c.PlanIntent(ctx, in)
	if err != nil || p.Status != "needs_selection" {
		t.Fatal(p, err)
	}
	in.Binding = &v1.IntentSelection{Name: p.Candidates[0].Binding.Name, ExpectedRevision: p.Candidates[0].Binding.Revision}
	p, err = c.PlanIntent(ctx, in)
	if err != nil || p.Status != "ready" {
		t.Fatal(p, err)
	}
	if _, err = c.Apply(ctx, p.Compiled.Manifest, p.ExpectedRevision); err != nil {
		t.Fatal(err)
	}
	result, err := c.SLOContext(ctx, "intent-api", v1.ContextOptions{})
	if err != nil || result.Definition.Intent == nil || result.Definition.Intent.Description != in.Intent.Spec.Description {
		t.Fatal(result, err)
	}
}

func TestIntentSDKHistoricalSamplingAndBindingHealth(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(s, "sdk", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "default", Token: "sdk"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	health, err := c.CheckBinding(ctx, "requests", 1)
	if err != nil || health.State != "compatible" || health.Tenant != "default" {
		t.Fatal(health, err)
	}
	now := time.Now().UTC().Truncate(time.Minute).Add(-5 * time.Minute)
	request := v1.IntentBacktestInput{Plan: v1.PlanIntentInput{Intent: v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "api-backtest", Project: "test"}, Spec: v1.IntentSpec{Description: "API success", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}, Binding: &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}, From: now.Add(-time.Hour), To: now, ResolutionSeconds: 300}
	out, err := c.BacktestIntent(ctx, request)
	if err != nil || len(out.Points) != 12 || out.ManifestDigest == "" {
		t.Fatal(out, err)
	}
}
