package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func TestProviderEvidenceSDKAndMCPContractsStayInSync(t *testing.T) {
	var reads atomic.Int64
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	attainment := .95
	budget := -4.
	evidence := v1.NativeSLOResults{APIVersion: v1.APIVersion, Kind: "NativeSLOResults", Tenant: "alpha", SLOID: "availability", Revision: 2, Provider: "datadog", Source: "dd", NativeID: "synthetic", State: "observed", From: from, To: to, CheckedAt: to, Target: .99, Attainment: &attainment, BudgetRemaining: &budget, BudgetBasis: "provider_reported", Points: []v1.NativeSLOPoint{{Time: from, Attainment: &attainment}}, Warnings: []string{"Native evidence is independent from Nines."}}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer agent" {
			t.Error("fixed read-only identity escaped")
		}
		var body any
		switch r.URL.Path {
		case "/api/slos/availability/native-results":
			if r.URL.Query().Get("from") != from.Format(time.RFC3339) || r.URL.Query().Get("to") != to.Format(time.RFC3339) {
				t.Error("native range changed")
			}
			body = evidence
		case "/api/connections/dd/native-slos":
			body = v1.NativeSLOCatalog{APIVersion: v1.APIVersion, Kind: "NativeSLOCatalog", Tenant: "alpha", Source: "dd", Provider: "datadog", Items: []v1.NativeSLOSummary{{ID: "synthetic", Name: "Availability", Importable: true}}}
		case "/api/connections/dd/native-slo-draft":
			q := r.URL.Query()
			if q.Get("nativeID") != "id?account=other" || q.Get("name") != "imported" || q.Get("project") != "prod" || q.Get("service") != "api" {
				t.Error("draft query contract changed")
			}
			body = v1.NativeSLODraft{APIVersion: v1.APIVersion, Kind: "NativeSLODraft", Tenant: "alpha", Source: "dd", NativeID: q.Get("nativeID"), Manifest: "apiVersion: nines.dev/v1alpha1\n", Warnings: []string{"Review before apply."}}
		case "/api/slos/availability/integration":
			body = v1.IntegrationStatus{APIVersion: v1.APIVersion, Kind: "SLOIntegrationStatus", Tenant: "alpha", SLOID: "availability", Revision: 2, Definition: v1.IntegrationStage{State: "saved"}, Evaluation: v1.IntegrationStage{State: "unknown", Message: "Alert delivery is not verified."}}
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if e := json.NewEncoder(w).Encode(body); e != nil {
			t.Error(e)
		}
	}))
	defer api.Close()
	sdk, e := client.New(client.Config{URL: api.URL, Tenant: "alpha", Token: "agent"})
	if e != nil {
		t.Fatal(e)
	}
	sdkResults, e := sdk.NativeSLOResults(context.Background(), "availability", from, to)
	if e != nil {
		t.Fatal(e)
	}
	session, ctx := connect(t, Config{Server: api.URL, Tenant: "alpha", Token: "agent"})
	var got v1.NativeSLOResults
	dataInto(t, call(t, ctx, session, "native_slo_results", map[string]any{"id": "availability", "from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)}), &got)
	if got.Revision != sdkResults.Revision || got.Attainment == nil || *got.Attainment != *sdkResults.Attainment || got.BudgetRemaining == nil || *got.BudgetRemaining != -4 || got.BudgetBasis != "provider_reported" {
		t.Fatal("SDK/MCP evidence differs", got)
	}
	var catalog v1.NativeSLOCatalog
	dataInto(t, call(t, ctx, session, "list_native_slos", map[string]any{"id": "dd"}), &catalog)
	if len(catalog.Items) != 1 || !catalog.Items[0].Importable {
		t.Fatal(catalog)
	}
	var draft v1.NativeSLODraft
	dataInto(t, call(t, ctx, session, "draft_native_slo_import", map[string]any{"source": "dd", "nativeID": "id?account=other", "name": "imported", "project": "prod", "service": "api"}), &draft)
	if draft.Manifest == "" || len(draft.Warnings) != 1 {
		t.Fatal(draft)
	}
	var status v1.IntegrationStatus
	dataInto(t, call(t, ctx, session, "get_slo_integration", map[string]any{"id": "availability"}), &status)
	if status.Evaluation.State != "unknown" || status.Revision != 2 {
		t.Fatal(status)
	}
	tools, e := session.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"native_slo_results", "list_native_slos", "draft_native_slo_import", "get_slo_integration"} {
		found := false
		for _, tool := range tools.Tools {
			if tool.Name == name {
				found = true
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
					t.Fatal("read evidence advertised as mutation", name)
				}
			}
		}
		if !found {
			t.Fatal("missing tool", name)
		}
	}
	before := reads.Load()
	rejected(t, ctx, session, "native_slo_results", map[string]any{"id": "../beta", "from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)})
	rejected(t, ctx, session, "native_slo_results", map[string]any{"id": "availability", "from": from.Format(time.RFC3339), "to": from.Add(32 * 24 * time.Hour).Format(time.RFC3339)})
	rejected(t, ctx, session, "list_native_slos", map[string]any{"id": "dd", "tenant": "beta"})
	rejected(t, ctx, session, "draft_native_slo_import", map[string]any{"source": "dd", "nativeID": "synthetic", "name": "imported", "project": "prod", "service": "api", "endpoint": "https://other.example"})
	if reads.Load() != before {
		t.Fatal("invalid or cross-route tool reached API")
	}
	// The same contract must be exposed without enabling write or deployment tools.
	if hasTool(tools.Tools, "deploy_slo_rules") || hasTool(tools.Tools, "apply_slo") {
		t.Fatal("read-only adapter exposed writes")
	}
}
