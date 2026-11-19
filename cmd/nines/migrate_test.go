package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const slothFile = `version: prometheus/v1
service: svc
slos:
  - name: availability
    objective: 99.9
    sli:
      events:
        error_query: sum(rate(errs[{{.window}}]))
        total_query: sum(rate(all[{{.window}}]))
  - name: ratio
    objective: 99
    sli:
      raw:
        error_ratio_query: sum(rate(a[{{.window}}]))
`

func TestImportRunsOfflineAndReportsConvertedAndSkipped(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sloth.yaml")
	if err := os.WriteFile(file, []byte(slothFile), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if e := run([]string{"import", "sloth", "-f", file, "--offline", "--agent"}, &out); e != nil {
		t.Fatal(e)
	}
	var got struct {
		Definitions []struct{ Name, YAML string }
		Skipped     []struct{ Origin, Reason string }
	}
	if json.Unmarshal(out.Bytes(), &got) != nil || len(got.Definitions) != 1 || len(got.Skipped) != 1 || got.Definitions[0].Name != "svc-availability" || !strings.Contains(got.Definitions[0].YAML, "lifecycle: experimental") {
		t.Fatalf("%s", out.String())
	}
}

func TestImportWritesFilesWithoutOverwritingAndValidatesOptions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sloth.yaml")
	if err := os.WriteFile(file, []byte(slothFile), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	var out bytes.Buffer
	if e := run([]string{"import", "sloth", "-f", file, "--out-dir", outDir, "--offline"}, &out); e != nil {
		t.Fatal(e)
	}
	written := filepath.Join(outDir, "svc-availability.slo.yaml")
	if _, err := os.Stat(written); err != nil {
		t.Fatal(err)
	}
	if e := run([]string{"import", "sloth", "-f", file, "--out-dir", outDir, "--offline"}, &out); e == nil || !strings.Contains(e.Error(), "--force") {
		t.Fatalf("an existing file must not be overwritten silently: %v", e)
	}
	if e := run([]string{"import", "sloth", "-f", file, "--out-dir", outDir, "--force", "--offline"}, &out); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{
		{"import", "sloth", "--offline"},
		{"import", "sloth", "-f", file, "--window", "soon", "--offline"},
		{"import", "sloth", "-f", file, "--window", "90d", "--offline"},
		{"import", "sloth", "-f", file, "--lifecycle", "live", "--offline"},
		{"import", "pyrra", "-f", file, "--offline", "--agent"}, // a Sloth file is not Pyrra
	} {
		var buf bytes.Buffer
		e := run(args, &buf)
		if args[1] == "pyrra" {
			var got struct{ Definitions []any }
			if e != nil || json.Unmarshal(buf.Bytes(), &got) != nil || len(got.Definitions) != 0 {
				t.Fatalf("%v %s", e, buf.String())
			}
			continue
		}
		if e == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
