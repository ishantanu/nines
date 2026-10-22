package client_test

import (
	"context"
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
