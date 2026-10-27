package dependencies_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func TestRealKubernetesDiscovery(t *testing.T) {
	origin := os.Getenv("NINES_DISCOVERY_TEST_URL")
	if origin == "" {
		t.Skip("run make test-dependencies for the disposable Kubernetes fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	newClient := func(tenant, token string) *client.Client {
		t.Helper()
		c, e := client.New(client.Config{URL: origin, Tenant: tenant, Token: token})
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	owner := newClient("commerce", os.Getenv("NINES_DISCOVERY_TEST_OWNER"))
	viewer := newClient("commerce", os.Getenv("NINES_DISCOVERY_TEST_VIEWER"))
	other := newClient("other", os.Getenv("NINES_DISCOVERY_TEST_OTHER"))
	input := v1.DependencyDiscoveryInput{Mode: "cluster", Namespace: "commerce", Name: "commerce-services", Project: "production", DisplayName: "Commerce services"}
	result, err := viewer.DiscoverDependencies(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tenant != "commerce" || len(result.Manifest.Spec.Services) != 3 || len(result.Manifest.Spec.Dependencies) != 2 || len(result.InventoryDigest) != 64 {
		t.Fatalf("unexpected discovery counts: nodes %d edges %d", len(result.Manifest.Spec.Services), len(result.Manifest.Spec.Dependencies))
	}
	encoded, _ := json.Marshal(result)
	for _, private := range []string{"postgres://", "private-cache-reference", "not-for-discovery", "example.invalid"} {
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatal("workload details escaped projection")
		}
	}
	for _, edge := range result.Manifest.Spec.Dependencies {
		if edge.Critical {
			t.Fatal("discovery inferred criticality")
		}
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "not observed traffic") {
		t.Fatal("evidence limitation missing")
	}
	// Preview is read-only even for a viewer; no topology object exists yet.
	request := func(path string, want int) {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, origin+path, nil)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("NINES_DISCOVERY_TEST_VIEWER"))
		req.Header.Set("X-Nines-Tenant", "commerce")
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("%s returned %d want %d", path, resp.StatusCode, want)
		}
	}
	request("/api/dependency-maps/commerce-services/view", http.StatusNotFound)
	rejected := input
	rejected.Namespace = "nines"
	_, err = viewer.DiscoverDependencies(ctx, rejected)
	expectStatus(t, err, http.StatusBadRequest)
	_, err = other.DiscoverDependencies(ctx, input)
	expectStatus(t, err, http.StatusBadRequest)
	for _, n := range result.Manifest.Spec.Services {
		if n.Name == "api" && n.DisplayName != "api" {
			t.Fatal("identity mapping lost")
		}
	}
	body, e := yaml.Marshal(result.Manifest)
	if e != nil {
		t.Fatal(e)
	}
	_, err = viewer.ApplyYAML(ctx, body, 0)
	expectStatus(t, err, http.StatusForbidden)
	planned, e := owner.PlanYAML(ctx, body)
	if e != nil || planned.ExpectedRevision != 0 {
		t.Fatal("planning failed", e)
	}
	revision, e := owner.ApplyYAML(ctx, body, 0)
	if e != nil || revision.Revision != 1 {
		t.Fatal("apply failed", e)
	}
	result.Manifest.Spec.Services[0].Position = &v1.GraphPosition{X: 80, Y: 240}
	body, e = yaml.Marshal(result.Manifest)
	if e != nil {
		t.Fatal(e)
	}
	revision, e = owner.ApplyYAML(ctx, body, 1)
	if e != nil || revision.Revision != 2 {
		t.Fatal("layout revision failed", e)
	}
	_, err = owner.ApplyYAML(ctx, body, 1)
	expectStatus(t, err, http.StatusConflict)
	request("/api/dependency-maps/commerce-services/view", http.StatusOK)
	container := os.Getenv("NINES_DISCOVERY_TEST_CONTAINER")
	if !strings.HasPrefix(container, "nines-discovery-acceptance-") {
		t.Fatal("fixture container identity missing")
	}
	// The actual shipped account can list inventory but cannot read Secrets or write workloads.
	for _, args := range [][]string{{"get", "secrets"}, {"create", "deployments.apps"}, {"list", "services", "-n", "nines"}} {
		command := append([]string{"exec", container, "kubectl", "auth", "can-i", "--as=system:serviceaccount:nines:nines-discovery", "-n", "commerce"}, args...)
		output, _ := exec.CommandContext(ctx, "docker", command...).CombinedOutput()
		if strings.TrimSpace(string(output)) != "no" {
			t.Fatal("excessive fixture RBAC", args)
		}
	}
	if output, e := exec.CommandContext(ctx, "docker", "exec", container, "kubectl", "-n", "commerce", "delete", "rolebinding", "nines-discovery").CombinedOutput(); e != nil {
		t.Fatal("revocation fixture failed", e, string(output))
	}
	_, err = viewer.DiscoverDependencies(ctx, input)
	expectStatus(t, err, http.StatusServiceUnavailable)
}
func expectStatus(t *testing.T, err error, want int) {
	t.Helper()
	var apiError *client.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != want {
		t.Fatalf("expected HTTP %d; got %v", want, err)
	}
}
