package topology_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestRealInstrumentedDependencyEvidence(t *testing.T) {
	origin := os.Getenv("NINES_TOPOLOGY_TEST_URL")
	if origin == "" {
		t.Skip("run make test-topology for the disposable instrumented HTTP chain")
	}
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "" {
		t.Fatal("use the disposable loopback provider")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	collector, e := prometheus.New(origin, "")
	if e != nil {
		t.Fatal(e)
	}
	svc := service.New(store.NewMemory(), collector)
	svc.TenantID = "commerce"
	other := service.New(store.NewMemory(), nil)
	other.TenantID = "other"
	handler, e := httpapi.NewMulti([]httpapi.Workspace{{ID: "commerce", Name: "Commerce", Service: svc, Grants: []tenancy.Grant{{Token: "fixture-viewer", Role: "viewer"}, {Token: "fixture-owner", Role: "owner"}}}, {ID: "other", Name: "Other", Service: other, Grants: []tenancy.Grant{{Token: "other-viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	owner, e := client.New(client.Config{URL: server.URL, Tenant: "commerce", Token: "fixture-owner"})
	if e != nil {
		t.Fatal(e)
	}
	viewer, e := client.New(client.Config{URL: server.URL, Tenant: "commerce", Token: "fixture-viewer"})
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"gateway.slo.yaml", "checkout.slo.yaml", "payments.slo.yaml", "journey.yaml", "dependencies.yaml"} {
		body, e := os.ReadFile("../../examples/service-graph/" + name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = owner.ApplyYAML(ctx, body, 0); e != nil {
			t.Fatal("example did not apply", name, e)
		}
	}
	wait := func(label string, condition func(v1.DependencyView) bool) v1.DependencyView {
		t.Helper()
		for {
			view, e := viewer.DependencyMap(ctx, "checkout-services", 1)
			if e == nil && condition(view) {
				t.Log("Verified:", label)
				return view
			}
			select {
			case <-ctx.Done():
				t.Fatal("dependency fixture timed out", label, e)
			case <-time.After(time.Second):
			}
		}
	}
	view := wait("real outbound requests, two observed links and actual sample timestamps", func(v v1.DependencyView) bool {
		if v.MatchedDeclaredEdges != 2 {
			return false
		}
		count := 0
		for _, edge := range v.Edges {
			if edge.Provenance == "observed" && edge.Requests != nil && *edge.Requests > 0 && edge.Evidence != nil && edge.Evidence.Freshness == "fresh" && edge.Evidence.Samples != nil && edge.Evidence.Samples.SampleContinuity != nil {
				count++
			}
		}
		return count == 2
	})
	if view.InstrumentationCoverage != "unknown" {
		t.Fatal("counter presence invented instrumentation coverage")
	}
	linkedSteps := 0
	for _, node := range view.Nodes {
		for _, journey := range node.Journeys {
			linkedSteps += len(journey.Steps)
		}
	}
	if linkedSteps != 3 {
		t.Fatal("missing service/SLO/journey step navigation", linkedSteps)
	}
	// A credential for another tenant cannot read this topology or select its source.
	foreign, e := client.New(client.Config{URL: server.URL, Tenant: "other", Token: "fixture-viewer"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = foreign.DependencyMap(ctx, "checkout-services", 0); e == nil {
		t.Fatal("cross-tenant graph leaked")
	}
	container := os.Getenv("NINES_TOPOLOGY_TEST_CHECKOUT")
	if !strings.HasPrefix(container, "nines-topology-acceptance-") {
		t.Fatal("refusing non-fixture workload")
	}
	owned, e := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "nines.acceptance"}}`, container).Output()
	if e != nil || strings.TrimSpace(string(owned)) != "topology" {
		t.Fatal("not our instrumented fixture")
	}
	if e = exec.CommandContext(ctx, "docker", "pause", container).Run(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = exec.CommandContext(context.Background(), "docker", "unpause", container).Run() }()
	stale := wait("paused exporter leaves a stale edge with qualified continuity", func(v v1.DependencyView) bool {
		for _, edge := range v.Edges {
			if edge.Provenance == "observed" && edge.Caller == "checkout" && edge.Callee == "payments" && edge.Evidence != nil && edge.Evidence.Freshness == "stale" {
				return true
			}
		}
		return false
	})
	if stale.ObservedAt.Before(view.ObservedAt) {
		t.Fatal("observation clock moved backwards")
	}
	// Keep a portable sanitized receipt for the test log, not private provider URLs.
	receipt, _ := json.Marshal(map[string]any{"tenant": stale.Tenant, "matchedDeclaredLinks": stale.MatchedDeclaredEdges, "instrumentationCoverage": stale.InstrumentationCoverage, "boundedNodes": len(stale.Nodes)})
	t.Log(fmt.Sprint(string(receipt)))
}
