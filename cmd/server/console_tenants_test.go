package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/workflow"
)

type consoleClient struct {
	t      *testing.T
	origin string
	http   *http.Client
	csrf   string
}

func (c *consoleClient) call(method, path, body, tenant string, want int) json.RawMessage {
	c.t.Helper()
	r, _ := http.NewRequest(method, c.origin+path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if strings.HasPrefix(body, "apiVersion") {
		r.Header.Set("Content-Type", "application/yaml")
		r.Header.Set("If-Match", `"0"`)
	}
	if c.csrf != "" && method != http.MethodGet {
		r.Header.Set("X-CSRF-Token", c.csrf)
	}
	if tenant != "" {
		r.Header.Set("X-Nines-Tenant", tenant)
	}
	res, err := c.http.Do(r)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: HTTP %d, want %d: %s", method, path, res.StatusCode, want, raw)
	}
	return raw
}

func signIn(t *testing.T, origin, user, password string) *consoleClient {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &consoleClient{t: t, origin: origin, http: &http.Client{Jar: jar}}
	var info struct {
		CSRF string `json:"csrfToken"`
	}
	_ = json.Unmarshal(c.call("GET", "/auth/session", "", "", 200), &info)
	c.csrf = info.CSRF
	c.call("POST", "/auth/local/local/login", fmt.Sprintf(`{"username":%q,"password":%q}`, user, password), "", 200)
	c.csrf = ""
	_ = json.Unmarshal(c.call("GET", "/auth/session", "", "", 200), &info)
	c.csrf = info.CSRF
	return c
}

// A platform administrator creates a tenant and its first owner, who uses it
// at once; retiring it blocks writes; deleting it needs the ID repeated,
// revokes access and deletes its data. Configured tenants cannot be changed.
func TestPlatformAdministratorsManageTenantsWithoutARestart(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	defer otel.SetMeterProvider(previous)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	config, err := identity.ParseConfig([]byte(fmt.Sprintf("publicURL: %s\nproviders:\n- id: local\n  name: Local\n  type: local\n  bootstrapUsername: admin\n  bootstrapPasswordEnv: ADMIN_PASSWORD\nplatformAdmins:\n- provider: local\n  subject: admin\n", origin)), func(string) string { return "synthetic-admin-password" })
	if err != nil {
		t.Fatal(err)
	}
	repo := identity.NewMemory()
	auth, err := identity.New(ctx, config, repo, map[string]bool{"default": true})
	if err != nil {
		t.Fatal(err)
	}
	defaultSvc := service.New(store.NewMemory(), nil)
	router, err := httpapi.NewRouter([]httpapi.Workspace{{ID: "default", Name: "Default", Service: defaultSvc, Workflows: workflow.New(defaultSvc, workflow.NewMemory()), Features: features.All()}}, log, auth)
	if err != nil {
		t.Fatal(err)
	}
	scheduler, _ := service.NewCollectionScheduler([]*service.Service{defaultSvc}, 1)
	reconciler := &tenantReconciler{repo: repo, opts: tenantOptions{memory: true, identityEnabled: true}, configured: map[string]bool{"default": true},
		prepare: func(w *httpapi.Workspace) { w.Features = features.All() }, router: router, auth: auth, scheduler: scheduler, log: log}
	defer reconciler.Close()
	deleted := []string{}
	auth.TenantsChanged = reconciler.Reconcile
	auth.DeleteTenantData = func(_ context.Context, id string) error {
		deleted = append(deleted, id)
		return nil
	}
	server.Config.Handler = router
	server.Start()
	defer server.Close()

	admin := signIn(t, origin, "admin", "synthetic-admin-password")
	// The first owner is a new local account; the administrator never sets
	// its password. The owner chooses one through the setup link.
	create := `{"reason":"Onboard the payments team","newTenant":{"id":"payments","name":"Payments","ownerUser":{"provider":"local","username":"pat","name":"Pat"}}}`
	var created identity.TenantCreated
	if err = json.Unmarshal(admin.call("POST", "/api/platform/tenants/create", create, "", 200), &created); err != nil || created.Version != 1 || created.State != identity.TenantActive || created.SetupLink == nil {
		t.Fatal("create", created, err)
	}
	_, token, _ := strings.Cut(created.SetupLink.URL, "/setup#")
	(&consoleClient{t: t, origin: origin, http: http.DefaultClient}).call("POST", "/auth/local/setup", `{"token":"`+token+`","password":"synthetic-owner-password"}`, "", 200)
	admin.call("POST", "/api/platform/tenants/create", create, "", 409)
	admin.call("POST", "/api/platform/tenants/create", `{"reason":"Shadow the default tenant","newTenant":{"id":"default","name":"Default","ownerProvider":"local","ownerSubject":"admin"}}`, "", 400)
	admin.call("POST", "/api/platform/tenants/create", `{"reason":"No owner","newTenant":{"id":"orphan","name":"Orphan"}}`, "", 400)

	// The new owner signs in and works in the tenant at once, without a restart.
	owner := signIn(t, origin, "pat", "synthetic-owner-password")
	var tenants []struct{ ID, Name, Role string }
	_ = json.Unmarshal(owner.call("GET", "/api/tenants", "", "", 200), &tenants)
	if len(tenants) != 1 || tenants[0].ID != "payments" || tenants[0].Role != "owner" {
		t.Fatal("the owner's tenants", tenants)
	}
	owner.call("POST", "/api/resources/apply", "apiVersion: nines.dev/v1beta1\nkind: ServiceLevelObjective\nmetadata: {name: checkout, project: payments}\nspec:\n  displayName: Checkout\n  service: api\n  target: 0.99\n  window: 1h\n  step: 1m\n", "payments", 200)
	owner.call("GET", "/api/slos", "", "default", 403)
	// A second tenant created at runtime is isolated from the first.
	admin.call("POST", "/api/platform/tenants/create", `{"reason":"Onboard billing","newTenant":{"id":"billing","name":"Billing","ownerProvider":"local","ownerSubject":"admin"}}`, "", 200)
	if string(admin.call("GET", "/api/slos", "", "billing", 200)) != "[]\n" {
		t.Fatal("a new tenant sees another tenant's SLOs")
	}
	admin.call("GET", "/api/slos", "", "payments", 403)
	owner.call("GET", "/api/slos", "", "billing", 403)
	owner.call("POST", "/api/platform/tenants/retire", `{"reason":"Not mine to retire","tenant":"payments","expectedVersion":1}`, "", 403)

	// Rename, with an optimistic version check.
	admin.call("POST", "/api/platform/tenants/rename", `{"reason":"Team renamed","tenant":"payments","name":"Payments platform","expectedVersion":2}`, "", 409)
	admin.call("POST", "/api/platform/tenants/rename", `{"reason":"Team renamed","tenant":"payments","name":"Payments platform","expectedVersion":1}`, "", 200)
	var workspace struct {
		Tenant struct{ Name string } `json:"tenant"`
	}
	_ = json.Unmarshal(owner.call("GET", "/api/workspace", "", "payments", 200), &workspace)
	if workspace.Tenant.Name != "Payments platform" {
		t.Fatal("rename not served", workspace)
	}

	// Configured tenants are shown but cannot be changed from the console.
	var overview struct {
		Provisioning string `json:"tenantProvisioning"`
		Tenants      []struct {
			ID, ManagedBy, State string
			Version              int64
		} `json:"tenants"`
	}
	_ = json.Unmarshal(admin.call("GET", "/api/platform/overview", "", "", 200), &overview)
	if overview.Provisioning != "platform console" || len(overview.Tenants) != 3 || overview.Tenants[1].ManagedBy != "configuration" || overview.Tenants[2].ID != "payments" || overview.Tenants[2].ManagedBy != "console" || overview.Tenants[2].Version != 2 {
		t.Fatal("overview", overview)
	}
	admin.call("POST", "/api/platform/tenants/retire", `{"reason":"Try the configured one","tenant":"default","expectedVersion":1}`, "", 409)

	// Deleting needs the tenant retired first.
	admin.call("POST", "/api/platform/tenants/delete", `{"reason":"Too early","tenant":"payments","confirm":"payments","expectedVersion":2}`, "", 409)
	admin.call("POST", "/api/platform/tenants/retire", `{"reason":"Team merged","tenant":"payments","expectedVersion":2}`, "", 200)
	owner.call("GET", "/api/slos/checkout", "", "payments", 200)
	owner.call("POST", "/api/resources/apply", "apiVersion: nines.dev/v1beta1\nkind: ServiceLevelObjective\nmetadata: {name: other, project: payments}\nspec:\n  displayName: Other\n  service: api\n  target: 0.99\n  window: 1h\n  step: 1m\n", "payments", 409)

	admin.call("POST", "/api/platform/tenants/delete", `{"reason":"Team merged","tenant":"payments","confirm":"Payments","expectedVersion":3}`, "", 400)
	admin.call("POST", "/api/platform/tenants/delete", `{"reason":"Team merged","tenant":"payments","confirm":"payments","expectedVersion":3}`, "", 200)
	if len(deleted) != 1 || deleted[0] != "payments" {
		t.Fatal("tenant data not deleted", deleted)
	}
	owner.call("GET", "/api/slos", "", "payments", 403)
	if mounted, _ := router.Mounted("payments"); mounted {
		t.Fatal("a deleted tenant is still served")
	}

	// Each lifecycle step is measured, with its outcome.
	var rm metricdata.ResourceMetrics
	if err = reader.Collect(ctx, &rm); err != nil {
		t.Fatal(err)
	}
	operations := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == "nines.operation.count" {
				for _, p := range sum.DataPoints {
					op, _ := p.Attributes.Value("nines.operation")
					outcome, _ := p.Attributes.Value("outcome")
					operations[op.AsString()+"/"+outcome.AsString()] += p.Value
				}
			}
		}
	}
	for _, want := range []string{"tenant.create/succeeded", "tenant.create/failed", "tenant.rename/succeeded", "tenant.rename/failed", "tenant.retire/succeeded", "tenant.delete/succeeded", "tenant.delete/failed"} {
		if operations[want] == 0 {
			t.Errorf("no %s operation recorded: %v", want, operations)
		}
	}

	// Every change is in the platform audit log.
	audit := string(admin.call("GET", "/api/platform/audit", "", "", 200))
	for _, action := range []string{"tenant.create", "tenant.rename", "tenant.retire", "tenant.delete"} {
		if !strings.Contains(audit, `"action":"`+action+`"`) {
			t.Fatal("missing audit", action)
		}
	}
}
