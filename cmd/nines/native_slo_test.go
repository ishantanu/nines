package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNativeCLIReadOnlyDraftAndAgentEnvelope(t *testing.T) {
	t.Setenv("NINES_API_KEY", "read")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Fatal("native mutation or lost tenant")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","kind":"NativeSLODraft","manifest":"apiVersion: nines.dev/v1alpha1\n","warnings":["Review before apply"]}`)
	}))
	defer server.Close()
	for _, agent := range []bool{false, true} {
		var out, errOut bytes.Buffer
		args := []string{"--server", server.URL, "--tenant", "alpha", "native", "draft", "nr", "--native-id", "one", "--name", "imported", "--project", "prod", "--service", "api"}
		if agent {
			args = append(args, "--agent")
		}
		if e := execute(context.Background(), args, &out, &errOut); e != nil {
			t.Fatal(e, out.String(), errOut.String())
		}
		if agent && !bytes.Contains(out.Bytes(), []byte(`"manifest"`)) {
			t.Fatal("agent lost envelope", out.String())
		}
		if !agent && out.String() != "apiVersion: nines.dev/v1alpha1\n" {
			t.Fatal("not portable YAML", out.String())
		}
	}
	before := calls
	var out, errOut bytes.Buffer
	if e := execute(context.Background(), []string{"--offline", "native", "list", "nr"}, &out, &errOut); e == nil || calls != before {
		t.Fatal("offline contacted provider")
	}
}

func TestNativeCLICatalogAndResultContracts(t *testing.T) {
	t.Setenv("NINES_API_KEY", "synthetic-read")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer synthetic-read" {
			t.Error("CLI identity differs from API contract")
		}
		switch r.URL.Path {
		case "/api/slos/availability/native-results":
			if r.URL.Query().Get("from") != "2026-01-01T00:00:00Z" || r.URL.Query().Get("to") != "2026-01-02T00:00:00Z" {
				t.Error("CLI range changed")
			}
			_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","kind":"NativeSLOResults","tenant":"alpha","sloId":"availability","revision":2,"attainment":0.95,"budgetRemaining":-4,"budgetBasis":"provider_reported","points":[]}`)
		case "/api/connections/dd/native-slos":
			_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","kind":"NativeSLOCatalog","tenant":"alpha","source":"dd","provider":"datadog","items":[]}`)
		default:
			t.Error("CLI endpoint changed", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		args []string
		kind string
	}{
		{[]string{"native", "list", "dd"}, "NativeSLOCatalog"},
		{[]string{"native", "results", "availability", "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-02T00:00:00Z"}, "NativeSLOResults"},
	} {
		var out, errOut bytes.Buffer
		args := append([]string{"--agent", "--server", server.URL, "--tenant", "alpha"}, tc.args...)
		if e := execute(context.Background(), args, &out, &errOut); e != nil {
			t.Fatal(e)
		}
		var result map[string]any
		if e := json.Unmarshal(out.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		if result["apiVersion"] != "nines.dev/v1alpha1" || result["tenant"] != "alpha" || result["kind"] != tc.kind {
			t.Fatal("CLI changed the versioned envelope", result)
		}
	}
}
