package client_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentCandidatesClientAuthorizationAndStrictScope(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := httpapi.NewMulti([]httpapi.Workspace{{Features: features.All(), ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	in := v1.IntentCandidateInput{Service: "api", Project: "test", Source: "primary", Population: map[string]string{"service": "api"}, Metrics: []string{"http_requests_total"}}
	out, err := c.DiscoverIntentCandidates(context.Background(), in)
	if err != nil || out.Tenant != "alpha" || len(out.Candidates) != 1 || !out.HumanConfirmationRequired || out.AdapterCalls != 1 {
		t.Fatal(out, err)
	}
	before, err := s.Store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	good, _ := json.Marshal(in)
	for _, tc := range []struct {
		body, query, tenant, token string
		code                       int
	}{
		{string(good), "", "alpha", "viewer", 200},
		{string(good), "?tenant=beta", "alpha", "viewer", 400},
		{string(good), "", "beta", "viewer", 403},
		{string(good), "", "alpha", "wrong", 401},
		{`{"service":"api","Service":"other"}`, "", "alpha", "viewer", 400},
		{`{"source":"primary","source":"other"}`, "", "alpha", "viewer", 400},
		{`{"population":{"service":"api","service":"other"}}`, "", "alpha", "viewer", 400},
		{`null`, "", "alpha", "viewer", 400},
		{`{"service":"api","project":"test","source":"primary","population":{"service":null}}`, "", "alpha", "viewer", 400},
		{string(good) + string(good), "", "alpha", "viewer", 400},
		{strings.Repeat(" ", 32769), "", "alpha", "viewer", 400},
	} {
		r := httptest.NewRequest("POST", "/api/intents/candidates"+tc.query, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Nines-Tenant", tc.tenant)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("response %d wanted %d: %s", w.Code, tc.code, w.Body.String())
		}
		if tc.code == 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable discovery")
		}
	}
	after, err := s.Store.List(context.Background())
	if err != nil || len(after) != len(before) {
		t.Fatal("discovery wrote objectives", err)
	}
}
