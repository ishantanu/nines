package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenSLOExportIsOfflineAndReportsOmissions(t *testing.T) {
	file := "../../examples/dash0/api-availability.slo.yaml"
	var out bytes.Buffer
	if err := run([]string{"openslo", "export", "--offline", "-f", file, "--agent"}, &out); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Document struct {
			APIVersion string `json:"apiVersion"`
		}
		Warnings []string
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Document.APIVersion != "openslo/v1" || len(result.Warnings) == 0 {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err := run([]string{"openslo", "export", "--offline", "-f", file}, &out); err == nil || out.Len() != 0 {
		t.Fatal("bare export silently lost semantics", out.String(), err)
	}
	if err := run([]string{"openslo", "export", "--offline", "-f", file, "--acknowledge-omissions"}, &out); err != nil || !strings.Contains(out.String(), "apiVersion: openslo/v1") {
		t.Fatal(out.String(), err)
	}
}
