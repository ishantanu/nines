package operator_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ishantanu/nines/internal/operator"
)

// The credential gateway is controlled by this test; the Kubernetes API and
// ruler behind it are real disposable services from run.sh.
func TestRealOperatorCleanupOutageAndCredentialRotation(t *testing.T) {
	f := realCluster(t)
	container := os.Getenv("NINES_OPERATOR_TEST_MIMIR_CONTAINER")
	if !strings.HasPrefix(container, "nines-operator-acceptance-") {
		t.Fatal("missing disposable ruler identity")
	}
	owned, e := exec.CommandContext(f.ctx, "docker", "inspect", "--format", `{{index .Config.Labels "nines.acceptance"}}`, container).Output()
	if e != nil || strings.TrimSpace(string(owned)) != "operator" {
		t.Fatal("refusing non-fixture ruler")
	}
	upstream, e := url.Parse(f.mimir)
	if e != nil {
		t.Fatal(e)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "fixture upstream unavailable", http.StatusServiceUnavailable)
	}
	var rotated atomic.Bool
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "Bearer initial-fixture-token"
		if rotated.Load() {
			expected = "Bearer rotated-fixture-token"
		}
		if r.Header.Get("Authorization") != expected {
			http.Error(w, "credential rejected", http.StatusUnauthorized)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer gateway.Close()
	token := filepath.Join(t.TempDir(), "ruler-token")
	if e = os.WriteFile(token, []byte("initial-fixture-token"), 0600); e != nil {
		t.Fatal(e)
	}
	source := operator.Source{Kind: "mimir", QueryURL: f.mimir + "/prometheus", RulerURL: gateway.URL + "/prometheus", MimirOrgID: "cleanup-failure", RulerNamespace: "original-route", RulerBearerTokenFile: token}
	c := &operator.Controller{API: f.api, Config: operator.Config{Namespace: "team-commerce", Tenant: "commerce", Projects: []string{"commerce"}, Sources: map[string]operator.Source{"primary": source}}}
	f.apply(f.desired("cleanup-failure", "primary"))
	f.wait("owned group accepted before failure injection", func() bool {
		_ = c.Reconcile(f.ctx, f.get("slo", "cleanup-failure"))
		return isReady(f.get("slo", "cleanup-failure"), "RulesAccepted")
	})
	saved := f.get("slo", "cleanup-failure")
	if len(saved.Metadata.Finalizers) != 1 || saved.Status.ManagedGroup == "" {
		t.Fatal("cleanup ownership not persisted")
	}
	originalDigest := saved.Status.DestinationDigest
	f.kubectl(nil, "delete", "slo", "cleanup-failure", "-n", "team-commerce", "--wait=false")
	paused := false
	defer func() {
		if paused {
			_ = exec.CommandContext(context.Background(), "docker", "unpause", container).Run()
		}
	}()
	if e = exec.CommandContext(f.ctx, "docker", "pause", container).Run(); e != nil {
		t.Fatal(e)
	}
	paused = true
	if e = c.Reconcile(f.ctx, f.get("slo", "cleanup-failure")); e == nil {
		t.Fatal("provider outage reported successful cleanup")
	}
	assertBlocked := func() {
		t.Helper()
		r := f.get("slo", "cleanup-failure")
		if len(r.Metadata.Finalizers) != 1 || r.Status.Conditions[0].Reason != "CleanupBlocked" || r.Status.DestinationDigest != originalDigest || r.Status.ManagedGroup != saved.Status.ManagedGroup {
			t.Fatal("failure lost original cleanup intent")
		}
	}
	assertBlocked()
	if e = exec.CommandContext(f.ctx, "docker", "unpause", container).Run(); e != nil {
		t.Fatal(e)
	}
	paused = false
	// The provider is back, but its accepted credential has rotated. The stale
	// mounted file must keep deletion pending, without changing its destination.
	rotated.Store(true)
	if e = c.Reconcile(f.ctx, f.get("slo", "cleanup-failure")); e == nil {
		t.Fatal("stale credential allowed cleanup")
	}
	assertBlocked()
	if e = os.WriteFile(token, []byte("rotated-fixture-token"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = c.Reconcile(f.ctx, f.get("slo", "cleanup-failure")); e != nil {
		t.Fatal("rotated credential was not reread", e)
	}
	f.wait("finalizer retry removed the owned resource", func() bool {
		_, e := f.api.Get(f.ctx, "ServiceLevelObjective", "cleanup-failure")
		return e != nil && strings.Contains(e.Error(), "404")
	})
	request, e := http.NewRequestWithContext(f.ctx, http.MethodGet, f.mimir+"/prometheus/config/v1/rules/original-route/"+url.PathEscape(saved.Status.ManagedGroup), nil)
	if e != nil {
		t.Fatal(e)
	}
	request.Header.Set("X-Scope-OrgID", "cleanup-failure")
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatal("original owned ruler group remains", response.StatusCode)
	}
	// Status never exposes mounted credentials, including failure conditions.
	b, _ := json.Marshal(saved.Status)
	for _, secret := range []string{"initial-fixture-token", "rotated-fixture-token"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("credential escaped into status")
		}
	}
	t.Log("Real provider outage and rotated credentials preserved finalizer, UID-owned group and original route; retry completed cleanup after mounted token rotation")
}
