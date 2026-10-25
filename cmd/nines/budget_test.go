package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestBudgetCIGateFailsClosedAndUsesTenantScope(t *testing.T) {
	for _, state := range []string{"proceed", "blocked", "review_required", "insufficient_evidence", "not_configured", "unexpected"} {
		t.Run(state, func(t *testing.T) {
			cliEnv(t)
			t.Setenv("NINES_API_KEY", "fixture-key")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/slos/api/budget" || r.URL.Query().Get("expectedRevision") != "3" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("scope or method changed", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(v1.ErrorBudgetDecision{APIVersion: v1.APIVersion, Tenant: "alpha", SLOID: "api", Revision: 3, DefinitionDigest: "definition", PolicyDigest: "policy", SourceDigest: "source", State: state, QualityState: "usable", Evidence: &v1.BudgetEvidence{Coverage: 1, Total: 1000, Freshness: v1.Freshness{State: "fresh"}}, Advisory: true})
			}))
			defer server.Close()
			var out bytes.Buffer
			err := run([]string{"budget", "api", "--server", server.URL, "--tenant", "alpha", "--expected-revision", "3", "--check"}, &out)
			if state == "proceed" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || exitCode(err) != 7 {
				t.Fatal("CI gate allowed non-proceed state", state, err)
			}
			if !json.Valid(out.Bytes()) {
				t.Fatal("machine decision output missing", out.String())
			}
		})
	}
}
func TestQualityCommandRequiresExplicitCohortRead(t *testing.T) {
	cliEnv(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/slos/api/quality" || (calls == 1 && r.URL.Query().Get("cohorts") != "") || (calls == 2 && r.URL.Query().Get("cohorts") != "true") {
			t.Error(r.URL)
		}
		_, _ = w.Write([]byte(`{"state":"limited","tenant":"alpha","sloId":"api","revision":1}`))
	}))
	defer server.Close()
	for _, extra := range [][]string{nil, {"--cohorts"}} {
		var out bytes.Buffer
		args := append([]string{"quality", "api", "--server", server.URL, "--tenant", "alpha"}, extra...)
		if err := run(args, &out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBudgetCIGateRejectsIncompleteOrMismatchedProceedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*v1.ErrorBudgetDecision)
	}{
		{"wrong revision", func(d *v1.ErrorBudgetDecision) { d.Revision = 4 }},
		{"wrong tenant", func(d *v1.ErrorBudgetDecision) { d.Tenant = "beta" }},
		{"missing evidence", func(d *v1.ErrorBudgetDecision) { d.Evidence = nil }},
		{"partial coverage", func(d *v1.ErrorBudgetDecision) { d.Evidence.Coverage = .5 }},
		{"stale evidence", func(d *v1.ErrorBudgetDecision) { d.Evidence.Freshness.State = "stale" }},
		{"missing policy", func(d *v1.ErrorBudgetDecision) { d.PolicyDigest = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cliEnv(t)
			d := v1.ErrorBudgetDecision{APIVersion: v1.APIVersion, Tenant: "alpha", SLOID: "api", Revision: 3, DefinitionDigest: "definition", PolicyDigest: "policy", SourceDigest: "source", State: "proceed", QualityState: "usable", Evidence: &v1.BudgetEvidence{Coverage: 1, Total: 1000, Freshness: v1.Freshness{State: "fresh"}}, Advisory: true}
			tc.change(&d)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(d)
			}))
			defer server.Close()
			var out bytes.Buffer
			err := run([]string{"budget", "api", "--server", server.URL, "--tenant", "alpha", "--expected-revision", "3", "--check"}, &out)
			if err == nil || exitCode(err) != 7 {
				t.Fatal("invalid proceed evidence passed the gate", err)
			}
		})
	}
}
