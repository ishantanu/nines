package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIIntegrationIsReadOnlyAndOfflineRefusesIt(t *testing.T) {
	t.Setenv("NINES_API_KEY", "viewer")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/slos/checkout/integration" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer viewer" {
			t.Error("incorrect scoped integration request", r)
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","sloId":"checkout","deployment":{"state":"not_deployed"}}`)
	}))
	defer server.Close()
	var out bytes.Buffer
	args := []string{"integration", "checkout", "--server", server.URL, "--tenant", "alpha", "--agent"}
	if err := run(args, &out); err != nil || !strings.Contains(out.String(), "not_deployed") {
		t.Fatal(err, out.String())
	}
	if err := run(append(args, "--offline"), &out); err == nil || calls != 1 {
		t.Fatal("offline performed a provider request", err, calls)
	}
}
