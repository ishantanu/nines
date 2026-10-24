package client_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
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
	status, archiveErr := c.AuditArchiveStatus(ctx, "")
	if archiveErr != nil || status.Enabled || status.State != "disabled" || status.Tenant != "alpha" {
		t.Fatal(status, archiveErr)
	}
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
	if _, err := viewer.AuditArchiveStatus(ctx, ""); err == nil {
		t.Fatal("viewer read tenant archive status")
	}
	if _, e = viewer.StartIntentFlowRun(ctx, v.ID, v1.CreateIntentFlowRun{ID: "forbidden", ExpectedVersion: v.Version, Mode: "guided"}); e == nil {
		t.Fatal("viewer started provider work")
	}
}

func TestIntentGovernanceClientDecisionRoutes(t *testing.T) {
	ctx := context.Background()
	operations := []struct {
		name, path string
		run        func(*client.Client) (v1.IntentConversation, error)
	}{
		{"reject", "reject", func(c *client.Client) (v1.IntentConversation, error) {
			return c.RejectIntentChangeSet(ctx, "chat", v1.ReviewIntentChangeSet{ExpectedVersion: 7, Digest: "reviewed", Confirm: true, Reason: "Evidence incomplete"})
		}},
		{"rollback-plan", "rollback", func(c *client.Client) (v1.IntentConversation, error) {
			return c.PrepareIntentRollback(ctx, "chat", v1.ReviewIntentChangeSet{ExpectedVersion: 7, Digest: "reviewed", Confirm: true, Reason: "Evidence incomplete"})
		}},
		{"rollback-approve", "rollback/approve", func(c *client.Client) (v1.IntentConversation, error) {
			return c.ApproveIntentRollback(ctx, "chat", v1.ReviewIntentChangeSet{ExpectedVersion: 7, Digest: "reviewed", Confirm: true, Reason: "Evidence incomplete"})
		}},
		{"rollback-apply", "rollback/apply", func(c *client.Client) (v1.IntentConversation, error) {
			return c.ApplyIntentRollback(ctx, "chat", v1.ReviewIntentChangeSet{ExpectedVersion: 7, Digest: "reviewed", Confirm: true, Reason: "Evidence incomplete"})
		}},
		{"rollback-reject", "rollback/reject", func(c *client.Client) (v1.IntentConversation, error) {
			return c.RejectIntentRollback(ctx, "chat", v1.ReviewIntentChangeSet{ExpectedVersion: 7, Digest: "reviewed", Confirm: true, Reason: "Evidence incomplete"})
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/api/intent-conversations/chat/change-set/"+operation.path || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error("unscoped decision request", r.Method, r.URL.Path)
				}
				var request v1.ReviewIntentChangeSet
				if json.NewDecoder(r.Body).Decode(&request) != nil || request.ExpectedVersion != 7 || request.Digest != "reviewed" || !request.Confirm || request.Reason != "Evidence incomplete" {
					t.Error("review preconditions lost", request)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","id":"chat","version":8}`)
			}))
			defer server.Close()
			c, e := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "fixture"})
			if e != nil {
				t.Fatal(e)
			}
			result, e := operation.run(c)
			if e != nil || result.Version != 8 {
				t.Fatal(result, e)
			}
		})
	}
}
func TestIntentAuditClientRejectsDifferentConversation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/intent-conversations/chat/audit" {
			t.Error(r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","conversation":"other","integrity":"verified","events":[]}`)
	}))
	defer server.Close()
	c, e := client.New(client.Config{URL: server.URL, Tenant: "alpha"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.IntentAudit(context.Background(), "chat"); e == nil {
		t.Fatal("different conversation accepted")
	}
}
