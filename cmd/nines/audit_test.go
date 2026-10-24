package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestAuditCLIExportPaginationAndScopes(t *testing.T) {
	cliEnv(t)
	t.Setenv("API_TOKEN", "audit-test-key")
	t.Setenv("NINES_TENANT", "alpha")
	var seen []string
	broken := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer audit-test-key" {
			t.Error("missing scope")
		}
		seen = append(seen, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/audit" {
			if broken && r.URL.Query().Get("cursor") != "" {
				w.WriteHeader(503)
				return
			}
			page := v1.AuditPage{APIVersion: v1.APIVersion, Tenant: "alpha", Items: []v1.AuditRecord{{ID: "first", Tenant: "alpha"}}}
			if r.URL.Query().Get("cursor") == "" {
				page.NextCursor = "next"
			} else {
				page.Items[0].ID = "second"
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/audit/")
		_ = json.NewEncoder(w).Encode(v1.AuditRecord{APIVersion: v1.APIVersion, Tenant: "alpha", ID: id, Conversation: "chat", Event: v1.IntentAuditEvent{Action: "change_set.proposed"}})
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	var out bytes.Buffer
	if err := run([]string{"audit", "export", "--actor", "reviewer", "--agent"}, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatal(out.String())
	}
	for i, line := range lines {
		var record v1.AuditRecord
		if json.Unmarshal([]byte(line), &record) != nil || record.ID != []string{"first", "second"}[i] {
			t.Fatal(line)
		}
	}
	if len(seen) != 4 || !strings.Contains(seen[2], "actor=reviewer&cursor=next") {
		t.Fatal(seen)
	}
	broken = true
	if err := run([]string{"audit", "export", "--agent"}, io.Discard); err == nil {
		t.Fatal("partial export reported success")
	}
	if err := run([]string{"audit", "list", "--offline"}, io.Discard); err == nil {
		t.Fatal("offline audit accessed server")
	}
	if err := run([]string{"intent", "conversations", "audit", "chat", "--after", "256", "--through", "300", "--help"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestAuditStatusCLIUsesScopedRead(t *testing.T) {
	cliEnv(t)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("API_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/audit/archive" || r.URL.Query().Get("record") != "one" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("unscoped archive read")
		}
		_, _ = io.WriteString(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","state":"disabled","recordId":"one","enabled":false}`)
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	var out bytes.Buffer
	if err := run([]string{"audit", "status", "--record", "one", "--agent"}, &out); err != nil || !strings.Contains(out.String(), `"state": "disabled"`) {
		t.Fatal(out.String(), err)
	}
}
