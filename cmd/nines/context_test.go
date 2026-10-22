package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLISearchAndContextCommands(t *testing.T) {
	t.Setenv("API_TOKEN", "viewer")
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer viewer" {
			t.Error("CLI changed scoped identity")
		}
		switch r.URL.Path {
		case "/api/resources/search":
			if r.URL.Query().Get("project") != "" || r.URL.Query().Get("query") != "checkout" || r.URL.Query().Get("limit") != "2" {
				t.Error(r.URL)
			}
			_, _ = fmt.Fprint(w, `{"tenant":"alpha","items":[{"kind":"ServiceLevelObjective","id":"checkout","project":"commerce","displayName":"Checkout","revision":2}],"nextCursor":"continue-token"}`)
		case "/api/slos/checkout/context":
			if r.URL.Query().Get("expectedRevision") != "2" {
				t.Error("missing revision guard")
			}
			_, _ = fmt.Fprint(w, `{"tenant":"alpha","resource":{"id":"checkout","revision":2},"health":"unknown"}`)
		case "/api/journeys/checkout/context":
			if r.URL.Query().Get("limit") != "3" || r.URL.Query().Get("cursor") != "same-page" {
				t.Error(r.URL)
			}
			_, _ = fmt.Fprint(w, `{"tenant":"alpha","healthScope":"returned_links_only","links":[]}`)
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	for _, args := range [][]string{{"search", "-query", "checkout", "-limit", "2", "-o", "table"}, {"context", "slo", "checkout", "-expected-revision", "2"}, {"context", "journey", "checkout", "-limit", "3", "-cursor", "same-page"}} {
		var out bytes.Buffer
		args = append(args, "-server", api.URL, "-tenant", "alpha")
		if err := run(args, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "checkout") && !strings.Contains(out.String(), "returned_links_only") {
			t.Fatal(out.String())
		}
	}
	for _, args := range [][]string{{"context", "slo", "checkout", "-cursor", "x"}, {"search", "-limit", "101"}, {"context", "journey", "checkout", "-at", "tomorrow"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatal("invalid context CLI arguments accepted", args)
		}
	}
}
