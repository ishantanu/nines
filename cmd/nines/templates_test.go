package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestTemplateCLIIsOfflineAndMachineReadable(t *testing.T) {
	t.Setenv("NINES_URL", "https://unreachable.invalid")
	t.Setenv("NINES_CONFIG", filepath.Join(t.TempDir(), "missing-config"))
	var out bytes.Buffer
	if err := run([]string{"--offline", "--agent", "templates", "list", "--category", "agent"}, &out); err != nil {
		t.Fatal(err)
	}
	var catalog v1.SLOTemplateCatalog
	if json.Unmarshal(out.Bytes(), &catalog) != nil || len(catalog.Templates) != 4 {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := run([]string{"--offline", "templates", "generate", "inference-ttft", "--name", "serving-ttft"}, &out); err != nil {
		t.Fatal(err)
	}
	d, err := model.ParseSLODefinition(out.Bytes())
	if err != nil || d.Metadata.Name != "serving-ttft" || d.Spec.Measurement.Type != "latency" || d.Spec.Lifecycle != v1.LifecycleExperimental {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err = run([]string{"--offline", "--agent", "templates", "generate", "agent-task-completion"}, &out); err != nil {
		t.Fatal(err)
	}
	var draft v1.SLOTemplateDraft
	if json.Unmarshal(out.Bytes(), &draft) != nil || draft.TelemetryVerified || len(draft.TemplateDigest) != 64 {
		t.Fatal(out.String())
	}
	path := filepath.Join(t.TempDir(), "task.yaml")
	out.Reset()
	args := []string{"--offline", "--agent", "templates", "generate", "agent-task-completion", "--out", path}
	if err = run(args, &out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = model.ParseSLODefinition(body); err != nil {
		t.Fatal(err)
	}
	if err = run(args, &out); err == nil {
		t.Fatal("overwrote existing file")
	}
}
func TestTemplateCLIRejectsAmbiguousValues(t *testing.T) {
	for _, body := range []string{"source: primary\nsource: other\n", "tenant: other\n", "target: 1\n", "{}\n---\n{}\n"} {
		file := filepath.Join(t.TempDir(), "values.yaml")
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := run([]string{"--offline", "templates", "generate", "agent-task-completion", "--values", file}, &out); err == nil {
			t.Fatal("accepted invalid values", body)
		}
	}
}
