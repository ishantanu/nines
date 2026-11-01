package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ishantanu/nines/internal/intenteval"
)

func TestFixtureReportFile(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected model call", 500)
	}))
	defer server.Close()
	t.Setenv("INTENT_MODEL_PROVIDER", "custom")
	t.Setenv("INTENT_MODEL_MODEL", "fixture-model")
	t.Setenv("INTENT_MODEL_ENDPOINT", server.URL+"/v1/chat/completions")
	path := filepath.Join(t.TempDir(), "report.json")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--out", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if calls.Load() != 0 {
		t.Fatal("fixture mode contacted the configured model")
	}
	if stdout.Len() != 0 {
		t.Fatal("file output also wrote stdout")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report intenteval.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Mode != "fixture" || !report.Complete || report.Passed != 105 || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report permissions: %v", info.Mode())
	}
}

func TestExistingOutputRefusedBeforeLiveRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "should not be reached", 500)
	}))
	defer server.Close()
	t.Setenv("INTENT_MODEL_PROVIDER", "custom")
	t.Setenv("INTENT_MODEL_MODEL", "fixture-model")
	t.Setenv("INTENT_MODEL_ENDPOINT", server.URL+"/v1/chat/completions")
	path := filepath.Join(t.TempDir(), "existing.json")
	const original = "preserve this report"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--live", "--out", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit %d", code)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || string(data) != original {
		t.Fatal("existing report overwritten or live request made")
	}
}

func TestUsageAndIncompleteExit(t *testing.T) {
	t.Setenv("INTENT_MODEL_PROVIDER", "")
	t.Setenv("INTENT_MODEL_MODEL", "")
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"--help"}, 0},
		{[]string{"extra"}, 2},
		{[]string{"--timeout", "0"}, 2},
		{[]string{"--runtime-version", "ignored"}, 2},
		{[]string{"--live", "--runtime-version", "line\nbreak"}, 2},
		{[]string{"--live"}, 1},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), tc.args, &stdout, &stderr); code != tc.want {
			t.Fatalf("%v: exit %d, want %d", tc.args, code, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := run(ctx, nil, &stdout, &stderr); code != 1 {
		t.Fatalf("cancelled run exit %d", code)
	}
	var report intenteval.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Passed != 0 {
		t.Fatalf("cancelled report passed: %+v", report)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("broken output") }
func TestOutputFailureReturnsNonzero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stderr bytes.Buffer
	if code := run(ctx, nil, failedWriter{}, &stderr); code != 1 || !bytes.Contains(stderr.Bytes(), []byte("could not write")) {
		t.Fatalf("output failure not reported: exit=%d %s", code, stderr.String())
	}
}

func TestWorkflowCaseArgumentsDoNotContactConfiguredProvider(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected model call", 500)
	}))
	defer server.Close()
	t.Setenv("INTENT_MODEL_PROVIDER", "custom")
	t.Setenv("INTENT_MODEL_MODEL", "fixture-model")
	t.Setenv("INTENT_MODEL_ENDPOINT", server.URL+"/v1/chat/completions")
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--case", "availability-reviewed-apply"}, 2},
		{[]string{"--case", "does-not-exist", "--live"}, 1},
		{[]string{"--case", "invented-quantity", "--live"}, 1},
		{[]string{"--suite", "unknown"}, 2},
		{[]string{"--suite", "workflow", "--case", "does-not-exist", "--live"}, 1},
		{[]string{"--suite", "workflow", "--case", "availability-reviewed-apply"}, 0},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), tc.args, &stdout, &stderr); code != tc.code {
			t.Fatalf("%v: exit %d, want %d: %s", tc.args, code, tc.code, stderr.String())
		}
		if tc.code == 0 {
			var report intenteval.Report
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.Passed != 5 || report.SelectedCase != "availability-reviewed-apply" {
				t.Fatalf("filtered fixture report: %+v, %v", report, err)
			}
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid arguments or fixture mode contacted the configured provider")
	}
}

func TestDeterministicTemplateSuite(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--suite", "templates"}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	var report intenteval.Report
	if json.Unmarshal(out.Bytes(), &report) != nil || report.Passed != 120 || report.Mode != "deterministic" || !report.Complete {
		t.Fatal(out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"--suite", "templates", "--live"}, &out, &stderr); code != 2 {
		t.Fatal("live model call allowed for deterministic suite", code)
	}
}

func TestTemplateSelectionCLI(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected model call", 500)
	}))
	defer server.Close()
	t.Setenv("INTENT_MODEL_PROVIDER", "custom")
	t.Setenv("INTENT_MODEL_MODEL", "test")
	t.Setenv("INTENT_MODEL_ENDPOINT", server.URL+"/v1/chat/completions")
	path := filepath.Join(t.TempDir(), "selection.json")
	var out, stderr bytes.Buffer
	args := []string{"--suite", "template-selection", "--case", "api-availability", "--out", path}
	if code := run(context.Background(), args, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report intenteval.Report
	if json.Unmarshal(data, &report) != nil || report.Passed != 5 || report.Acceptance.Status != "fixture_only" {
		t.Fatal("invalid selection report")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("report is not private")
	}
	if code := run(context.Background(), append(args, "--live"), &out, &stderr); code != 1 {
		t.Fatal("existing live output accepted")
	}
	if code := run(context.Background(), []string{"--suite", "template-selection", "--case", "unknown", "--live"}, &out, &stderr); code != 1 {
		t.Fatal("unknown live case accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("fixture or invalid request contacted configured provider")
	}
}
