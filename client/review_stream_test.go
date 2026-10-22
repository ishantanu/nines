package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestSDKReviewStreamValidatesTenantAndUsesHeaders(t *testing.T) {
	wrong := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Last-Event-ID") != "1" {
			t.Error("missing scoped headers")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		tenant := "alpha"
		if wrong {
			tenant = "beta"
		}
		run := v1.InvestigationRun{APIVersion: v1.APIVersion, Tenant: tenant, ID: "run-1", InvestigationID: "review-1", Version: 2, Status: "completed"}
		b, _ := json.Marshal(run)
		_, _ = fmt.Fprintf(w, "id: 2\nevent: run\ndata: %s\n\nevent: done\ndata: {\"reason\":\"terminal\"}\n\n", b)
	}))
	defer server.Close()
	c, _ := New(Config{URL: server.URL, Tenant: "alpha", Token: "token"})
	received := 0
	done, err := c.WatchInvestigationRun(context.Background(), "review-1", "run-1", 1, func(run v1.InvestigationRun) error { received++; return nil })
	if err != nil || !done || received != 1 {
		t.Fatal(done, received, err)
	}
	wrong = true
	if _, err = c.WatchInvestigationRun(context.Background(), "review-1", "run-1", 1, func(v1.InvestigationRun) error { return nil }); err == nil {
		t.Fatal("accepted wrong tenant event")
	}
}
