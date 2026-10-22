package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLocalLoginUserCRUDRevocationAndCrossTenantBoundaries(t *testing.T) {
	ctx := context.Background()
	config, err := ParseConfig([]byte(`publicURL: http://127.0.0.1
providers:
- id: local
  name: Local
  type: local
  bootstrapUsername: admin
  bootstrapPasswordEnv: BOOTSTRAP_PASSWORD
bootstrap:
- tenant: alpha
  provider: local
  subject: admin
  role: owner
- tenant: beta
  provider: local
  subject: admin
  role: owner
`), func(string) string { return "fixture-bootstrap-password" })
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMemory()
	s, err := New(ctx, config, repo, map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost(dummyLocalHash); err != nil || cost != 12 {
		t.Fatal("invalid constant-time dummy hash", cost, err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	info := request(t, browser, "GET", server.URL+"/auth/session", "", "", 200)
	var csrf string
	if err := json.Unmarshal(info["csrfToken"], &csrf); err != nil {
		t.Fatal(err)
	}
	request(t, browser, "POST", server.URL+"/auth/local/local/login", csrf, `{"username":"admin","password":"wrong-password"}`, 401)
	request(t, browser, "POST", server.URL+"/auth/local/local/login", csrf, `{"username":"admin","password":"fixture-bootstrap-password"}`, 200)
	actor := Actor{ID: "bootstrap", Kind: "api_token", Grants: map[string]string{"alpha": "owner"}}
	in := LocalUserInput{Provider: "local", Username: "alex", Name: "Alex", Password: "fixture-viewer-password", Role: "viewer"}
	user, err := s.CreateLocalUser(ctx, actor, "alpha", in)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := s.login(ctx, "local", externalUser{Subject: "alex", Name: "Alex"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.UpdateLocalUser(ctx, actor, "alpha", user.ID, LocalUserInput{Name: "Alex Updated", Password: "fixture-rotated-password", ExpectedVersion: 1})
	if err != nil || out.Version != 2 {
		t.Fatal(out, err)
	}
	if _, err = s.SessionActor(ctx, token); err == nil {
		t.Fatal("password reset did not revoke session")
	}
	if _, err = s.UpdateLocalUser(ctx, actor, "alpha", user.ID, LocalUserInput{Name: "stale", ExpectedVersion: 1}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale profile write accepted", err)
	}
	if _, err = s.Grant(ctx, Actor{Kind: "api_token", Grants: map[string]string{"beta": "owner"}}, "beta", "local", "alex", "viewer", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateLocalUser(ctx, actor, "alpha", user.ID, LocalUserInput{Name: "Takeover", Password: "fixture-attacker-password", ExpectedVersion: 2}); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross-tenant identity changed", err)
	}
	if _, err = s.CreateLocalUser(ctx, actor, "alpha", in); !errors.Is(err, ErrConflict) {
		t.Fatal("existing identity overwritten", err)
	}
	admin := s.user("local", externalUser{Subject: "admin"})
	if err = s.RemoveMember(ctx, actor, "alpha", admin.ID, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("last owner removed", err)
	}
	// A local identity pre-provisioned in another tenant cannot be claimed by
	// assigning an initial password through a different tenant's create flow.
	if _, err = s.Grant(ctx, Actor{Kind: "api_token", Grants: map[string]string{"beta": "owner"}}, "beta", "local", "reserved", "viewer", 0); err != nil {
		t.Fatal(err)
	}
	in.Username = "reserved"
	if _, err = s.CreateLocalUser(ctx, actor, "alpha", in); !errors.Is(err, ErrConflict) {
		t.Fatal("pre-provisioned identity claimed", err)
	}
}
