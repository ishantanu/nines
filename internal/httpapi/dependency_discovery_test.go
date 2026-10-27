package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/kubediscovery"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

type discoveryFixture struct {
	inventory v1.KubernetesInventory
	calls     int
}

func (f *discoveryFixture) Namespaces() []string { return []string{"commerce"} }
func (f *discoveryFixture) Read(context.Context, string) (v1.KubernetesInventory, error) {
	f.calls++
	return f.inventory, nil
}
func TestDependencyDiscoveryIsReadOnlyTenantBoundAndPositionsAreRevisioned(t *testing.T) {
	body, err := os.ReadFile("../kubediscovery/testdata/resources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := kubediscovery.Project(body, "commerce")
	if err != nil {
		t.Fatal(err)
	}
	reader := &discoveryFixture{inventory: inventory}
	a, b := service.New(store.NewMemory(), nil), service.New(store.NewMemory(), nil)
	a.KubernetesDiscovery = reader
	handler, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: a, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}}, {ID: "beta", Name: "Beta", Service: b, Grants: []tenancy.Grant{{Token: "beta", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(tenant, token, method, path string, body []byte, match string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		r.Header.Set("Content-Type", "application/json")
		if strings.HasPrefix(path, "/api/resources/") {
			r.Header.Set("Content-Type", "application/yaml")
			r.Header.Set("If-Match", match)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s tenant %s: %d want %d %s", method, path, tenant, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	input := v1.DependencyDiscoveryInput{Mode: "cluster", Namespace: "commerce", Name: "discovered", Project: "production", DisplayName: "Discovered services"}
	encode := func(in v1.DependencyDiscoveryInput) []byte {
		b, e := json.Marshal(in)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	response := request("alpha", "viewer", "POST", "/api/dependency-discovery/kubernetes", encode(input), "", 200)
	var result v1.DependencyDiscovery
	if json.Unmarshal(response, &result) != nil || result.Tenant != "alpha" || reader.calls != 1 {
		t.Fatal(string(response), reader.calls)
	}
	if _, err = a.ReadTopology(context.Background(), v1.DependencyMapKind, "discovered", 0); err != model.ErrNotFound {
		t.Fatal("preview saved a map", err)
	}
	invalidIdentity := input
	invalidIdentity.Name = "../invalid"
	request("alpha", "viewer", "POST", "/api/dependency-discovery/kubernetes", encode(invalidIdentity), "", 400)
	input.Namespace = "other"
	request("alpha", "viewer", "POST", "/api/dependency-discovery/kubernetes", encode(input), "", 400)
	input.Namespace = "commerce"
	request("beta", "beta", "POST", "/api/dependency-discovery/kubernetes", encode(input), "", 400)
	request("beta", "viewer", "GET", "/api/dependency-discovery/kubernetes", nil, "", 403)
	if reader.calls != 1 {
		t.Fatal("unauthorized namespace/tenant reached cluster")
	}
	input.Mode, input.Inventory = "resources", &inventory
	request("alpha", "agent", "POST", "/api/dependency-discovery/kubernetes", encode(input), "", 200)
	unsafe := strings.TrimSuffix(string(encode(input)), "}") + `,"token":"not-accepted"}`
	request("alpha", "viewer", "POST", "/api/dependency-discovery/kubernetes", []byte(unsafe), "", 400)
	result.Manifest.Spec.Services[0].Position = &v1.GraphPosition{X: 80, Y: 120}
	definition, err := yaml.Marshal(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	request("alpha", "viewer", "POST", "/api/resources/plan", definition, "", 200)
	request("alpha", "viewer", "POST", "/api/resources/apply", definition, `"0"`, 403)
	request("alpha", "agent", "POST", "/api/resources/apply", definition, `"0"`, 403)
	request("alpha", "owner", "POST", "/api/resources/apply", definition, `"0"`, 200)
	result.Manifest.Spec.Services[0].Position.X = 320
	definition, err = yaml.Marshal(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	request("alpha", "owner", "POST", "/api/resources/apply", definition, `"0"`, 409)
	request("alpha", "owner", "POST", "/api/resources/apply", definition, `"1"`, 200)
	graph := request("alpha", "viewer", "GET", "/api/dependency-maps/discovered/view", nil, "", 200)
	var view v1.DependencyView
	if json.Unmarshal(graph, &view) != nil || view.Revision != 2 || view.Nodes[0].Position.X != 320 {
		t.Fatal(string(graph))
	}
	old, err := a.ReadTopology(context.Background(), v1.DependencyMapKind, "discovered", 1)
	if err != nil || !strings.Contains(string(old.Manifest), `"x":80`) {
		t.Fatal("prior position revision lost", old, err)
	}
	result.Manifest.Spec.Services[0].Position.X = 10001
	invalid, err := yaml.Marshal(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	request("alpha", "owner", "POST", "/api/resources/plan", invalid, "", 400)
}
