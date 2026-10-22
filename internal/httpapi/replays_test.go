package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/replay"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func replayHandler(t *testing.T) http.Handler {
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s := service.New(store.NewMemory(), nil)
		s.TenantID = id
		if err := s.Store.Create(context.Background(), model.SLO{ID: "checkout", Name: "Checkout", Project: "commerce", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, Replays: replay.New(s, replay.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "editor", Role: "editor"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}})
	}
	h, err := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestReplayAPITenantRolesAndSeparatePayload(t *testing.T) {
	h := replayHandler(t)
	now := time.Now().Add(-time.Minute).UnixMilli()
	body := fmt.Sprintf(`{"id":"one-session","title":"Checkout","consent":true,"policy":"masked-v1","references":[{"kind":"ServiceLevelObjective","id":"checkout","revision":1}],"events":[{"type":4,"timestamp":%d,"data":{"href":"/?secret=token","width":800,"height":600}},{"type":2,"timestamp":%d,"data":{"node":{"type":0,"id":1,"childNodes":[]},"initialOffset":{"left":0,"top":0}}}]}`, now, now+1)
	if w := invRequest(h, "POST", "/api/session-replays", body, "viewer", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, role := range []string{"owner", "editor", "agent"} {
		b := strings.Replace(body, "one-session", role+"-session", 1)
		if w := invRequest(h, "POST", "/api/session-replays", b, role, "alpha"); w.Code != 201 {
			t.Fatal(role, w.Code, w.Body.String())
		}
	}
	w := invRequest(h, "GET", "/api/session-replays", "", "viewer", "alpha")
	var list v1.SessionReplayList
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Items) != 3 || strings.Contains(w.Body.String(), "childNodes") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/session-replays/owner-session", "/api/session-replays/owner-session/events"} {
		if w := invRequest(h, "GET", path, "", "viewer", "beta"); w.Code != 404 {
			t.Fatal("other tenant read recording", w.Code)
		}
	}
	w = invRequest(h, "GET", "/api/session-replays/owner-session/events", "", "viewer", "alpha")
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "POST", "/api/session-replays/owner-session/archive", `{"expectedVersion":1,"archived":true}`, "editor", "alpha"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = invRequest(h, "POST", "/api/session-replays/owner-session/archive", `{"expectedVersion":1,"archived":true}`, "owner", "alpha"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = invRequest(h, "GET", "/api/session-replays/owner-session/events", "", "viewer", "alpha"); w.Code != 404 {
		t.Fatal("archived events visible", w.Code)
	}
	for _, tc := range []struct{ path, body string }{{"/api/session-replays", strings.TrimSuffix(body, "}") + `,"tenant":"beta"}`}, {"/api/session-replays?tenant=beta", body}} {
		if w := invRequest(h, "POST", tc.path, tc.body, "owner", "alpha"); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestReplayPlayerUsesIsolatedPolicy(t *testing.T) {
	w := httptest.NewRecorder()
	frontend().ServeHTTP(w, httptest.NewRequest("GET", "/replay-player.html", nil))
	csp := w.Header().Get("Content-Security-Policy")
	for _, value := range []string{"connect-src 'none'", "form-action 'none'", "img-src data:", "frame-ancestors 'self'"} {
		if !strings.Contains(csp, value) {
			t.Fatal("missing player boundary", value, csp)
		}
	}
	if w.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatal("player not embeddable by same-origin app")
	}
}
