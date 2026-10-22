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
	"time"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentCLISelectionAndSafeOutput(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(s, "cli", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "default")
	t.Setenv("API_TOKEN", "cli")
	dir := t.TempDir()
	file := filepath.Join(dir, "intent.yaml")
	text := `apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjectiveIntent
metadata:
  name: intent-api
  project: test
spec:
  description: Successful API attempts
  service: api
  source: primary
  outcome: availability
  population:
    service: api
  target: 0.999
  windowSeconds: 3600
  noDataPolicy: unknown
`
	if err = os.WriteFile(file, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = run([]string{"intent", "plan", "-f", file}, &out); err != nil || !strings.Contains(out.String(), "needs_selection") {
		t.Fatal(out.String(), err)
	}
	dest := filepath.Join(dir, "slo.yaml")
	args := []string{"intent", "plan", "-f", file, "-binding", "requests", "-expected-revision", "1", "-out", dest}
	if err = run(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || !bytes.Contains(b, []byte("intent:")) || !bytes.Contains(b, []byte("providerWindow:")) {
		t.Fatal(string(b), err)
	}
	now := time.Now().UTC().Truncate(time.Minute).Add(-5 * time.Minute)
	out.Reset()
	if err = run([]string{"intent", "backtest", "-f", file, "-binding", "requests", "-expected-revision", "1", "-from", now.Add(-time.Hour).Format(time.RFC3339), "-to", now.Format(time.RFC3339), "-resolution", "5m"}, &out); err != nil || !strings.Contains(out.String(), "completeCutoffs") {
		t.Fatal(out.String(), err)
	}
	if err = run(args, io.Discard); err == nil {
		t.Fatal("overwrote reviewed artifact")
	}
	if err = run([]string{"intent", "plan", "-f", file, "-binding", "requests"}, io.Discard); err == nil {
		t.Fatal("implicit revision accepted")
	}
}
