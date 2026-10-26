package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishantanu/nines/internal/slotemplates"
)

func TestTenantExtensionConfigurationLoadsPrivateSnapshotAndFailsClosed(t *testing.T) {
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	lock := filepath.Join(root, "extensions.lock.yaml")
	if _, err = slotemplates.Install(lock, data, slotemplates.SHA256(data), false); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "tenants.yaml")
	text := `tenants:
  - id: alpha
    name: Alpha
    sloExtensionLock: extensions.lock.yaml
    source:
      kind: manual
    access:
      - tokenEnv: EXTENSION_TEST_TOKEN
        role: owner
  - id: beta
    name: Beta
    source:
      kind: manual
    access:
      - tokenEnv: EXTENSION_TEST_TOKEN
        role: owner
`
	t.Setenv("EXTENSION_TEST_TOKEN", "extension-fixture-token-longer-than-24")
	if err = os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	runtimes, err := initTenants(context.Background(), config, false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimes) != 2 {
		t.Fatal(len(runtimes))
	}
	for _, r := range runtimes {
		catalog, err := r.Workspace.Templates.List("")
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if r.Workspace.ID == "alpha" {
			want = 1
		}
		if len(catalog.Extensions) != want {
			t.Fatal("extension catalog crossed tenants")
		}
		r.Close()
	}
	if err = os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if runtimes, err = initTenants(context.Background(), config, false, true, ""); err == nil {
		for _, r := range runtimes {
			r.Close()
		}
		t.Fatal("missing configured extension lock did not fail startup")
	}
}
