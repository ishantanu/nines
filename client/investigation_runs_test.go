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

func TestReviewRunSDKUsesScopedRequests(t *testing.T) {
	calls := 0
	wrongTenant := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing tenant/authentication")
		}
		tenant := "alpha"
		if wrongTenant {
			tenant = "beta"
		}
		if r.Method == "POST" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["tenant"] != nil || body["steps"] != nil || body["actor"] != nil {
				t.Error("client accepted execution override")
			}
		}
		if r.Method == "POST" && !strings.HasSuffix(r.URL.Path, "/cancel") {
			w.WriteHeader(202)
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/runs") {
			_ = json.NewEncoder(w).Encode(v1.InvestigationRunList{APIVersion: v1.APIVersion, Tenant: tenant, Items: []v1.InvestigationRun{}})
		} else {
			_ = json.NewEncoder(w).Encode(v1.InvestigationRun{APIVersion: v1.APIVersion, Tenant: tenant, ID: "run-1", InvestigationID: "review-1"})
		}
	}))
	defer server.Close()
	c, err := New(Config{URL: server.URL, Tenant: "alpha", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = c.CreateInvestigationRun(ctx, "review-1", v1.CreateInvestigationRun{ID: "run-1", ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.InvestigationRuns(ctx, "review-1"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.InvestigationRun(ctx, "review-1", "run-1"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.CancelInvestigationRun(ctx, "review-1", "run-1", v1.CancelInvestigationRun{ExpectedVersion: 2}); err != nil {
		t.Fatal(err)
	}
	before := calls
	if _, err = c.InvestigationRun(ctx, "../beta", "run-1"); err == nil || calls != before {
		t.Fatal("invalid scope sent")
	}
	wrongTenant = true
	if _, err = c.InvestigationRun(ctx, "review-1", "run-1"); err == nil {
		t.Fatal("wrong-tenant response accepted")
	}
}
