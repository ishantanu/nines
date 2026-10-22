package mcpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestTelemetryToolsUseTypedScopedEvidence(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(s, "viewer", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "default", Token: "viewer"})
	for name, input := range map[string]any{"plan_slo_intent": v1.PlanIntentInput{Intent: v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "intent-api", Project: "test"}, Spec: v1.IntentSpec{Description: "API success", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}, Binding: &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}, "discover_metric_names": map[string]any{"source": "primary", "prefix": "http_"}, "discover_telemetry": map[string]any{"source": "primary", "metric": "http_requests_total"}, "list_topology_resources": map[string]any{"limit": 1}, "service_dependency_map": map[string]any{"id": "services"}, "compile_measurement_binding": map[string]any{"id": "requests", "options": v1.CompileBindingInput{ExpectedRevision: 1, Name: "compiled", DisplayName: "Compiled", Target: .999, WindowSeconds: 3600}}} {
		r := call(t, ctx, session, name, input)
		b, _ := json.Marshal(r.StructuredContent)
		var out struct {
			Tenant string `json:"tenant"`
			Data   struct {
				Tenant  string `json:"tenant"`
				Version string `json:"apiVersion"`
			} `json:"data"`
		}
		if json.Unmarshal(b, &out) != nil || out.Tenant != "default" || out.Data.Tenant != "default" || out.Data.Version != v1.APIVersion {
			t.Fatal(name, string(b))
		}
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "service_dependency_map", Arguments: map[string]any{"id": "services", "tenant": "beta"}})
	if err == nil && !bad.IsError {
		t.Fatal("tenant override accepted")
	}
}

func TestIntentBacktestAndHealthMCPTools(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(s, "token", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer api.Close()
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "default", Token: "token"})
	call(t, ctx, session, "check_binding_health", map[string]any{"id": "requests", "expectedRevision": 1})
	now := time.Now().UTC().Truncate(time.Minute).Add(-5 * time.Minute)
	input := v1.IntentBacktestInput{Plan: v1.PlanIntentInput{Intent: v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "api-backtest", Project: "test"}, Spec: v1.IntentSpec{Description: "API success", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}, Binding: &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}, From: now.Add(-time.Hour), To: now, ResolutionSeconds: 300}
	call(t, ctx, session, "backtest_slo_intent", input)
	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "backtest_slo_intent", Arguments: map[string]any{"plan": input.Plan, "from": input.From, "to": input.To, "resolutionSeconds": 300, "tenant": "other"}})
	if err == nil && !denied.IsError {
		t.Fatal("tenant override accepted")
	}
}
