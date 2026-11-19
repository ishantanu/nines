package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestMigrationWarnsWhenTheChosenConnectionDoesNotSpeakPromQL(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	ws := Workspace{ID: "alpha", Name: "Alpha", Service: svc, Source: tenancy.Source{Kind: "datadog"},
		Connections: []tenancy.Connection{{Name: "prom", Source: tenancy.Source{Kind: "prometheus"}}, {Name: "obs", Source: tenancy.Source{Kind: "dash0"}}, {Name: "nr", Source: tenancy.Source{Kind: "newrelic"}}},
		Grants:      []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}
	h, err := NewMulti([]Workspace{ws}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	doc := "version: prometheus/v1\nservice: svc\nslos:\n  - name: a\n    objective: 99.9\n    sli:\n      events:\n        error_query: sum(rate(e[{{.window}}]))\n        total_query: sum(rate(t[{{.window}}]))\n"
	notes := func(source string) []v1.MigrationNote {
		body, _ := json.Marshal(v1.MigrationRequest{Tool: "sloth", Content: doc, Source: source})
		r := httptest.NewRequest("POST", "/api/migrations/convert", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer viewer")
		r.Header.Set(tenancy.Header, "alpha")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var out v1.MigrationResult
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Definitions) != 1 {
			t.Fatalf("%+v %v", out, err)
		}
		var warned []v1.MigrationNote
		for _, n := range out.Notes {
			if n.Field == "source" {
				warned = append(warned, n)
			}
		}
		return warned
	}
	// PromQL-speaking connections are not warned about.
	for _, name := range []string{"prom", "obs", "unknown-managed-connection"} {
		if n := notes(name); len(n) != 0 {
			t.Errorf("%s: %+v", name, n)
		}
	}
	// A Datadog primary or a New Relic connection cannot run PromQL: say so up front.
	for _, name := range []string{"", "primary", "nr"} {
		n := notes(name)
		if len(n) != 1 || n[0].Level != "lossy" || !strings.Contains(n[0].Message, "not PromQL") {
			t.Errorf("%q: %+v", name, n)
		}
	}
}
