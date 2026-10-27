package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func TestRetirementClientDoesNotApproveMissingReviewOrWrongTenant(t *testing.T) {
	tenant := "alpha"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("missing tenant")
		}
		_ = json.NewEncoder(w).Encode(v1.RetirementPlan{APIVersion: v1.APIVersion, Tenant: tenant, SLO: "api", Action: "archive", ExpectedRevision: 3, Digest: "reviewed"})
	}))
	defer server.Close()
	api, e := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = api.PlanRetirement(context.Background(), "api", "archive"); e != nil {
		t.Fatal(e)
	}
	if _, e = api.ApplyRetirement(context.Background(), "api", v1.RetirementInput{Action: "archive", ExpectedRevision: 3}); e == nil || calls != 1 {
		t.Fatal("missing digest sent", e, calls)
	}
	if _, e = api.ApplyRetirement(context.Background(), "api", v1.RetirementInput{Action: "delete", ExpectedRevision: 3, Digest: "reviewed"}); e == nil || calls != 1 {
		t.Fatal("missing confirmation sent", e, calls)
	}
	tenant = "beta"
	if _, e = api.PlanRetirement(context.Background(), "api", "archive"); e == nil {
		t.Fatal("wrong tenant accepted")
	}
}
