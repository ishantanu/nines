package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"

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
	inlinePath := filepath.Join(t.TempDir(), "inline.yaml")
	body := `apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjective
metadata: {name: inline-cli, project: test}
spec:
  displayName: API success
  service: api
  source: primary
  target: 0.999
  windowSeconds: 3600
  stepSeconds: 60
  measurement:
    type: availability
    metric: http_requests_total
    labels: {service: api}
    successLabels: {status: "200"}
    scrapeIntervalSeconds: 10
    observationPoint: Completed API requests
`
	if err = os.WriteFile(inlinePath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var verified bytes.Buffer
	if err = run([]string{"telemetry", "verify", "-f", inlinePath, "--agent"}, &verified); err != nil || !strings.Contains(verified.String(), "measurementDigest") {
		t.Fatal(verified.String(), err)
	}
	if _, err = s.Store.Read(context.Background(), "inline-cli"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("CLI verification saved SLO", err)
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
