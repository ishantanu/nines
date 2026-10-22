package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestInvestigationSDKTenantAndMutationBoundary(t *testing.T) {
	calls := 0
	wrongTenant := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("tenant scope or credentials missing")
		}
		if r.Method == "POST" {
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
				return
			}
			if in["tenant"] != nil || in["actor"] != nil {
				t.Error("client attempted to impersonate an actor")
			}
		}
		if r.Method == "POST" && r.URL.Path == "/api/investigations" {
			w.WriteHeader(201)
		}
		tenant := "alpha"
		if wrongTenant {
			tenant = "beta"
		}
		if strings.Contains(r.URL.RawQuery, "limit=") {
			_ = json.NewEncoder(w).Encode(v1.InvestigationList{APIVersion: v1.APIVersion, Tenant: tenant, Items: []v1.InvestigationSummary{}})
		} else {
			_ = json.NewEncoder(w).Encode(v1.Investigation{APIVersion: v1.APIVersion, Tenant: tenant, InvestigationSummary: v1.InvestigationSummary{ID: "incident", Version: 1}, Entries: []v1.InvestigationEntry{}})
		}
	}))
	defer server.Close()
	c, err := New(Config{URL: server.URL, Tenant: "alpha", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = c.CreateInvestigation(ctx, v1.CreateInvestigation{ID: "incident"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AppendInvestigationMessage(ctx, "incident", v1.AppendInvestigationMessage{ID: "message", ExpectedVersion: 1, Text: "question"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.CaptureInvestigationEvidence(ctx, "incident", v1.CaptureInvestigationEvidence{ID: "evidence", ExpectedVersion: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Investigations(ctx, "", 25); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Investigation(ctx, "incident"); err != nil {
		t.Fatal(err)
	}
	count := calls
	if _, err = c.Investigation(ctx, "../other"); err == nil || calls != count {
		t.Fatal("invalid identifier reached the server")
	}
	wrongTenant = true
	if _, err = c.Investigation(ctx, "incident"); err == nil {
		t.Fatal("accepted another tenant's response")
	}
}
