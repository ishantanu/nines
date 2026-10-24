package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testauth"
)

func TestBrowserIdentityTenantTeamsCSRFAndMachineBoundary(t *testing.T) {
	ctx := context.Background()
	ldap, err := testauth.NewLDAP(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer ldap.Close()
	server := httptest.NewUnstartedServer(nil)
	public := "http://" + server.Listener.Addr().String()
	config, err := identity.ParseConfig([]byte(fmt.Sprintf(`publicURL: %s
providers:
- id: directory
  name: Directory
  type: ldap
  url: %s
  caFile: %s
  bindDN: cn=reader,dc=example,dc=test
  bindPasswordEnv: READER
  userBaseDN: ou=people,dc=example,dc=test
  userFilter: (uid={username})
bootstrap:
- {tenant: alpha, provider: directory, subject: uuid-alice, role: owner}
- {tenant: beta, provider: directory, subject: uuid-bob, role: owner}
`, public, ldap.URL, ldap.CAFile)), func(string) string { return "fixture-reader" })
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(ctx, config, identity.NewMemory(), map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil)}, {ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil)}}, slog.New(slog.NewTextHandler(io.Discard, nil)), auth)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = h
	server.Start()
	defer server.Close()
	req := func(c *http.Client, tenant, token, csrf, method, path, body string, want int) map[string]json.RawMessage {
		t.Helper()
		r, _ := http.NewRequest(method, public+path, strings.NewReader(body))
		if strings.Contains(path, "resources/apply") {
			r.Header.Set("Content-Type", "application/yaml")
			r.Header.Set("If-Match", `"0"`)
		} else if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if tenant != "" {
			r.Header.Set("X-Nines-Tenant", tenant)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		response, e := c.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			_ = response.Body.Close()
		}()
		b, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s %s tenant=%s: %d want %d: %s", method, path, tenant, response.StatusCode, want, b)
		}
		out := map[string]json.RawMessage{}
		_ = json.Unmarshal(b, &out)
		return out
	}
	login := func(user string) (*http.Client, string) {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar}
		m := req(client, "", "", "", "GET", "/auth/session", "", 200)
		var csrf string
		if err := json.Unmarshal(m["csrfToken"], &csrf); err != nil {
			t.Fatal(err)
		}
		m = req(client, "", "", csrf, "POST", "/auth/ldap/directory/login", fmt.Sprintf(`{"username":%q,"password":%q}`, user, "fixture-"+user), 200)
		if err := json.Unmarshal(m["csrfToken"], &csrf); err != nil {
			t.Fatal(err)
		}
		return client, csrf
	}
	alice, aliceCSRF := login("alice")
	bob, bobCSRF := login("bob")
	req(alice, "alpha", "", "", "GET", "/api/slos", "", 200)
	req(alice, "beta", "", "", "GET", "/api/slos", "", 403)
	req(bob, "beta", "", bobCSRF, "POST", "/api/team-access/members", `{"provider":"directory","subject":"uuid-alice","role":"viewer","expectedVersion":0}`, 200)
	// Alice's existing session gains only the explicit beta viewer grant.
	req(alice, "beta", "", "", "GET", "/api/slos", "", 200)
	manifest := func(id string) string {
		return fmt.Sprintf("apiVersion: nines.dev/v1alpha1\nkind: ServiceLevelObjective\nmetadata:\n  name: %s\n  project: commerce\nspec:\n  displayName: Availability\n  service: checkout\n  target: 0.999\n  windowSeconds: 3600\n  stepSeconds: 60\n", id)
	}
	req(alice, "alpha", "", "", "POST", "/api/resources/apply", manifest("no-csrf"), 403)
	req(alice, "beta", "", aliceCSRF, "POST", "/api/resources/apply", manifest("beta-denied"), 403)
	req(alice, "alpha", "", aliceCSRF, "POST", "/api/resources/apply", manifest("owner-created"), 200)
	req(alice, "alpha", "unrelated-invalid-token", aliceCSRF, "GET", "/api/slos", "", 401)
	granted := req(alice, "alpha", "", aliceCSRF, "POST", "/api/team-access/members", `{"provider":"directory","subject":"uuid-bob","role":"viewer","expectedVersion":0}`, 200)
	var userID string
	if err := json.Unmarshal(granted["userId"], &userID); err != nil {
		t.Fatal(err)
	}
	teamBody := fmt.Sprintf(`{"id":"ops","name":"Ops","role":"editor","members":[%q],"expectedVersion":0}`, userID)
	req(alice, "alpha", "", aliceCSRF, "POST", "/api/team-access/teams", teamBody, 200)
	req(bob, "alpha", "", bobCSRF, "POST", "/api/resources/apply", manifest("editor-created"), 200)
	req(bob, "alpha", "", bobCSRF, "POST", "/api/team-access/members", `{"provider":"directory","subject":"uuid-bob","role":"owner","expectedVersion":2}`, 403)
	req(bob, "alpha", "", bobCSRF, "POST", "/api/slos/editor-created/rules/apply", `{}`, 403)
	req(bob, "alpha", "", bobCSRF, "POST", "/api/workflows/not-real/approve", `{}`, 403)
	req(alice, "alpha", "", aliceCSRF, "POST", "/api/team-access/teams", teamBody, 409)
	// Removing Bob's membership also drops his team, while his beta ownership remains.
	req(alice, "alpha", "", aliceCSRF, "POST", "/api/team-access/members", `{"provider":"directory","subject":"uuid-bob","role":"viewer","expectedVersion":1}`, 409)
	dir := req(alice, "alpha", "", "", "GET", "/api/team-access", "", 200)
	var members []identity.MemberView
	if err := json.Unmarshal(dir["members"], &members); err != nil {
		t.Fatal(err)
	}
	var version int64
	for _, m := range members {
		if m.UserID == userID {
			version = m.Version
		}
	}
	r, _ := http.NewRequest("DELETE", public+"/api/team-access/members/"+userID, nil)
	r.Header.Set("X-Nines-Tenant", "alpha")
	r.Header.Set("X-CSRF-Token", aliceCSRF)
	r.Header.Set("If-Match", fmt.Sprintf(`"%d"`, version))
	res, err := alice.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	req(bob, "alpha", "", "", "GET", "/api/slos", "", 403)
	req(bob, "beta", "", "", "GET", "/api/slos", "", 200)
	created := req(alice, "alpha", "", aliceCSRF, "POST", "/api/team-access/service-accounts", fmt.Sprintf(`{"id":"automation","name":"Automation","role":"agent","expiresAt":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339)), 201)
	var token string
	if err := json.Unmarshal(created["token"], &token); err != nil {
		t.Fatal(err)
	}
	machine := &http.Client{}
	req(machine, "alpha", token, "", "GET", "/api/slos", "", 200)
	req(machine, "beta", token, "", "GET", "/api/slos", "", 403)
	req(machine, "alpha", token, "", "POST", "/api/resources/apply", manifest("agent-denied"), 403)
	req(alice, "alpha", "", aliceCSRF, "DELETE", "/api/team-access/service-accounts/automation", "", 200)
	req(machine, "alpha", token, "", "GET", "/api/slos", "", 401)
	events := req(alice, "alpha", "", "", "GET", "/api/team-access/audit", "", 200)
	var audit []identity.Audit
	if err := json.Unmarshal(events["items"], &audit); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range audit {
		if e.ActorKind == "user" && e.ActorName == "Alice Directory" && e.Resource == "/api/resources/apply" && e.Outcome == "HTTP 200" {
			found = true
		}
	}
	if !found {
		t.Fatal("named mutation audit missing", audit)
	}
	b, _ := json.Marshal(events)
	if strings.Contains(string(b), token) || strings.Contains(string(b), "fixture-alice") || strings.Contains(string(b), aliceCSRF) {
		t.Fatal("audit exposed credentials")
	}
	req(alice, "", "", aliceCSRF, "POST", "/auth/logout", "", 200)
	req(alice, "alpha", "", "", "GET", "/api/slos", "", 401)
}
func TestEditorPermissionMatrix(t *testing.T) {
	for _, role := range []string{"", "administrator", "unknown"} {
		if allowed(role, httptest.NewRequest("GET", "/api/slos", nil)) {
			t.Fatal("unknown role obtained read access")
		}
	}
	for _, tc := range []struct {
		method, path string
		want         bool
	}{{"GET", "/api/slos", true}, {"POST", "/api/resources/apply", true}, {"POST", "/api/workflows", true}, {"POST", "/api/slos/s/samples", true}, {"POST", "/api/slos/s/rules/plan", true}, {"POST", "/api/slos/s/rules/apply", false}, {"POST", "/api/workflows/w/approve", false}, {"POST", "/api/connections/primary/test", false}, {"GET", "/api/team-access", false}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if allowed("editor", r) != tc.want {
			t.Fatalf("editor permission %s %s", tc.method, tc.path)
		}
	}
}

func TestSSOMappedRolesEnforcedByTenantAPI(t *testing.T) {
	ctx := context.Background()
	idp, err := testauth.NewOIDC()
	if err != nil {
		t.Fatal(err)
	}
	defer idp.Close()
	server := httptest.NewUnstartedServer(nil)
	public := "http://" + server.Listener.Addr().String()
	if err := idp.AllowRedirect(public + "/auth/oidc/company/callback"); err != nil {
		t.Fatal(err)
	}
	config, err := identity.ParseConfig([]byte(fmt.Sprintf(`publicURL: %s
providers:
- id: company
  name: Microsoft fixture
  type: oidc
  issuerURL: %s
  clientID: fixture-client
  clientSecretEnv: OIDC_SECRET
  claimsMaxAgeMinutes: 15
  roleMappings:
  - {id: admin, tenant: alpha, claim: roles, value: Reliability.Admin, role: owner}
  - {id: engineers, tenant: alpha, claim: groups, value: group-engineers, role: editor}
  - {id: readers, tenant: beta, claim: roles, value: Reliability.Reader, role: viewer}
`, public, idp.Server.URL)), func(string) string { return "fixture-secret" })
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(ctx, config, identity.NewMemory(), map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil)}, {ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil)}}, slog.New(slog.NewTextHandler(io.Discard, nil)), auth)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	login := func() (*http.Client, string) {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, e := client.Get(public + "/auth/oidc/company/login")
		if e != nil {
			t.Fatal(e)
		}
		_ = res.Body.Close()
		location, e := url.Parse(res.Header.Get("Location"))
		if e != nil {
			t.Fatal(e)
		}
		q := location.Query()
		q.Set("login_as", "alice")
		location.RawQuery = q.Encode()
		res, e = client.Get(location.String())
		if e != nil {
			t.Fatal(e)
		}
		_ = res.Body.Close()
		res, e = client.Get(res.Header.Get("Location"))
		if e != nil {
			t.Fatal(e)
		}
		_ = res.Body.Close()
		if res.StatusCode != 303 {
			t.Fatal(res.StatusCode)
		}
		res, e = client.Get(public + "/auth/session")
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			_ = res.Body.Close()
		}()
		var session struct {
			CSRF string `json:"csrfToken"`
		}
		if err := json.NewDecoder(res.Body).Decode(&session); err != nil {
			t.Fatal(err)
		}
		return client, session.CSRF
	}
	req := func(c *http.Client, csrf, tenant, method, path, body string, want int) []byte {
		t.Helper()
		r, _ := http.NewRequest(method, public+path, strings.NewReader(body))
		r.Header.Set("X-Nines-Tenant", tenant)
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		res, e := c.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			_ = res.Body.Close()
		}()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s tenant %s = %d want %d: %s", method, path, tenant, res.StatusCode, want, b)
		}
		return b
	}
	idp.SetAuthorizationClaims(map[string]any{"groups": []string{"group-engineers"}, "roles": []string{"Reliability.Reader"}})
	editor, csrf := login()
	// Exchange a browser-approved CLI grant after the real signed OIDC flow.
	verifier := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	startBody, _ := json.Marshal(map[string]string{"tenant": "alpha", "redirectURI": "http://127.0.0.1:54321/callback", "challenge": challenge, "state": verifier})
	started := req(editor, "", "", "POST", "/auth/cli/start", string(startBody), 200)
	var startedFlow struct {
		URL string `json:"authorizationURL"`
	}
	if err := json.Unmarshal(started, &startedFlow); err != nil {
		t.Fatal(err)
	}
	authorization, err := url.Parse(startedFlow.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := editor.Get(authorization.String())
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	_ = page.Body.Close()
	for _, visible := range []string{public, "alpha", "editor", "http://127.0.0.1:54321/callback"} {
		if !strings.Contains(string(pageBody), visible) {
			t.Fatal("authorization page omitted scope", visible)
		}
	}
	approval, err := editor.PostForm(public+"/auth/cli/approve", url.Values{"request": {authorization.Query().Get("request")}, "csrf": {csrf}, "decision": {"approve"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = approval.Body.Close()
	if approval.StatusCode != 303 {
		t.Fatal("CLI approval failed", approval.StatusCode)
	}
	callback, err := url.Parse(approval.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	exchange, _ := json.Marshal(map[string]string{"code": callback.Query().Get("code"), "codeVerifier": verifier, "redirectURI": "http://127.0.0.1:54321/callback"})
	exchanged := req(server.Client(), "", "", "POST", "/auth/cli/exchange", string(exchange), 200)
	var credential struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(exchanged, &credential); err != nil || credential.Token == "" {
		t.Fatal("CLI exchange failed", err)
	}
	cliRequest := func(tenant, path string, want int) {
		t.Helper()
		request, err := http.NewRequest("GET", public+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+credential.Token)
		request.Header.Set("X-Nines-Tenant", tenant)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != want {
			t.Fatalf("CLI %s %s: %d want %d", tenant, path, response.StatusCode, want)
		}
	}
	cliRequest("alpha", "/api/slos", 200)
	cliRequest("alpha", "/api/team-access", 403)
	cliRequest("beta", "/api/slos", 403)
	req(editor, "", "alpha", "GET", "/api/slos", "", 200)
	req(editor, "", "beta", "GET", "/api/slos", "", 200)
	req(editor, "", "alpha", "GET", "/api/team-access", "", 403)
	req(editor, csrf, "beta", "POST", "/api/resources/apply", "{}", 403)
	req(editor, csrf, "alpha", "POST", "/api/slos/not-present/rules/apply", "{}", 403)
	idp.SetAuthorizationClaims(map[string]any{"roles": []string{"Reliability.Admin"}})
	owner, ownerCSRF := login()
	directory := req(owner, "", "alpha", "GET", "/api/team-access", "", 200)
	if !strings.Contains(string(directory), "ssoMappings") || strings.Contains(string(directory), "Reliability.Reader") {
		t.Fatal("mapping view crossed tenant", string(directory))
	}
	// A mapped owner passes both the HTTP role gate and transactional owner check.
	req(owner, "", "alpha", "POST", "/api/team-access/members", `{"provider":"company","subject":"bob","role":"viewer","expectedVersion":0}`, 403)
	req(owner, ownerCSRF, "alpha", "POST", "/api/team-access/members", `{"provider":"company","subject":"bob","role":"viewer","expectedVersion":0}`, 200)
	cliRequest("alpha", "/api/team-access", 200)
	// New claims replace the user's previous evidence for every existing session.
	req(editor, "", "beta", "GET", "/api/slos", "", 403)
	idp.SetAuthorizationClaims(map[string]any{})
	login()
	req(owner, "", "alpha", "GET", "/api/team-access", "", 403)
	req(editor, "", "alpha", "GET", "/api/slos", "", 403)
	cliRequest("alpha", "/api/slos", 401)
}
