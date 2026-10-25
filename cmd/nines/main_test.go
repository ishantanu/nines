package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
)

func TestCLIValidationPlanAndRevisionGuard(t *testing.T) {
	for _, tc := range []struct{ kind, body string }{{model.JourneyKind, examples.Checkout}, {model.SLOKind, examples.APIAvailability}} {
		t.Run(tc.kind, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "journey.yaml")
			if err := os.WriteFile(file, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := run([]string{"validate", "-f", file}, &out); err != nil {
				t.Fatal(err)
			}
			if err := run([]string{"apply", "-f", file}, &out); err == nil {
				t.Fatal("apply without expected revision accepted")
			}
			t.Setenv("API_TOKEN", "cli-token")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer cli-token" {
					t.Error("missing token")
				}

				if r.Header.Get("Content-Type") != "application/yaml" {
					t.Error("missing YAML media type")
				}
				body, _ := io.ReadAll(r.Body)
				if !bytes.Contains(body, []byte("kind: "+tc.kind)) {
					t.Error("manifest was not sent as YAML")
				}
				if r.URL.Path == "/api/resources/plan" {
					_, _ = w.Write([]byte(`{"action":"create","expectedRevision":0}`))
					return
				}
				if r.Header.Get("If-Match") != `"0"` {
					t.Error("missing create precondition")
				}
				w.WriteHeader(409)
				_, _ = w.Write([]byte(`{"error":"stale revision"}`))
			}))
			defer server.Close()
			if err := run([]string{"plan", "-f", file, "-server", server.URL}, &out); err != nil {
				t.Fatal(err)
			}
			if err := run([]string{"apply", "-f", file, "-server", server.URL, "-expected-revision", "0"}, &out); err == nil {
				t.Fatal("API failure ignored")
			}

		})
	}
}

func TestCLIInitDurationsRuleGenerationAndReadCommands(t *testing.T) {
	var definition bytes.Buffer
	if err := run([]string{"init", "slo", "-name", "checkout", "-window", "28d", "-step", "1m", "-source", "production", "-short-window", "5m", "-long-window", "1h", "-for", "2m", "-good-query", "sum(increase(good[1m]))", "-total-query", "sum(increase(total[1m]))"}, &definition); err != nil {
		t.Fatal(err)
	}
	d, err := model.ParseSLODefinition(definition.Bytes())
	if err != nil || d.Spec.WindowSeconds != 28*86400 || d.Spec.Policy.ForSeconds != 120 || d.Spec.Source != "production" {
		t.Fatal(d, err)
	}
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(file, definition.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var rules bytes.Buffer
	if err := run([]string{"rules", "generate", "-f", file, "-tenant", "commerce"}, &rules); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rules.Bytes(), []byte("nines_source: production")) || !bytes.Contains(rules.Bytes(), []byte("for: 120s")) {
		t.Fatal(rules.String())
	}
	var export bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nines-Tenant") != "commerce" {
			t.Error("tenant missing")
		}
		switch r.URL.Path {
		case "/api/slos":
			_, _ = w.Write([]byte(`[{"slo":{"id":"checkout","target":0.999},"status":{"reliability":null,"coverage":0,"burnRate":null}}]`))
		case "/api/slos/checkout/manifest":
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write(definition.Bytes())
		case "/api/slos/checkout/history":
			if r.URL.Query().Get("rangeSeconds") != "21600" || r.URL.Query().Get("resolutionSeconds") != "300" || r.URL.Query().Get("revision") != "1" {
				t.Error("history duration conversion failed")
			}
			_, _ = w.Write([]byte(`{"target":0.999,"points":[{"at":"2026-10-19T00:00:00Z","coverage":0,"intervalReliability":null,"reliability":null}]}`))
		default:
			t.Error("unexpected path", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	if err := run([]string{"get", "slos", "-tenant", "commerce", "-server", server.URL}, &export); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(export.Bytes(), []byte("OBSERVED")) || !bytes.Contains(export.Bytes(), []byte("unknown")) {
		t.Fatal(export.String())
	}
	export.Reset()
	if err := run([]string{"export", "slo", "checkout", "-tenant", "commerce", "-server", server.URL}, &export); err != nil || export.String() != definition.String() {
		t.Fatal(err, export.String())
	}
	export.Reset()
	if err := run([]string{"history", "checkout", "-range", "6h", "-resolution", "5m", "--revision", "1", "-o", "csv", "-tenant", "commerce", "-server", server.URL}, &export); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(export.Bytes(), []byte("interval_sli,rolling_sli")) {
		t.Fatal(export.String())
	}
	if err := run([]string{"rules", "apply", "-id", "checkout", "-server", server.URL}, &export); err == nil {
		t.Fatal("rules applied without reviewed digest")
	}
}

func TestDescribeSLOUsesOneMatchingRead(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/slos/checkout" || r.Header.Get("X-Nines-Tenant") != "commerce" {
			t.Error("describe used a split or unscoped read", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"slo":{"id":"checkout","project":"p","service":"api","name":"Checkout","target":0.99,"windowSeconds":3600,"stepSeconds":60,"revision":2},"status":{"sloId":"checkout","reliability":0.995,"coverage":1}}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := run([]string{"describe", "slo", "checkout", "-server", server.URL, "-tenant", "commerce"}, &out); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !bytes.Contains(out.Bytes(), []byte(`"revision": 2`)) || !bytes.Contains(out.Bytes(), []byte(`"ServiceLevelObjective"`)) || !bytes.Contains(out.Bytes(), []byte(`"reliability": 0.995`)) {
		t.Fatal(requests, out.String())
	}
}

func TestOfflineLifecycleAndTrafficGuardCLI(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"init", "slo", "--name", "api", "--lifecycle", "experimental", "--min-events", "100"}, &out); err != nil {
		t.Fatal(err)
	}
	d, err := model.ParseSLODefinition(out.Bytes())
	if err != nil || d.Spec.Lifecycle != "experimental" || d.Spec.Policy == nil || d.Spec.Policy.MinEvents != 100 {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err = run([]string{"init", "slo", "--name", "api", "--target", "0.9", "--burn-rate", "14.4"}, &out); err == nil {
		t.Fatal("unreachable threshold accepted offline")
	}
	out.Reset()
	if err = run([]string{"init", "slo", "--name", "api", "--lifecycle", "active"}, &out); err == nil {
		t.Fatal("unknown lifecycle accepted offline")
	}
}
