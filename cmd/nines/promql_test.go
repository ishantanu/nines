package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishantanu/nines/examples"
)

func TestOfflineCLIRejectsInvalidPromQLBeforeGeneratingArtifacts(t *testing.T) {
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(file, []byte(examples.APIAvailability), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"validate", "--offline"}, {"rules", "generate", "--offline", "--tenant", "commerce"}} {
		for _, query := range []string{"sum(", "42", "up[5m]"} {
			args := append(append([]string{}, command...), "-f", file, "--good-query", query)
			var output bytes.Buffer
			if err := run(args, &output); err == nil || !strings.Contains(err.Error(), "goodQuery") || output.Len() != 0 {
				t.Fatalf("invalid query produced output: %v %s", err, output.String())
			}
		}
	}
	var output bytes.Buffer
	if err := run([]string{"validate", "--offline", "-f", file, "--agent"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "PromQL types and generated expressions validated offline") {
		t.Fatal("validation receipt omits its checked scope", output.String())
	}
}
