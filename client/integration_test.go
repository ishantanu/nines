package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ishantanu/nines/client"
)

func TestIntegrationClientChecksTenantAndIdentifier(t *testing.T) {
	tenant := "alpha"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/slos/checkout/integration" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error(r)
		}
		_, _ = fmt.Fprintf(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":%q,"definition":{"state":"saved"}}`, tenant)
	}))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := c.Integration(context.Background(), "checkout"); err != nil || out.Definition.State != "saved" {
		t.Fatal(err, out)
	}
	if _, err := c.Integration(context.Background(), "../beta"); err == nil {
		t.Fatal("unsafe identifier accepted")
	}
	tenant = "beta"
	if _, err := c.Integration(context.Background(), "checkout"); err == nil {
		t.Fatal("wrong tenant response accepted")
	}
}

func TestRuleAutomationClientScopeAndExplicitControls(t *testing.T) {
	tenant := "alpha"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/slos/checkout/rules/automation" || r.Method != "POST" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error(r)
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["expectedRevision"] != float64(4) {
			t.Error("missing revision")
		}
		if _, ok := body["paused"].(bool); !ok {
			t.Error("pause/resume not explicit")
		}
		_, _ = fmt.Fprintf(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":%q,"sloId":"checkout","automation":{"enabled":true,"paused":true,"revision":4,"state":"paused"}}`, tenant)
	}))
	defer server.Close()
	c, e := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner"})
	if e != nil {
		t.Fatal(e)
	}
	if out, e := c.SetRuleAutomation(context.Background(), "checkout", 4, true); e != nil || !out.Automation.Paused {
		t.Fatal(out, e)
	}
	tenant = "beta"
	if _, e = c.SetRuleAutomation(context.Background(), "checkout", 4, false); e == nil {
		t.Fatal("cross-tenant response accepted")
	}
}
