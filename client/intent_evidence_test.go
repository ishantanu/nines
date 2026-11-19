package client_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentEvidenceClientViewerTenantAndStrictRequests(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	in, err := testtelemetry.EvidenceInput(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.PrepareIntentEvidence(context.Background(), in)
	if err != nil || out.Tenant != "alpha" || !out.HumanConfirmationRequired || len(out.Comparisons) != 1 || out.Comparisons[0].Current == nil || out.Comparisons[0].HistoryState != "not_requested" {
		t.Fatal(out, err)
	}
	good, _ := json.Marshal(in)
	for _, tc := range []struct {
		body, tenant, token, query string
		code                       int
	}{
		{string(good), "alpha", "viewer", "", 200},
		{string(good), "beta", "viewer", "", 403},
		{string(good), "alpha", "bad", "", 401},
		{string(good), "alpha", "viewer", "?source=other", 400},
		{`{"sourceDigest":"x","sourceDigest":"y"}`, "alpha", "viewer", "", 400},
		{`{"plan":{"intent":{},"intent":{}}}`, "alpha", "viewer", "", 400},
		{`{"tenant":"beta"}`, "alpha", "viewer", "", 400},
		{`null`, "alpha", "viewer", "", 400},
		{string(good) + string(good), "alpha", "viewer", "", 400},
	} {
		r := httptest.NewRequest("POST", "/api/intents/evidence"+tc.query, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Nines-Tenant", tc.tenant)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("got %d expected %d: %s", w.Code, tc.code, w.Body.String())
		}
		if tc.code == 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable evidence")
		}
	}
}
