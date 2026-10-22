package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReplayCLIUsesNinesScopeAndKeepsEventsExplicit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer viewer" || r.URL.Path != "/api/session-replays" || r.URL.Query().Get("resourceId") != "checkout" {
			t.Error("unexpected replay request")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","items":[],"retentionDays":7,"maxSessions":50}`)
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("API_TOKEN", "viewer")
	var out bytes.Buffer
	if err := run([]string{"replays", "list", "-resource-kind", "ServiceLevelObjective", "-resource-id", "checkout", "-o", "yaml"}, &out); err != nil || !strings.Contains(out.String(), "retentionDays: 7") {
		t.Fatal(out.String(), err)
	}
	if err := run([]string{"replays", "events", "../other"}, &out); err == nil || calls != 1 {
		t.Fatal("unsafe path reached server")
	}
}
