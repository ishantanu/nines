package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestTenantBootstrapPinsMimirOrgAndIgnoresLegacyCredential(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prometheus/api/v1/query_range" {
			t.Error(r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		good := 99.
		if r.Header.Get("X-Scope-OrgID") == "upstream-beta" {
			good = 80
		} else if r.Header.Get("X-Scope-OrgID") != "upstream-alpha" {
			t.Error("unexpected upstream org")
		}
		v := 100.
		if r.Form.Get("query") == "good" {
			v = good
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"values": [][2]any{{at.Unix(), strconv.FormatFloat(v, 'f', -1, 64)}}}}}})
	}))
	defer source.Close()
	t.Setenv("BOOTSTRAP_OWNER_TOKEN", "bootstrap-owner-token-more-than-24-chars")
	config := map[string]any{"tenants": []any{map[string]any{"id": "alpha", "name": "Alpha", "access": []any{map[string]any{"tokenEnv": "BOOTSTRAP_OWNER_TOKEN", "role": "owner"}}, "source": map[string]any{"kind": "mimir", "url": source.URL + "/prometheus", "mimirOrgID": "upstream-alpha"}}, map[string]any{"id": "beta", "name": "Beta", "access": []any{map[string]any{"tokenEnv": "BOOTSTRAP_OWNER_TOKEN", "role": "owner"}}, "source": map[string]any{"kind": "mimir", "url": source.URL + "/prometheus", "mimirOrgID": "upstream-beta"}}}}
	b, _ := json.Marshal(config)
	file := filepath.Join(t.TempDir(), "tenants.json")
	if err := os.WriteFile(file, b, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runtimes, err := initTenants(ctx, file, false, true, "legacy-not-a-grant")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range runtimes {
			r.Close()
		}
	}()
	for n, r := range runtimes {
		svc := r.Workspace.Service
		svc.Now = func() time.Time { return at.Add(time.Minute) }
		if svc.SourceKind != "mimir" || svc.TenantID != []string{"alpha", "beta"}[n] || len(r.Workspace.Grants) != 1 || r.Workspace.Grants[0].Token == "legacy-not-a-grant" {
			t.Fatal("bootstrap lost trusted tenant binding")
		}
		if err := svc.Create(ctx, model.SLO{ID: "same-slo", Name: "Counts", Project: "p", Service: "s", Target: .95, WindowSeconds: 60, StepSeconds: 60, GoodQuery: "good", TotalQuery: "total"}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Collect(ctx, "same-slo", at.Add(-time.Minute), at); err != nil {
			t.Fatal(err)
		}
		status, err := svc.Status(ctx, "same-slo", time.Time{})
		if err != nil || status.Good != []float64{99, 80}[n] || status.Coverage != 1 {
			t.Fatal(status, err)
		}
	}
}
func TestDemoBootstrapIgnoresExternalTenantConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "bad-database-do-not-connect")
	t.Setenv("PROMETHEUS_URL", "https://should-not-connect.invalid")
	t.Setenv("MIMIR_URL", "https://should-not-connect.invalid")
	runtimes, err := initTenants(context.Background(), "/missing/tenant-config.yaml", true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range runtimes {
			r.Close()
		}
	}()
	if len(runtimes) != 2 || runtimes[0].Workspace.Service.Store == runtimes[1].Workspace.Service.Store {
		t.Fatal("demo tenant stores alias")
	}
	for n, r := range runtimes {
		svc := r.Workspace.Service
		if svc.Collector != nil || r.Webhook != "" {
			t.Fatal("demo enabled external source")
		}
		snap, err := svc.Status(context.Background(), "checkout-success", time.Time{})
		if err != nil || snap.Reliability == nil {
			t.Fatal(snap, err)
		}
		if n == 0 && snap.Alert.Phase != "firing" {
			t.Fatal("commerce should show the synthetic fast-burn alert")
		}
		if n == 1 && *snap.Reliability < .999 {
			t.Fatal("sandbox should have independent healthy samples")
		}
	}
}

func TestNamedConnectionRoutesMeasurementsAndRulesToSameDataset(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Minute)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org := r.Header.Get("X-Scope-OrgID")
		token := r.Header.Get("Authorization")
		if org == "production-org" && token != "Bearer production-read" || org == "staging-org" && token != "Bearer staging-read" {
			t.Error("incorrect query credential")
		}
		if org != "production-org" && org != "staging-org" {
			t.Error("incorrect org", org)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		timestamp := r.Form.Get("end")
		value := 99.
		if org == "staging-org" {
			value = 80
		}
		if r.Form.Get("query") == "total" {
			value = 100
		}
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[%s,"%g"]]}]}}`, timestamp, value)
	}))
	defer upstream.Close()
	t.Setenv("OWNER_NAMED", strings.Repeat("o", 24))
	t.Setenv("PRODUCTION_READ", "production-read")
	t.Setenv("STAGING_READ", "staging-read")
	t.Setenv("PRODUCTION_RULES", "production-rules")
	config := fmt.Sprintf(`tenants:
- id: commerce
  name: Commerce
  access:
  - tokenEnv: OWNER_NAMED
    role: owner
  connections:
  - name: production
    kind: mimir
    url: %s/prometheus
    mimirOrgID: production-org
    bearerTokenEnv: PRODUCTION_READ
    rules:
      namespace: nines
      rulerURL: %s/prometheus
      bearerTokenEnv: PRODUCTION_RULES
  - name: staging
    kind: mimir
    url: %s/prometheus
    mimirOrgID: staging-org
    bearerTokenEnv: STAGING_READ
    rules:
      namespace: nines
      rulerURL: %s/prometheus
`, upstream.URL, upstream.URL, upstream.URL, upstream.URL)
	file := filepath.Join(t.TempDir(), "tenants.yaml")
	if err := os.WriteFile(file, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	runtimes, err := initTenants(context.Background(), file, false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer runtimes[0].Close()
	runtime := runtimes[0]
	svc := runtime.Workspace.Service
	svc.Now = func() time.Time { return at }
	for _, name := range []string{"production", "staging"} {
		slo := model.SLO{ID: name, Source: name, Project: "p", Service: "api", Name: name, Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "good", TotalQuery: "total"}
		if err := svc.Create(context.Background(), slo); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Collect(context.Background(), name, at.Add(-time.Minute), at); err != nil {
			t.Fatal(err)
		}
		rec, err := svc.Store.Read(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		want := 99.
		if name == "staging" {
			want = 80
		}
		if rec.Points[at.Unix()].Good != want {
			t.Fatal("wrong dataset collected", rec)
		}
		plan, err := runtime.Workspace.Rules.Plan(rec.SLO)
		if err != nil || plan.Target.MimirOrgID != name+"-org" {
			t.Fatal(plan, err)
		}
		if name == "production" && plan.Target.BearerToken != "production-rules" {
			t.Fatal("ruler reused read-only credential")
		}
	}
}
