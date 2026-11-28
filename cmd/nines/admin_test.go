package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The admin CLI calls the platform API without a tenant header, never sends
// an owner password but prints the owner's setup link, looks up the current
// version when none is given, and needs the ID repeated to delete.
func TestAdminTenantsCLI(t *testing.T) {
	cliEnv(t)
	var mu sync.Mutex
	calls := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "" || r.Header.Get("Authorization") != "Bearer wp_cli_synthetic" {
			http.Error(w, `{"error":"bad request headers"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/platform/overview" {
			_, _ = io.WriteString(w, `{"tenantProvisioning":"platform console","tenants":[{"id":"default","name":"Default","managedBy":"configuration"},{"id":"payments","name":"Payments","managedBy":"console","state":"retired","version":4}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		calls[r.URL.Path] = body
		mu.Unlock()
		if r.URL.Path == "/api/platform/tenants/rename" {
			w.WriteHeader(409)
			_, _ = io.WriteString(w, `{"error":"the tenant changed since you read it"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"payments","name":"Payments","state":"active","version":1,"setupLink":{"url":"http://127.0.0.1/setup#synthetic-token","expiresAt":"2030-01-01T00:00:00Z"}}`)
	}))
	defer server.Close()
	base := []string{"--server", server.URL, "--tenant", "default", "--agent"}
	t.Setenv("NINES_API_KEY", "wp_cli_synthetic")
	var out bytes.Buffer

	if err := run(append([]string{"admin", "tenants", "list", "-o", "table"}, base[:4]...), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "payments  Payments  console") || !strings.Contains(out.String(), "default   Default   configuration") {
		t.Fatal("table", out.String())
	}

	create := append([]string{"admin", "tenants", "create", "payments", "--name", "Payments", "--owner-username", "pat", "--reason", "Onboard payments"}, base...)
	out.Reset()
	if err := run(create, &out); err != nil {
		t.Fatal(err)
	}
	owner := calls["/api/platform/tenants/create"]["newTenant"].(map[string]any)["ownerUser"].(map[string]any)
	if _, sent := owner["password"]; sent || owner["provider"] != "local" || owner["name"] != "pat" {
		t.Fatal("the CLI must never send an owner password", owner)
	}
	if !strings.Contains(out.String(), "/setup#synthetic-token") {
		t.Fatal("setup link not shown", out.String())
	}

	if err := run(append([]string{"admin", "tenants", "rename", "payments", "--name", "Pay", "--reason", "Renamed"}, base...), &out); err == nil || !strings.Contains(err.Error(), "changed since you read it") {
		t.Fatal("server reason lost", err)
	}
	if calls["/api/platform/tenants/rename"]["expectedVersion"] != float64(4) {
		t.Fatal("current version not used", calls["/api/platform/tenants/rename"])
	}
	if err := run(append([]string{"admin", "tenants", "retire", "default", "--reason", "No"}, base...), &out); err == nil || !strings.Contains(err.Error(), "server configuration") {
		t.Fatal("configured tenant not refused", err)
	}

	del := append([]string{"admin", "tenants", "delete", "payments", "--reason", "Merged"}, base...)
	if err := run(del, &out); err == nil || !strings.Contains(err.Error(), "--confirm-id") {
		t.Fatal("delete without confirmation", err)
	}
	if err := run(append(del, "--confirm-id", "payments", "--expected-version", "5"), &out); err != nil {
		t.Fatal(err)
	}
	if got := calls["/api/platform/tenants/delete"]; got["confirm"] != "payments" || got["expectedVersion"] != float64(5) || got["reason"] != "Merged" {
		t.Fatal("delete", got)
	}
}

// Login picks an environment key in auto mode; when the server rejects it,
// the error says which variable to unset instead of a bare HTTP 401.
func TestLoginExplainsARejectedEnvironmentKey(t *testing.T) {
	cliEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"valid tenant API token required"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv("NINES_API_KEY", "synthetic-stale-key")
	var out bytes.Buffer
	err := run([]string{"login", "--context", "playground", "--server", server.URL, "--tenant", "prometheus"}, &out)
	if err == nil || !strings.Contains(err.Error(), "Unset NINES_API_KEY") || exitCode(err) != 3 {
		t.Fatal("unexplained rejection", err)
	}
}
