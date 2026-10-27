package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestOfflineKubernetesDiscoveryNeedsNoServerOrCredentials(t *testing.T) {
	cliEnv(t)
	t.Setenv("NINES_URL", "https://invalid.example")
	t.Setenv("NINES_API_KEY", "must-not-be-loaded")
	var out bytes.Buffer
	args := []string{"dependencies", "import-kubernetes", "--offline", "--namespace", "commerce", "-f", "../../internal/kubediscovery/testdata/resources.yaml", "--agent"}
	if err := run(args, &out); err != nil {
		t.Fatal(out.String(), err)
	}
	var result v1.DependencyDiscovery
	if json.Unmarshal(out.Bytes(), &result) != nil || len(result.Manifest.Spec.Dependencies) != 2 || len(result.Evidence) != 2 {
		t.Fatal(out.String())
	}
	dest := filepath.Join(t.TempDir(), "dependencies.yaml")
	args = append(args, "--out", dest)
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = model.ParseDefinition(body); err != nil {
		t.Fatal(err)
	}
	if err = run(args, &out); err == nil {
		t.Fatal("overwrote a candidate artifact")
	}
	if err = run([]string{"dependencies", "discover", "--namespace", "commerce", "--offline"}, &out); err == nil || !strings.Contains(err.Error(), "requires a server") {
		t.Fatal("offline live discovery was not rejected", err)
	}
}
