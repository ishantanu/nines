package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
)

func demoAuthConfig(public, username string, enabled bool, password string) (Config, error) {
	return ParseConfig([]byte(fmt.Sprintf(`publicURL: %s
providers:
- id: local
  name: Local
  type: local
  allowDemoCredentials: %t
  bootstrapUsername: %s
  bootstrapPasswordEnv: PASSWORD
bootstrap:
- tenant: alpha
  provider: local
  subject: %s
  role: owner
`, public, enabled, username, username)), func(string) string { return password })
}

func TestDemoCredentialsRequireExplicitLoopbackAdmin(t *testing.T) {
	for _, tc := range []struct {
		name, public, user, password string
		enabled, valid               bool
	}{
		{"playground", "http://127.0.0.1", "admin", "admin", true, true},
		{"localhost", "http://localhost", "admin", "admin", true, true},
		{"ipv6", "http://[::1]", "admin", "admin", true, true},
		{"normal policy", "http://127.0.0.1", "admin", "admin", false, false},
		{"remote rejected", "https://nines.example.com", "admin", "admin", true, false},
		{"other bootstrap user", "http://127.0.0.1", "viewer", "admin", true, false},
		{"other short password", "http://127.0.0.1", "admin", "short", true, false},
		{"strong override", "http://127.0.0.1", "admin", "fixture-custom-password", true, true},
		{"persistent strong", "https://nines.example.com", "admin", "fixture-custom-password", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := demoAuthConfig(tc.public, tc.user, tc.enabled, tc.password)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}

func TestDemoAdminLoginAndPasswordChangePreserveOrdinaryPolicy(t *testing.T) {
	ctx := context.Background()
	cfg, err := demoAuthConfig("http://127.0.0.1", "admin", true, "admin")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMemory()
	s, err := New(ctx, cfg, repo, map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar}
	info := request(t, browser, "GET", server.URL+"/auth/session", "", "", 200)
	var csrf string
	if err := json.Unmarshal(info["csrfToken"], &csrf); err != nil {
		t.Fatal(err)
	}
	request(t, browser, "POST", server.URL+"/auth/local/local/login", csrf, `{"username":"admin","password":"admin"}`, 200)
	owner := Actor{ID: "test-owner", Kind: "api_token", Grants: map[string]string{"alpha": "owner"}}
	input := LocalUserInput{Provider: "local", Username: "viewer", Name: "Viewer", Password: "admin", Role: "viewer"}
	if _, err := s.CreateLocalUser(ctx, owner, "alpha", input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ordinary short password accepted: %v", err)
	}
	input.Password = "fixture-viewer-password"
	viewer, err := s.CreateLocalUser(ctx, owner, "alpha", input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateLocalUser(ctx, owner, "alpha", viewer.ID, LocalUserInput{Name: "Viewer", Password: "admin", ExpectedVersion: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ordinary password reset bypassed policy: %v", err)
	}
	admin := s.user("local", externalUser{Subject: "admin"})
	changed, err := s.UpdateLocalUser(ctx, owner, "alpha", admin.ID, LocalUserInput{Name: "Admin", Password: "fixture-custom-password", ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := New(ctx, cfg, repo, map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.authenticateLocal(ctx, "local", "admin", "admin"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("restart restored default password: %v", err)
	}
	if _, err := restarted.authenticateLocal(ctx, "local", "admin", "fixture-custom-password"); err != nil {
		t.Fatal(err)
	}
	token, _, err := restarted.login(ctx, "local", externalUser{Subject: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.UpdateLocalUser(ctx, owner, "alpha", admin.ID, LocalUserInput{Name: "Admin", Password: "admin", ExpectedVersion: changed.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.SessionActor(ctx, token); !errors.Is(err, ErrForbidden) {
		t.Fatalf("password reset did not revoke session: %v", err)
	}
	if _, err := restarted.authenticateLocal(ctx, "local", "admin", "admin"); err != nil {
		t.Fatal(err)
	}
}
