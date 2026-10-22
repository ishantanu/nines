package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStartupFailureDoesNotLogRawConfiguration(t *testing.T) {
	if path := os.Getenv("NINES_TEST_INVALID_AUTH_FILE"); path != "" {
		flag.CommandLine = flag.NewFlagSet("nines-server", flag.ExitOnError)
		os.Args = []string{"nines-server", "--memory", "--auth-file", path}
		main()
		return
	}
	const sensitive = "private-config-value-must-not-be-logged"
	path := filepath.Join(t.TempDir(), "invalid-auth.yaml")
	if err := os.WriteFile(path, []byte("sessionHours: "+sensitive+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupFailureDoesNotLogRawConfiguration$")
	cmd.Env = []string{"NINES_TEST_INVALID_AUTH_FILE=" + path, "OTEL_LOGS_EXPORTER=none", "OTEL_METRICS_EXPORTER=none", "OTEL_TRACES_EXPORTER=none"}
	output, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("startup must fail with exit 1: %v %s", err, output)
	}
	if bytes.Contains(output, []byte(sensitive)) || bytes.Contains(output, []byte(path)) || !bytes.Contains(output, []byte("server stopped")) {
		t.Fatalf("startup failure must use a fixed diagnostic: %s", output)
	}
}
