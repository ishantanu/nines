package client_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentConversationClientAttributionAndCAS(t *testing.T) {
	s, _, e := testtelemetry.New()
	if e != nil {
		t.Fatal(e)
	}
	s.TenantID = "alpha"
	flow := intentflow.New(s, intentflow.NewMemory())
	h, e := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Service: s, IntentConversations: flow, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	c, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner"})
	ctx := context.Background()
	in := v1.CreateIntentConversation{ID: "sdk-chat", Title: "SDK intent", Text: "99.9% over 1 hour", Scope: v1.IntentAuthoringScope{Metadata: v1.Metadata{Name: "new-slo", Project: "test"}, Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, NoDataPolicy: "unknown"}}
	v, e := c.CreateIntentConversation(ctx, in)
	if e != nil || v.Tenant != "alpha" {
		t.Fatal(v, e)
	}
	v, e = c.AppendIntentConversation(ctx, v.ID, v1.AppendIntentConversation{ID: "answer", ExpectedVersion: v.Version, Text: "Count eligible attempts including retries"})
	if e != nil || len(v.Messages) != 2 || v.Messages[1].Actor.Kind != "api_token" {
		t.Fatal(v, e)
	}
	if _, e = c.AppendIntentConversation(ctx, v.ID, v1.AppendIntentConversation{ID: "stale", ExpectedVersion: 1, Text: "stale change"}); e == nil {
		t.Fatal("stale append accepted")
	}
	list, e := c.IntentConversations(ctx, "")
	if e != nil || len(list.Items) != 1 {
		t.Fatal(list, e)
	}
	read, e := c.IntentConversation(ctx, v.ID)
	if e != nil || read.Version != v.Version {
		t.Fatal(read, e)
	}
	if _, e = c.IntentConversation(ctx, "../beta"); e == nil {
		t.Fatal("invalid ID accepted")
	}
	viewer, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if _, e = viewer.StartIntentFlowRun(ctx, v.ID, v1.CreateIntentFlowRun{ID: "forbidden", ExpectedVersion: v.Version, Mode: "guided"}); e == nil {
		t.Fatal("viewer started provider work")
	}
}
