package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/testauth"
)

func cliIdentity(t *testing.T) (*Service, *httptest.Server, string, Actor) {
	t.Helper()
	l, e := testauth.NewLDAP(t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Close)
	h := httptest.NewUnstartedServer(nil)
	s, e := New(context.Background(), ldapConfig(t, l, "http://"+h.Listener.Addr().String()), NewMemory(), map[string]bool{"alpha": true, "beta": true})
	if e != nil {
		t.Fatal(e)
	}
	h.Config.Handler = s.Handler()
	h.Start()
	t.Cleanup(h.Close)
	token, a, e := s.login(context.Background(), "directory", externalUser{Subject: "uuid-alice", Name: "Alice"})
	if e != nil {
		t.Fatal(e)
	}
	return s, h, token, a
}
func startCLI(t *testing.T, h *httptest.Server, tenant string) (string, string, string) {
	t.Helper()
	verifier := randomToken()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	res := request(t, h.Client(), "POST", h.URL+"/auth/cli/start", "", `{"tenant":"`+tenant+`","redirectURI":"http://127.0.0.1:54321/callback","challenge":"`+challenge+`","state":"`+randomToken()+`"}`, 200)
	var u string
	if err := json.Unmarshal(res["authorizationURL"], &u); err != nil {
		t.Fatal(err)
	}
	return u, verifier, challenge
}
func authorizeCLI(t *testing.T, s *Service, h *httptest.Server, session, authorization string) string {
	t.Helper()
	u, _ := url.Parse(authorization)
	ticket := u.Query().Get("request")
	req := httptest.NewRequest("POST", "/auth/cli/approve", strings.NewReader(url.Values{"request": {ticket}, "csrf": {hash("csrf:" + session)}, "decision": {"approve"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: s.cookieName("session"), Value: session})
	req.AddCookie(&http.Cookie{Name: s.cookieName("cli_flow"), Value: ticket})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 303 {
		t.Fatalf("approve %d: %s", w.Code, w.Body.String())
	}
	callback, _ := url.Parse(w.Header().Get("Location"))
	return callback.Query().Get("code")
}
func exchangeBody(code, verifier string) string {
	b, _ := json.Marshal(map[string]string{"code": code, "codeVerifier": verifier, "redirectURI": "http://127.0.0.1:54321/callback"})
	return string(b)
}
func TestCLIGrantPKCEReplayTenantAndLiveMembership(t *testing.T) {
	s, h, session, a := cliIdentity(t)
	u, verifier, _ := startCLI(t, h, "alpha")
	code := authorizeCLI(t, s, h, session, u)
	request(t, h.Client(), "POST", h.URL+"/auth/cli/exchange", "", exchangeBody(code, randomToken()), 403)
	result := request(t, h.Client(), "POST", h.URL+"/auth/cli/exchange", "", exchangeBody(code, verifier), 200)
	var token string
	if err := json.Unmarshal(result["token"], &token); err != nil {
		t.Fatal(err)
	}
	actor, e := s.CLIActor(context.Background(), token)
	if e != nil || actor.Session || actor.Grants["alpha"] != "owner" || actor.Grants["beta"] != "" || actor.ID != a.ID {
		t.Fatalf("wrong actor: %+v %v", actor, e)
	}
	request(t, h.Client(), "POST", h.URL+"/auth/cli/exchange", "", exchangeBody(code, verifier), 404)
	// A role reduction takes effect without reissuing a CLI token.
	e = s.repo.Update(context.Background(), "tenant:alpha", func(tx Tx) error { return s.saveMember(tx, "alpha", Member{a.ID, "viewer", "viewer", 2}) })
	if e != nil {
		t.Fatal(e)
	}
	actor, e = s.CLIActor(context.Background(), token)
	if e != nil || actor.Grants["alpha"] != "viewer" {
		t.Fatalf("cached role: %+v %v", actor, e)
	}
	e = s.repo.Update(context.Background(), "session", func(tx Tx) error { return tx.Delete("session", "global", hash(session)) })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CLIActor(context.Background(), token); e == nil {
		t.Fatal("source session logout did not revoke CLI")
	}
}
func TestCLIFlowRequiresCSRFAndExactlyOneCodeConsumer(t *testing.T) {
	s, h, session, _ := cliIdentity(t)
	u, verifier, _ := startCLI(t, h, "alpha")
	parsed, _ := url.Parse(u)
	ticket := parsed.Query().Get("request")
	r := httptest.NewRequest("POST", "/auth/cli/approve", strings.NewReader(url.Values{"request": {ticket}, "csrf": {"wrong"}, "decision": {"approve"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: s.cookieName("session"), Value: session})
	r.AddCookie(&http.Cookie{Name: s.cookieName("cli_flow"), Value: ticket})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	code := authorizeCLI(t, s, h, session, u)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, e := h.Client().Post(h.URL+"/auth/cli/exchange", "application/json", strings.NewReader(exchangeBody(code, verifier)))
			if e != nil {
				t.Error(e)
				return
			}
			defer func() {
				_ = res.Body.Close()
			}()
			if _, err := io.Copy(io.Discard, res.Body); err != nil {
				t.Error(err)
				return
			}
			if res.StatusCode == 200 {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("authorization code reused", successes.Load())
	}
}
func TestCLIRejectsCallbackAndWrongTenantAndExpiredFlow(t *testing.T) {
	s, h, session, _ := cliIdentity(t)
	for _, uri := range []string{"https://remote.test/callback", "http://localhost:123/callback", "http://127.0.0.1:123/other", "http://127.0.0.1:123/callback?x=1", "http://user@127.0.0.1:123/callback", "http://127.0.0.1:0/callback"} {
		b, _ := json.Marshal(map[string]string{"tenant": "alpha", "redirectURI": uri, "challenge": randomToken(), "state": randomToken()})
		request(t, h.Client(), "POST", h.URL+"/auth/cli/start", "", string(b), 400)
	}
	u, _, _ := startCLI(t, h, "beta")
	r := httptest.NewRequest("GET", u, nil)
	r.AddCookie(&http.Cookie{Name: s.cookieName("session"), Value: session})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("unauthorized tenant", w.Code)
	}
	u, _, _ = startCLI(t, h, "alpha")
	s.Now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	r = httptest.NewRequest("GET", u, nil)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("expired request", w.Code)
	}
}
func TestCLIOIDCSignInReturnsToExplicitApproval(t *testing.T) {
	idp, e := testauth.NewOIDC()
	if e != nil {
		t.Fatal(e)
	}
	defer idp.Close()
	h := httptest.NewUnstartedServer(nil)
	public := "http://" + h.Listener.Addr().String()
	if err := idp.AllowRedirect(public + "/auth/oidc/company/callback"); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PublicURL: public, SessionHours: 12, Providers: []ProviderConfig{{ID: "company", Name: "Company SSO", Type: "oidc", IssuerURL: idp.Server.URL, ClientID: "fixture-client", secret: "fixture-secret"}}, Bootstrap: []Bootstrap{{"alpha", "company", "alice", "owner"}}}
	s, e := New(context.Background(), cfg, NewMemory(), map[string]bool{"alpha": true})
	if e != nil {
		t.Fatal(e)
	}
	h.Config.Handler = s.Handler()
	h.Start()
	defer h.Close()
	c := newClient()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u, _, _ := startCLI(t, h, "alpha")
	res, e := c.Get(u)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.Header.Get("Referrer-Policy") != "strict-origin" {
		t.Fatal("native form Origin would be suppressed")
	}
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "http://127.0.0.1:54321") {
		t.Fatal("form redirect destination is blocked")
	}
	if !strings.Contains(string(body), "Sign in with Company SSO") {
		t.Fatal("missing SSO entry")
	}
	res, e = c.Get(h.URL + "/auth/oidc/company/login")
	if e != nil {
		t.Fatal(e)
	}
	_ = res.Body.Close()
	provider, _ := url.Parse(res.Header.Get("Location"))
	q := provider.Query()
	q.Set("login_as", "alice")
	provider.RawQuery = q.Encode()
	res, e = c.Get(provider.String())
	if e != nil {
		t.Fatal(e)
	}
	_ = res.Body.Close()
	res, e = c.Get(res.Header.Get("Location"))
	if e != nil {
		t.Fatal(e)
	}
	_ = res.Body.Close()
	if res.StatusCode != 303 || res.Header.Get("Location") != u {
		t.Fatal("lost CLI return", res.StatusCode, res.Header.Get("Location"))
	}
	res, e = c.Get(u)
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "Authorize CLI") || !strings.Contains(string(body), "owner") {
		t.Fatal("missing explicit approval")
	}
}

func TestCLITokenLogoutAndExpiry(t *testing.T) {
	s, h, session, _ := cliIdentity(t)
	u, verifier, _ := startCLI(t, h, "alpha")
	code := authorizeCLI(t, s, h, session, u)
	out := request(t, h.Client(), "POST", h.URL+"/auth/cli/exchange", "", exchangeBody(code, verifier), 200)
	var token string
	if err := json.Unmarshal(out["token"], &token); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", h.URL+"/auth/cli/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := h.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	if _, e = s.CLIActor(context.Background(), token); e == nil {
		t.Fatal("revoked CLI token accepted")
	}
	if _, e = s.SessionActor(context.Background(), session); e != nil {
		t.Fatal("CLI logout revoked browser session", e)
	}
	u, verifier, _ = startCLI(t, h, "alpha")
	code = authorizeCLI(t, s, h, session, u)
	out = request(t, h.Client(), "POST", h.URL+"/auth/cli/exchange", "", exchangeBody(code, verifier), 200)
	if err := json.Unmarshal(out["token"], &token); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return time.Now().Add(61 * time.Minute) }
	if _, e = s.CLIActor(context.Background(), token); e == nil {
		t.Fatal("expired CLI token accepted")
	}
}

func TestCLINativeLDAPFormSignIn(t *testing.T) {
	s, h, _, _ := cliIdentity(t)
	u, _, _ := startCLI(t, h, "alpha")
	c := newClient()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, e := c.Get(u)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	marker := `name="csrf" value="`
	_, rest, ok := strings.Cut(string(body), marker)
	if !ok {
		t.Fatal("missing login CSRF")
	}
	csrf, _, _ := strings.Cut(rest, `"`)
	form := url.Values{"provider": {"directory"}, "username": {"alice"}, "password": {"fixture-alice"}, "csrf": {csrf}}
	r, _ := http.NewRequest("POST", h.URL+"/auth/cli/ldap", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", h.URL)
	res, e = c.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	_ = res.Body.Close()
	if res.StatusCode != 303 || res.Header.Get("Location") != u {
		t.Fatal("LDAP CLI return failed", res.StatusCode)
	}
	res, e = c.Get(u)
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "Authorize CLI") {
		t.Fatal("LDAP login did not reach approval")
	}
	parsed, _ := url.Parse(u)
	if _, e = s.readCLIFlow(context.Background(), parsed.Query().Get("request")); e != nil {
		t.Fatal(e)
	}
}
