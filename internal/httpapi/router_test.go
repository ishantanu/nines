package httpapi

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

const routerSLO = "apiVersion: nines.dev/v1beta1\nkind: ServiceLevelObjective\nmetadata: {name: checkout, project: shop}\nspec:\n  displayName: Checkout\n  service: api\n  target: 0.99\n  window: 1h\n  step: 1m\n"

func routerWorkspace(id, token string) Workspace {
	svc := service.New(store.NewMemory(), nil)
	return Workspace{ID: id, Name: strings.ToUpper(id), Service: svc, Workflows: workflow.New(svc, workflow.NewMemory()), Features: features.All(), Grants: []tenancy.Grant{{Token: token, Role: "owner"}}}
}

// Tenants mounted and retired while the router serves keep their isolation,
// and a retired tenant is read-only.
func TestRouterMountsAndRetiresTenantsWhileServing(t *testing.T) {
	router, err := NewRouter([]Workspace{routerWorkspace("alpha", "alpha-owner")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(token, tenant, method, path, body string) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		if body != "" {
			r.Header.Set("Content-Type", "application/yaml")
			r.Header.Set("If-Match", `"0"`)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	if code := call("beta-owner", "beta", "GET", "/api/slos", ""); code != 401 {
		t.Fatalf("an unknown tenant's token: HTTP %d", code)
	}

	// Mount while requests to the existing tenant are in flight.
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if code := call("alpha-owner", "alpha", "GET", "/api/slos", ""); code != 200 {
				t.Errorf("alpha during mount: HTTP %d", code)
			}
		})
	}
	if err = router.Mount(routerWorkspace("beta", "beta-owner")); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err = router.Mount(routerWorkspace("beta", "other")); err == nil {
		t.Fatal("a second tenant with the same ID was mounted")
	}
	if code := call("beta-owner", "beta", "POST", "/api/resources/apply", routerSLO); code != 200 {
		t.Fatalf("write to the new tenant: HTTP %d", code)
	}
	if code := call("beta-owner", "alpha", "GET", "/api/slos", ""); code != 403 {
		t.Fatalf("the new tenant's token reached another tenant: HTTP %d", code)
	}

	if err = router.Retire("beta"); err != nil {
		t.Fatal(err)
	}
	if mounted, retired := router.Mounted("beta"); !mounted || !retired {
		t.Fatalf("beta mounted=%v retired=%v", mounted, retired)
	}
	if code := call("beta-owner", "beta", "GET", "/api/slos/checkout", ""); code != 200 {
		t.Fatalf("a retired tenant must stay readable: HTTP %d", code)
	}
	if code := call("beta-owner", "beta", "POST", "/api/resources/apply", routerSLO); code != 409 {
		t.Fatalf("a retired tenant must refuse writes: HTTP %d", code)
	}
	if code := call("alpha-owner", "alpha", "POST", "/api/resources/apply", routerSLO); code != 200 {
		t.Fatalf("retiring one tenant changed another: HTTP %d", code)
	}
	if err = router.Retire("gamma"); err == nil {
		t.Fatal("retired a tenant that is not mounted")
	}
}
