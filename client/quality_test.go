package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestOperationalEvidenceClientPinsScopeAndRejectsMismatchedResponses(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "cross-tenant"}[bad], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer fixture" || r.URL.Query().Get("expectedRevision") != "2" {
					t.Error("request lost scope", r.URL)
				}
				tenant := "alpha"
				if bad {
					tenant = "beta"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": v1.APIVersion, "tenant": tenant, "sloId": "api", "revision": 2, "state": "usable"})
			}))
			defer server.Close()
			c, err := New(Config{URL: server.URL, Tenant: "alpha", Token: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.MeasurementQuality(context.Background(), "api", 2, true); (err != nil) != bad {
				t.Fatal(err)
			}
			if _, err = c.ErrorBudgetDecision(context.Background(), "api", 2); (err != nil) != bad {
				t.Fatal(err)
			}
		})
	}
}
