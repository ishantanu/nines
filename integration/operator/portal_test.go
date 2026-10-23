package operator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/operator"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestRealOperatorPortalPublication(t *testing.T) {
	f := realCluster(t)
	f.kubectl(nil, "create", "namespace", "nines")
	manifest, err := os.ReadFile("../../deploy/operator/portal-rbac.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.kubectl(manifest, "apply", "-f", "-")
	token := f.kubectl(nil, "-n", "nines", "create", "token", "nines-publisher", "--duration=15m")
	tokenFile := filepath.Join(t.TempDir(), "publisher-token")
	if err := os.WriteFile(tokenFile, bytes.TrimSpace(token), 0600); err != nil {
		t.Fatal(err)
	}
	api := *f.api
	api.TokenFile = tokenFile
	if _, err := api.Get(f.ctx, "PrometheusRule", "must-not-read"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatal("publisher unexpectedly has provider-rule permissions", err)
	}
	collector, err := prometheus.NewMimir(f.mimir+"/prometheus", "portal-fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	svc := service.New(st, collector)
	svc.TenantID = "commerce"
	svc.TenantName = "Commerce"
	svc.SourceKind = "mimir"
	publisher := &operator.Publisher{Tenant: "commerce", Store: st, APIFor: func(namespace string) (*operator.API, error) { return &api, nil }}
	target := rules.Target{Kind: "operator", ProviderKind: "mimir", Connection: "primary", Namespace: "team-commerce", Publisher: "portal-fixture", QueryURL: f.mimir + "/prometheus", MimirOrgID: "portal-fixture"}
	manager := &rules.Manager{Tenant: "commerce", Targets: map[string]rules.Target{"primary": target}, External: publisher}
	handler, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "commerce", Name: "Commerce", Service: svc, Rules: manager, Grants: []tenancy.Grant{{Token: "portal-owner", Role: "owner"}, {Token: "portal-viewer", Role: "viewer"}, {Token: "portal-agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	request := func(role, method, path string, input any, want int) []byte {
		t.Helper()
		var data []byte
		var match string
		if input != nil {
			data, err = json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
		}
		if path == "/api/resources/apply" {
			body := input.(map[string]any)
			data = []byte(body["manifest"].(string))
			match = strconv.Quote(fmt.Sprint(body["expectedRevision"]))
		}
		req, err := http.NewRequestWithContext(f.ctx, method, server.URL+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer portal-"+role)
		req.Header.Set("X-Nines-Tenant", "commerce")
		req.Header.Set("Content-Type", "application/json")
		if match != "" {
			req.Header.Set("Content-Type", "application/yaml")
			req.Header.Set("If-Match", match)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, body)
		}
		return body
	}
	resource := f.desired("portal-checkout", "primary")
	native, err := operator.Native(resource)
	if err != nil {
		t.Fatal(err)
	}
	slo := native.SLO.SLO()
	// Several reviewed portal changes precede the first Kubernetes publication:
	// portal revision 3 must survive a Kubernetes resource starting at generation 1.
	for revision := int64(0); revision < 3; revision++ {
		definition := model.DefinitionFromSLO(slo)
		definition.Spec.DisplayName = "Portal checkout " + strconv.FormatInt(revision, 10)
		yaml, err := operator.YAML(definition)
		if err != nil {
			t.Fatal(err)
		}
		request("owner", http.MethodPost, "/api/resources/apply", map[string]any{"manifest": string(yaml), "expectedRevision": revision}, 200)
	}
	record, err := st.Read(f.ctx, slo.ID)
	if err != nil {
		t.Fatal(err)
	}
	slo = record.SLO
	if _, err := api.Get(f.ctx, v1.SLOKind, slo.ID); err == nil {
		t.Fatal("saving a portal definition deployed rules without review")
	}
	path := "/api/slos/" + slo.ID
	var plan rules.Plan
	if err := json.Unmarshal(request("agent", http.MethodPost, path+"/rules/plan", map[string]any{}, 200), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Resource == nil || plan.SLORevision != 3 {
		t.Fatal(plan)
	}
	body := map[string]string{"expectedDigest": plan.Digest}
	request("viewer", http.MethodPost, path+"/rules/apply", body, 403)
	request("agent", http.MethodPost, path+"/rules/apply", body, 403)
	request("agent", http.MethodPost, path+"/rules/withdraw", body, 403)
	request("owner", http.MethodPost, path+"/rules/apply", map[string]string{"expectedDigest": "stale"}, 409)
	var result rules.Deployment
	if err := json.Unmarshal(request("owner", http.MethodPost, path+"/rules/apply", body, 200), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "submitted" || result.Activation.State != "accepted" || result.Evaluation.State != "unknown" {
		t.Fatal(result)
	}
	current, err := api.Get(f.ctx, v1.SLOKind, slo.ID)
	if err != nil || current.Metadata.Generation != 1 || current.Metadata.Annotations[k.PortalRevision] != "3" {
		t.Fatal(current, err)
	}
	controller := operator.Controller{API: f.api, Config: operator.Config{Tenant: "commerce", Namespace: "team-commerce", Projects: []string{"commerce"}, Sources: map[string]operator.Source{"primary": {Kind: "mimir", QueryURL: f.mimir + "/prometheus", RulerURL: f.mimir + "/prometheus", MimirOrgID: "portal-fixture", RulerNamespace: "portal-fixture"}}}}
	runCtx, stop := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- controller.Run(runCtx, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil && err != context.Canceled {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("operator did not stop")
		}
	}()
	var integration v1.IntegrationStatus
	f.wait("portal publication reconciled into current Mimir rules and output", func() bool {
		if err := json.Unmarshal(request("viewer", http.MethodGet, path+"/integration", nil, 200), &integration); err != nil {
			t.Fatal(err)
		}
		return integration.Deployment.State == "stored" && integration.Evaluation.State == "evaluating" && integration.RuleOutput != nil && integration.RuleOutput.State == "verified"
	})
	identity, err := manager.OutputIdentity(f.ctx, slo)
	if err != nil || identity.Revision != 3 || identity.ResourceUID != current.Metadata.UID {
		t.Fatal(identity, err)
	}
	// Saving another portal edit leaves the existing publication unchanged.
	definition := model.DefinitionFromSLO(slo)
	definition.Spec.Target = .995
	yaml, err := operator.YAML(definition)
	if err != nil {
		t.Fatal(err)
	}
	request("owner", http.MethodPost, "/api/resources/apply", map[string]any{"manifest": string(yaml), "expectedRevision": 3}, 200)
	old, err := api.Get(f.ctx, v1.SLOKind, slo.ID)
	if err != nil || old.Metadata.Annotations[k.PortalRevision] != "3" {
		t.Fatal("unreviewed edit published", old, err)
	}
	if err := json.Unmarshal(request("viewer", http.MethodGet, path+"/integration", nil, 200), &integration); err != nil || integration.Deployment.State != "out_of_date" {
		t.Fatal(integration, err)
	}
	if err := json.Unmarshal(request("owner", http.MethodPost, path+"/rules/plan", map[string]any{}, 200), &plan); err != nil {
		t.Fatal(err)
	}
	request("owner", http.MethodPost, path+"/rules/apply", map[string]string{"expectedDigest": plan.Digest}, 200)
	f.wait("new reviewed portal revision has current operator evidence", func() bool {
		_ = json.Unmarshal(request("viewer", http.MethodGet, path+"/integration", nil, 200), &integration)
		return integration.Revision == 4 && integration.Evaluation.State == "evaluating" && integration.RuleOutput != nil && integration.RuleOutput.State == "verified"
	})
	// Withdrawal keeps the DB definition and route until the owned Mimir finalizer
	// completes; then the explicit follow-up releases the pin.
	if err := json.Unmarshal(request("owner", http.MethodPost, path+"/rules/plan", map[string]any{}, 200), &plan); err != nil {
		t.Fatal(err)
	}
	request("owner", http.MethodPost, path+"/rules/withdraw", map[string]string{"expectedDigest": plan.Digest}, 200)
	f.wait("operator finalizer and foreground cleanup complete", func() bool {
		_, err := api.Get(f.ctx, v1.SLOKind, slo.ID)
		return err != nil && strings.Contains(err.Error(), "404")
	})
	if err := json.Unmarshal(request("owner", http.MethodPost, path+"/rules/plan", map[string]any{}, 200), &plan); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(request("owner", http.MethodPost, path+"/rules/withdraw", map[string]string{"expectedDigest": plan.Digest}, 200), &result); err != nil || result.State != "withdrawn" {
		t.Fatal(result, err)
	}
	record, err = st.Read(f.ctx, slo.ID)
	if err != nil || record.OperatorRoute != "" || model.ConfigRevision(record.SLO) != 4 {
		t.Fatal("withdraw lost definition or route was not released", record.OperatorRoute, err)
	}
}
