package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/replay"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestReplayClientPreservesScopeAndSeparatesPayload(t *testing.T) {
	workspaces := []httpapi.Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s := service.New(store.NewMemory(), nil)
		s.TenantID = id
		workspaces = append(workspaces, httpapi.Workspace{ID: id, Name: id, Service: s, Replays: replay.New(s, replay.NewMemory()), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}}})
	}
	h, err := httpapi.NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	c, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner"})
	now := time.Now().Add(-time.Minute).UnixMilli()
	var events []json.RawMessage
	if err := json.Unmarshal([]byte(fmt.Sprintf(`[{"type":4,"timestamp":%d,"data":{"href":"/?token=secret"}},{"type":2,"timestamp":%d,"data":{"node":{"type":0,"id":1,"childNodes":[{"type":3,"id":2,"textContent":"private input"}]}}}]`, now, now+1)), &events); err != nil {
		t.Fatal(err)
	}
	in := v1.CreateSessionReplay{ID: "sdk-capture", Title: "SDK recording", Consent: true, Policy: replay.Policy, Events: events}
	ctx := context.Background()
	created, err := c.CreateSessionReplay(ctx, in)
	if err != nil || created.ID != in.ID || created.Tenant != "alpha" {
		t.Fatal(created, err)
	}
	retry, err := c.CreateSessionReplay(ctx, in)
	if err != nil || retry.Digest != created.Digest || retry.Version != created.Version {
		t.Fatal(retry, err)
	}
	list, err := c.SessionReplays(ctx, "", "")
	if err != nil || len(list.Items) != 1 {
		t.Fatal(list, err)
	}
	payload, err := c.ReplayEvents(ctx, in.ID)
	if err != nil || payload.Digest != created.Digest || len(payload.Events) != 2 {
		t.Fatal(payload, err)
	}
	b, _ := json.Marshal(payload)
	if string(b) == "" || (strings.Contains(string(b), "secret") || strings.Contains(string(b), "private input")) {
		t.Fatal("recording was not masked")
	}
	other, _ := client.New(client.Config{URL: server.URL, Tenant: "beta", Token: "owner"})
	if _, err = other.ReplayEvents(ctx, in.ID); err == nil {
		t.Fatal("cross-tenant recording returned")
	}
	viewer, _ := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "viewer"})
	if _, err = viewer.CreateSessionReplay(ctx, in); err == nil {
		t.Fatal("viewer uploaded recording")
	}
	if _, err = c.SessionReplay(ctx, "../beta"); err == nil {
		t.Fatal("unsafe replay ID accepted")
	}
}
