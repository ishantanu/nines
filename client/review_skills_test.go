package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReviewSkillDiscoveryRejectsOtherTenantAndIdentity(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer viewer" {
			t.Error("unscoped request")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"beta","item":{"metadata":{"name":"evidence-check","version":"1.0.0"}}}`)
	}))
	defer server.Close()
	c, err := New(Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ReviewSkill(context.Background(), "evidence-check", "1.0.0"); err == nil {
		t.Fatal("another tenant accepted")
	}
	if _, err = c.ReviewSkill(context.Background(), "../other", "latest"); err == nil || calls != 1 {
		t.Fatal("unsafe path reached server")
	}
}
