package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestIntentContextCLIUsesReadOnlyPinnedScope(t *testing.T) {
	cliEnv(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/intent-context" || r.URL.Query().Get("service") != "api" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("unexpected request", r.Method, r.URL, r.Header.Get("X-Nines-Tenant"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v1.IntentContext{APIVersion: v1.APIVersion, Tenant: "alpha", Objectives: []v1.SLOContext{}, Warnings: []string{"saved_evidence_only_no_upstream_requests"}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	args := []string{"--agent", "--server", server.URL, "--tenant", "alpha", "intent", "context", "--service", "api", "--source", "primary", "--project", "p"}
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !bytes.Contains(out.Bytes(), []byte("saved_evidence_only")) {
		t.Fatal(calls, out.String())
	}
	if err := run(append(args, "--journey", "checkout"), &out); err == nil || calls != 1 {
		t.Fatal("unversioned journey contacted server", err, calls)
	}
}
