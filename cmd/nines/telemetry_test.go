package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestTelemetryCLICommandsAgainstAPI(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(s, "cli-token", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	t.Setenv("API_TOKEN", "cli-token")
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "default")
	for _, command := range [][]string{{"telemetry", "names", "primary", "-prefix", "http_"}, {"telemetry", "inspect", "primary", "-metric", "http_requests_total"}, {"bindings", "list", "-limit", "1"}, {"dependencies", "list"}, {"dependencies", "show", "services", "-expected-revision", "1"}} {
		var out bytes.Buffer
		if err = run(command, &out); err != nil || !strings.Contains(out.String(), `"tenant": "default"`) {
			t.Fatal(command, out.String(), err)
		}
	}
	path := filepath.Join(t.TempDir(), "slo.yaml")
	args := []string{"bindings", "compile", "requests", "-expected-revision", "1", "-name", "compiled", "-out", path}
	if err = run(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(b, []byte("measurementBinding:")) {
		t.Fatal(string(b), err)
	}
	if err = run(args, io.Discard); err == nil {
		t.Fatal("overwrote existing output file")
	}
}
