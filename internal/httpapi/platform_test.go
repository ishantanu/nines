package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestPlatformConsoleIsSeparateFromTenantAccess(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	public := "http://" + server.Listener.Addr().String()
	config, err := identity.ParseConfig([]byte(fmt.Sprintf(`publicURL: %s
providers:
- id: local
  name: Local
  type: local
  bootstrapUsername: admin
  bootstrapPasswordEnv: PLATFORM_PASSWORD
platformAdmins:
- provider: local
  subject: admin
`, public)), func(string) string { return "fixture-platform-password" })
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(context.Background(), config, identity.NewMemory(), map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Connections: []tenancy.Connection{{Name: "primary"}, {Name: "regional"}}, Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "tenant-owner-key", Role: "owner"}}}, {ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil)}}, slog.New(slog.NewTextHandler(io.Discard, nil)), auth)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = h
	server.Start()
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	req := func(method, path, body, csrf, tenant, bearer string, want int) json.RawMessage {
		t.Helper()
		r, _ := http.NewRequest(method, public+path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if tenant != "" {
			r.Header.Set("X-Nines-Tenant", tenant)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, e := browser.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.StatusCode, want, raw)
		}
		return raw
	}
	var info struct {
		CSRF string `json:"csrfToken"`
	}
	if json.Unmarshal(req("GET", "/auth/session", "", "", "", "", 200), &info) != nil {
		t.Fatal("session")
	}
	if json.Unmarshal(req("POST", "/auth/local/local/login", `{"username":"admin","password":"fixture-platform-password"}`, info.CSRF, "", "", 200), &info) != nil {
		t.Fatal("login")
	}
	var inventory struct {
		Tenants []struct {
			ID          string `json:"id"`
			Connections int    `json:"connections"`
		} `json:"tenants"`
	}
	if err = json.Unmarshal(req("GET", "/api/platform/overview", "", "", "", "", 200), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Tenants) != 2 || inventory.Tenants[0].Connections != 2 || inventory.Tenants[1].Connections != 1 {
		t.Fatal("primary connection double counted", inventory)
	}
	if string(req("GET", "/api/tenants", "", "", "", "", 200)) != "[]\n" {
		t.Fatal("platform role implicitly added tenant grants")
	}
	req("GET", "/api/slos", "", "", "alpha", "", 403)
	req("GET", "/api/platform/users", "", "", "", "tenant-owner-key", 401)
	req("POST", "/api/platform/memberships/save", `{"reason":"Assign an explicit owner","tenant":"alpha","provider":"local","subject":"admin","role":"owner","expectedVersion":0}`, info.CSRF, "", "", 200)
	req("GET", "/api/slos", "", "", "alpha", "", 200)
	req("GET", "/api/slos", "", "", "beta", "", 403)
	for _, path := range []string{"/admin", "/admin/tenants", "/admin/users", "/admin/audit"} {
		raw := req("GET", path, "", "", "", "", 200)
		if !strings.Contains(string(raw), `<html`) {
			t.Fatal("admin deep link missing shell", path)
		}
	}
	req("GET", "/admin/unrecognized", "", "", "", "", 404)
}
