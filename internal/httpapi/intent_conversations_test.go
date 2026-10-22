package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentConversationAPIRolesScopeAndStrictRequests(t *testing.T) {
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s, _, e := testtelemetry.New()
		if e != nil {
			t.Fatal(e)
		}
		s.TenantID = id
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, IntentConversations: intentflow.New(s, intentflow.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}})
	}
	h, e := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	body := `{"id":"intent-chat","title":"Checkout","text":"99.9% over 1 hour","scope":{"metadata":{"name":"new-slo","project":"test"},"service":"api","source":"primary","population":{"service":"api"},"noDataPolicy":"unknown"}}`
	if w := invRequest(h, "POST", "/api/intent-conversations", body, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := invRequest(h, "POST", "/api/intent-conversations", body, "agent", "alpha")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var c v1.IntentConversation
	if json.Unmarshal(w.Body.Bytes(), &c) != nil || c.Tenant != "alpha" {
		t.Fatal(w.Body.String())
	}
	if w = invRequest(h, "GET", "/api/intent-conversations/intent-chat", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant chat exposed", w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/selection", `{"expectedVersion":1,"confirm":true,"intentDigest":"forged","binding":{"name":"requests","expectedRevision":1}}`, "agent", "alpha"); w.Code != 403 {
		t.Fatal("agent selected binding", w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations", strings.TrimSuffix(body, "}")+`,"tenant":"beta"}`, "owner", "alpha"); w.Code != 400 {
		t.Fatal("tenant injection accepted", w.Code)
	}
	if w = invRequest(h, "GET", "/api/intent-conversations?cursor=x&tenant=beta", "", "viewer", "alpha"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "GET", "/api/intent-conversations/intent-chat", "", "viewer", "alpha"); w.Code != 200 || strings.Contains(w.Body.String(), "requestDigest") || strings.Contains(w.Body.String(), "native") {
		t.Fatal("private state leaked", w.Code, w.Body.String())
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/runs", `{"id":"one","expectedVersion":1,"mode":"guided"}`, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/runs", `{"id":"one","expectedVersion":1,"mode":"guided"}`, "agent", "alpha"); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "POST", "/api/intent-conversations/intent-chat/messages", `{"id":"late","expectedVersion":2,"text":"change source"}`, "owner", "alpha"); w.Code != 409 {
		t.Fatal("edited active conversation", w.Code, w.Body.String())
	}
	for _, suffix := range []string{"/change-set/approve", "/change-set/apply"} {
		if w := invRequest(h, "POST", "/api/intent-conversations/intent-chat"+suffix, `{"expectedVersion":2,"digest":"forged","confirm":true}`, "agent", "alpha"); w.Code != 403 {
			t.Fatal("agent reached approval/apply", suffix, w.Code)
		}
		if w := invRequest(h, "POST", "/api/intent-conversations/intent-chat"+suffix, `{}`, "viewer", "alpha"); w.Code != 403 {
			t.Fatal("viewer reached approval/apply", suffix, w.Code)
		}
	}
	if w := invRequest(h, "GET", "/api/intent-conversations/intent-chat/change-set/verification", "", "viewer", "beta"); w.Code != 404 {
		t.Fatal("cross-tenant verification", w.Code)
	}
	if _, e = workspaces[0].Service.Store.Read(context.Background(), "new-slo"); e == nil {
		t.Fatal("conversation applied definition")
	}
}
