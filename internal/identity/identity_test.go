package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/testauth"
)

func ldapConfig(t *testing.T, l *testauth.LDAP, public string) Config {
	t.Helper()
	body := fmt.Sprintf(`publicURL: %s
providers:
- id: directory
  name: Company directory
  type: ldap
  url: %s
  startTLS: %v
  caFile: %s
  bindDN: cn=reader,dc=example,dc=test
  bindPasswordEnv: READER_PASSWORD
  userBaseDN: ou=people,dc=example,dc=test
  userFilter: (uid={username})
bootstrap:
- tenant: alpha
  provider: directory
  subject: uuid-alice
  role: owner
`, public, l.URL, strings.HasPrefix(l.URL, "ldap://"), l.CAFile)
	c, err := ParseConfig([]byte(body), func(string) string { return "fixture-reader" })
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func request(t *testing.T, c *http.Client, method, path, csrf, body string, want int) map[string]json.RawMessage {
	t.Helper()
	r, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	res, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = res.Body.Close()
	}()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("%s %s = %d want %d: %s", method, path, res.StatusCode, want, b)
	}
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(b, &out)
	return out
}
func textField(t *testing.T, m map[string]json.RawMessage, k string) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(m[k], &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}
func loginDirectory(t *testing.T, s *Service, h *httptest.Server, user, password string, want int) (*http.Client, string, Actor) {
	t.Helper()
	c := newClient()
	info := request(t, c, "GET", h.URL+"/auth/session", "", "", 200)
	csrf := textField(t, info, "csrfToken")
	b, _ := json.Marshal(map[string]string{"username": user, "password": password})
	out := request(t, c, "POST", h.URL+"/auth/ldap/directory/login", csrf, string(b), want)
	if want != 200 {
		return c, "", Actor{}
	}
	var actor Actor
	if err := json.Unmarshal(out["actor"], &actor); err != nil {
		t.Fatal(err)
	}
	return c, textField(t, out, "csrfToken"), actor
}
func TestDirectoryEncryptedLoginFailureAndSessionLifecycle(t *testing.T) {
	for _, startTLS := range []bool{false, true} {
		t.Run(fmt.Sprint(startTLS), func(t *testing.T) {
			l, err := testauth.NewLDAP(t.TempDir(), startTLS)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			h := httptest.NewUnstartedServer(nil)
			c := ldapConfig(t, l, "http://"+h.Listener.Addr().String())
			s, err := New(context.Background(), c, NewMemory(), map[string]bool{"alpha": true, "beta": true})
			if err != nil {
				t.Fatal(err)
			}
			h.Config.Handler = s.Handler()
			h.Start()
			defer h.Close()
			_, _, _ = loginDirectory(t, s, h, "alice", "bad-password", 401)
			client, csrf, a := loginDirectory(t, s, h, "alice", "fixture-alice", 200)
			me := request(t, client, "GET", h.URL+"/auth/session", "", "", 200)
			var verified Actor
			if err := json.Unmarshal(me["actor"], &verified); err != nil {
				t.Error(err)
				return
			}
			if verified.Kind != "user" || verified.Name != "Alice Directory" || verified.IdentitySubject != "uuid-alice" {
				t.Fatal(verified)
			}
			raw := ""
			u, _ := url.Parse(h.URL)
			for _, cookie := range client.Jar.Cookies(u) {
				if cookie.Name == s.cookieName("session") {
					raw = cookie.Value
				}
			}
			actual, err := s.SessionActor(context.Background(), raw)
			if err != nil || actual.Grants["alpha"] != "owner" || actual.Grants["beta"] != "" {
				t.Fatal(actual, err)
			}
			request(t, client, "POST", h.URL+"/auth/logout", "", "", 403)
			r, _ := http.NewRequest("POST", h.URL+"/auth/logout", nil)
			r.Header.Set("X-CSRF-Token", csrf)
			r.Header.Set("Origin", "https://evil.invalid")
			res, err := client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != 403 {
				t.Fatal(res.StatusCode)
			}
			_, _, _ = loginDirectory(t, s, h, "*)(uid=*)", "fixture-alice", 401)
			request(t, client, "POST", h.URL+"/auth/logout-all", csrf, "", 200)
			if _, err = s.SessionActor(context.Background(), raw); err == nil {
				t.Fatal("revoked session accepted")
			}
			_, _, _ = loginDirectory(t, s, h, "alice", "fixture-alice", 200)
			if _, err = s.SessionActor(context.Background(), raw); err == nil {
				t.Fatal("old session revived after login")
			}
			if a.ID == "" || l.PlainBinds.Load() != 0 {
				t.Fatal("anonymous/plaintext bind")
			}

		})
	}
}
func TestLDAPRejectsUntrustedCertificateAndBlankPassword(t *testing.T) {
	l, err := testauth.NewLDAP(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	cfg := ldapConfig(t, l, "http://127.0.0.1:8199").Providers[0]
	cfg.CAFile = ""
	p, err := initProvider(context.Background(), cfg, "http://127.0.0.1:8199")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.authenticateLDAP(context.Background(), "alice", "fixture-alice"); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	if l.Binds.Load() != 0 {
		t.Fatal("credentials sent before certificate verification")
	}
	if _, err = p.authenticateLDAP(context.Background(), "alice", ""); err == nil {
		t.Fatal("empty password accepted")
	}
}
func TestDirectoryIdentityUsesImmutableADGUID(t *testing.T) {
	l, err := testauth.NewLDAP(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	cfg := ldapConfig(t, l, "http://127.0.0.1:8199").Providers[0]
	cfg.SubjectAttribute = "objectGUID"
	p, err := initProvider(context.Background(), cfg, "http://127.0.0.1:8199")
	if err != nil {
		t.Fatal(err)
	}
	u, err := p.authenticateLDAP(context.Background(), "alice", "fixture-alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.Subject != base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
		t.Fatal(u.Subject)
	}
}
func TestOIDCSignedTokensPKCEStateNonceAndReplay(t *testing.T) {
	for _, mode := range []string{"valid", "nonce", "audience", "issuer", "expired", "signature", "state", "missing-cookie"} {
		t.Run(mode, func(t *testing.T) {
			idp, err := testauth.NewOIDC()
			if err != nil {
				t.Fatal(err)
			}
			defer idp.Close()
			h := httptest.NewUnstartedServer(nil)
			public := "http://" + h.Listener.Addr().String()
			if err := idp.AllowRedirect(public + "/auth/oidc/company/callback"); err != nil {
				t.Fatal(err)
			}
			c := Config{PublicURL: public, SessionHours: 12, Providers: []ProviderConfig{{ID: "company", Name: "Company SSO", Type: "oidc", IssuerURL: idp.Server.URL, ClientID: "fixture-client", secret: "fixture-secret"}}, Bootstrap: []Bootstrap{{"alpha", "company", "alice", "owner"}}}
			s, err := New(context.Background(), c, NewMemory(), map[string]bool{"alpha": true})
			if err != nil {
				t.Fatal(err)
			}
			h.Config.Handler = s.Handler()
			h.Start()
			defer h.Close()
			client := newClient()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			res, err := client.Get(h.URL + "/auth/oidc/company/login")
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			authURL, _ := url.Parse(res.Header.Get("Location"))
			q := authURL.Query()
			if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
				t.Fatal(q)
			}
			q.Set("login_as", "alice")
			authURL.RawQuery = q.Encode()
			res, err = client.Get(authURL.String())
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			callback := res.Header.Get("Location")
			switch mode {
			case "nonce":
				idp.BadNonce = true
			case "audience":
				idp.BadAudience = true
			case "issuer":
				idp.BadIssuer = true
			case "expired":
				idp.Expired = true
			case "signature":
				idp.BadSignature = true
			case "state":
				u, _ := url.Parse(callback)
				q := u.Query()
				q.Set("state", strings.Repeat("x", 43))
				u.RawQuery = q.Encode()
				callback = u.String()
			case "missing-cookie":
				client = newClient()
				client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			}
			res, err = client.Get(callback)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if mode == "valid" {
				if res.StatusCode != 303 {
					t.Fatalf("callback %d", res.StatusCode)
				}
				me := request(t, client, "GET", h.URL+"/auth/session", "", "", 200)
				var yes bool
				if err := json.Unmarshal(me["authenticated"], &yes); err != nil {
					t.Error(err)
					return
				}
				if !yes {
					t.Fatal("not authenticated")
				}
				if len(me["actor"]) == 0 {
					t.Fatal(me)
				}
				res, err = client.Get(callback)
				if err != nil {
					t.Fatal(err)
				}
				_ = res.Body.Close()
				if res.StatusCode != 403 {
					t.Fatal("replayed OIDC state accepted")
				}
			} else {
				if res.StatusCode != 403 {
					t.Fatalf("invalid %s token/state accepted: %d", mode, res.StatusCode)
				}
				me := request(t, client, "GET", h.URL+"/auth/session", "", "", 200)
				var yes bool
				if err := json.Unmarshal(me["authenticated"], &yes); err != nil {
					t.Error(err)
					return
				}
				if yes {
					t.Fatal("invalid token issued session")
				}
			}
		})
	}
}
func serviceFixture(t *testing.T, repo Repository) (*Service, Actor) {
	t.Helper()
	cfg := Config{PublicURL: "http://127.0.0.1:8199", SessionHours: 12, Providers: []ProviderConfig{{ID: "directory", Name: "Directory", Type: "ldap", URL: "ldaps://127.0.0.1:1", BindDN: "cn=reader", secret: "fixture-reader", UserBaseDN: "dc=example", UserFilter: "(uid={username})", SubjectAttribute: "entryUUID", NameAttribute: "displayName", EmailAttribute: "mail"}}, Bootstrap: []Bootstrap{{"alpha", "directory", "alice", "owner"}, {"beta", "directory", "alice", "owner"}}}
	s, err := New(context.Background(), cfg, repo, map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	_, owner, err := s.login(context.Background(), "directory", externalUser{Subject: "alice", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	return s, owner
}
func TestTenantTeamMembershipAndServiceAccountRevocation(t *testing.T) {
	s, owner := serviceFixture(t, NewMemory())
	ctx := context.Background()
	m, err := s.Grant(ctx, owner, "alpha", "directory", "bob", "viewer", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Grant(ctx, owner, "beta", "directory", "bob", "viewer", 0); err != nil {
		t.Fatal(err)
	}
	token, bob, err := s.login(ctx, "directory", externalUser{Subject: "bob", Name: "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	if bob.Grants["alpha"] != "viewer" || bob.Grants["beta"] != "viewer" {
		t.Fatal(bob)
	}
	team, err := s.SaveTeam(ctx, owner, "alpha", Team{ID: "operators", Name: "Operators", Role: "editor", Members: []string{m.UserID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	bob, err = s.SessionActor(ctx, token)
	if err != nil || bob.Grants["alpha"] != "editor" || bob.Grants["beta"] != "viewer" {
		t.Fatal(bob, err)
	}
	if _, err = s.SaveTeam(ctx, bob, "alpha", Team{ID: "self-owner", Name: "Self", Role: "owner"}, 0); err == nil {
		t.Fatal("self escalation")
	}
	if _, err = s.Grant(ctx, bob, "alpha", "directory", "bob", "owner", m.Version); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err = s.SaveTeam(ctx, owner, "beta", Team{ID: "foreign", Name: "Foreign", Role: "editor", Members: []string{"u-foreign"}}, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign user accepted", err)
	}
	if _, err = s.SaveTeam(ctx, owner, "alpha", team, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("stale team changed", err)
	}
	if err = s.RemoveTeam(ctx, owner, "alpha", team.ID, team.Version); err != nil {
		t.Fatal(err)
	}
	bob, _ = s.SessionActor(ctx, token)
	if bob.Grants["alpha"] != "viewer" {
		t.Fatal("removed team still grants", bob)
	}
	a, secret, err := s.CreateAccount(ctx, owner, "alpha", "agent", "Agent", "agent", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "wp_sa_") || a.Role != "agent" {
		t.Fatal(a)
	}
	machine, err := s.ServiceActor(ctx, secret)
	if err != nil || machine.Grants["alpha"] != "agent" || machine.Grants["beta"] != "" {
		t.Fatal(machine, err)
	}
	if _, _, err = s.CreateAccount(ctx, owner, "alpha", "owner", "Owner", "owner", time.Now().Add(time.Hour)); !errors.Is(err, ErrInvalid) {
		t.Fatal("machine owner allowed")
	}
	if err = s.RevokeAccount(ctx, owner, "alpha", "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ServiceActor(ctx, secret); err == nil {
		t.Fatal("revoked token accepted")
	}
	dir, err := s.Directory(ctx, owner, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(dir)
	if strings.Contains(string(b), secret) || strings.Contains(string(b), "tokenHash") {
		t.Fatal("secret exposed")
	}
	var current Member
	_ = s.repo.View(ctx, func(tx Tx) error { var e error; current, e = read[Member](tx, "member", "alpha", m.UserID); return e })
	if err = s.RemoveMember(ctx, owner, "alpha", m.UserID, current.Version); err != nil {
		t.Fatal(err)
	}
	bob, _ = s.SessionActor(ctx, token)
	if bob.Grants["alpha"] != "" || bob.Grants["beta"] != "viewer" {
		t.Fatal("membership removal crossed tenant or failed", bob)
	}
}
func TestLastOwnerConcurrentChangesAndBootstrapDoesNotRestoreRemovedAccess(t *testing.T) {
	repo := NewMemory()
	s, a := serviceFixture(t, repo)
	ctx := context.Background()
	b, err := s.Grant(ctx, a, "alpha", "directory", "bob", "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	var alice Member
	if err := s.repo.View(ctx, func(tx Tx) error { alice, _ = read[Member](tx, "member", "alpha", a.ID); return nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, m := range []Member{alice, b} {
		wg.Add(1)
		go func(m Member) { defer wg.Done(); results <- s.RemoveMember(ctx, a, "alpha", m.UserID, m.Version) }(m)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("last owner invariant", success)
	}
	// A separate fixture proves one-time bootstrap never resurrects revocation.
	repo2 := NewMemory()
	s2, a2 := serviceFixture(t, repo2)
	_, err = s2.Grant(ctx, a2, "alpha", "directory", "replacement", "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	var first Member
	if err := s2.repo.View(ctx, func(tx Tx) error { first, _ = read[Member](tx, "member", "alpha", a2.ID); return nil }); err != nil {
		t.Fatal(err)
	}
	if err = s2.RemoveMember(ctx, a2, "alpha", a2.ID, first.Version); err != nil {
		t.Fatal(err)
	}
	_, err = New(ctx, s2.config, repo2, s2.tenants)
	if err != nil {
		t.Fatal(err)
	}
	err = repo2.View(ctx, func(tx Tx) error { _, e := tx.Get("member", "alpha", a2.ID); return e })
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("bootstrap restored removed owner", err)
	}
}
func TestConfigurationRejectsUnsafeAuthOptions(t *testing.T) {
	l, err := testauth.NewLDAP(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	valid := fmt.Sprintf("publicURL: https://slo.example.test\nproviders:\n- id: directory\n  name: Directory\n  type: ldap\n  url: %s\n  bindDN: cn=reader\n  bindPasswordEnv: READER_PASSWORD\n  userBaseDN: dc=example\n  userFilter: (uid={username})\n", l.URL)
	for _, body := range []string{strings.Replace(valid, l.URL, "ldap://127.0.0.1:389", 1), strings.Replace(valid, "https://slo.example.test", "http://slo.example.test", 1), strings.Replace(valid, "https://slo.example.test", "https://slo.example.test/path", 1), valid + "  insecureSkipVerify: true\n", strings.Replace(valid, "{username}", "*", 1), strings.Replace(valid, "name: Directory", "name: Directory\n  clientID: invalid", 1), strings.Replace(valid, "bindPasswordEnv: READER_PASSWORD", "bindPasswordEnv: missing", 1), valid + "---\n{}\n"} {
		if _, err = ParseConfig([]byte(body), func(string) string { return "secret" }); err == nil {
			t.Fatal("unsafe auth config accepted", body)
		}
	}
}
func TestAuditPaginationAndRepositoryIsolation(t *testing.T) {
	s, a := serviceFixture(t, NewMemory())
	ctx := context.Background()
	for i := 0; i < 205; i++ {
		if err := s.RecordAudit(ctx, "alpha", a, "POST", "/api/resources/apply", "HTTP 200", fmt.Sprint(i), `"2"`); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.AuditPage(ctx, a, "alpha", "")
	if err != nil || len(page) != 200 {
		t.Fatal(len(page), err)
	}
	next, err := s.AuditPage(ctx, a, "alpha", page[len(page)-1].ID)
	if err != nil || len(next) != 5 {
		t.Fatal(len(next), err)
	}
	other, err := s.AuditPage(ctx, a, "beta", "")
	if err != nil || len(other) != 0 {
		t.Fatal(other, err)
	}
	err = s.repo.View(ctx, func(tx Tx) error { return put(tx, "member", "alpha", "forged", Member{}, time.Time{}) })
	if !errors.Is(err, ErrForbidden) {
		t.Fatal("view mutated state")
	}
}
func TestPostgresIdentitySessionSurvivesRestart(t *testing.T) {
	db := os.Getenv("TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	testPostgresContract(t, db)
}

func TestExpiryAndSecureCookieAttributes(t *testing.T) {
	s, owner := serviceFixture(t, NewMemory())
	ctx := context.Background()
	token, _, err := s.login(ctx, "directory", externalUser{Subject: "alice", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	_, serviceToken, err := s.CreateAccount(ctx, owner, "alpha", "short", "Short", "viewer", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	original := s.Now
	s.Now = func() time.Time { return original().Add(25 * time.Hour) }
	if _, err = s.SessionActor(ctx, token); !errors.Is(err, ErrForbidden) {
		t.Fatal("expired browser session accepted", err)
	}
	if _, err = s.ServiceActor(ctx, serviceToken); !errors.Is(err, ErrForbidden) {
		t.Fatal("expired machine credential accepted", err)
	}
	s.Now = original
	s.config.PublicURL = "https://slo.example.test"
	w := httptest.NewRecorder()
	s.setCookie(w, "session", randomToken(), time.Now().Add(time.Hour))
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "__Host-wp_session" || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Domain != "" || cookies[0].Path != "/" {
		t.Fatal(cookies)
	}
}
