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
	if report.Mode != "fixture" || !report.Complete || report.Passed != 75 || report.Failed != 0 {
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
