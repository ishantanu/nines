package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntentConversationCLIUsesScopedYAMLRequests(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer agent" {
			t.Error("unscoped conversation operation")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","id":"chat","version":2}`)
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("API_TOKEN", "agent")
	file := filepath.Join(t.TempDir(), "run.yaml")
	if err := os.WriteFile(file, []byte("id: prepare\nexpectedVersion: 1\nmode: guided\nmodelConsent: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if e := run([]string{"intent", "conversations", "run", "chat", "-f", file, "-o", "yaml"}, &out); e != nil || !strings.Contains(out.String(), "apiVersion: nines.dev/v1alpha1") {
		t.Fatal(out.String(), e)
	}
	if err := os.WriteFile(file, []byte("id: prepare\nexpectedVersion: 1\nmode: guided\nmodelConsent: false\ntenant: beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if e := run([]string{"intent", "conversations", "run", "chat", "-f", file}, &out); e == nil || calls != 1 {
		t.Fatal("unknown scope field reached API", e)
	}
}
