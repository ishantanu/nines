package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/testauth"
)

func oidcHarness(t *testing.T) (*harness, *testauth.OIDC) {
	t.Helper()
	requireFixture(t)
	idp, err := testauth.NewOIDC()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(idp.Close)
	h := newHarness(t, func(public string) identity.Config {
		return parseConfig(t, fmt.Sprintf(`publicURL: %s
providers:
- id: company
  name: Entra-shaped protocol fixture
  type: oidc
  issuerURL: %s
  clientID: fixture-client
  clientSecretEnv: ACCEPTANCE_OIDC_SECRET
  claimsMaxAgeMinutes: 5
  roleMappings:
  - {id: admins, tenant: alpha, claim: roles, value: Reliability.Admin, role: owner}
  - {id: engineers, tenant: alpha, claim: groups, value: group-engineers, role: editor}
  - {id: readers, tenant: beta, claim: roles, value: Reliability.Reader, role: viewer}
bootstrap:
- {tenant: alpha, provider: company, subject: bob, role: owner}
`, public, idp.Server.URL), func(string) string { return "fixture-secret" })
	})
	if err := idp.AllowRedirect(h.server.URL + "/auth/oidc/company/callback"); err != nil {
		t.Fatal(err)
	}
	return h, idp
}

func TestRealIdentityOIDCBrowserCLIRevocation(t *testing.T) {
	h, idp := oidcHarness(t)
	editor, csrf := h.oidc(idp, "alice", map[string]any{"groups": []string{"group-engineers"}, "roles": []string{"Reliability.Reader"}})
	h.call(editor, "alpha", "", "", "GET", "/api/slos", "", 200)
	h.call(editor, "beta", "", "", "GET", "/api/slos", "", 200)
	h.call(editor, "alpha", "", csrf, "POST", "/api/resources/apply", sloBody("editor-created"), 200)
	h.call(editor, "beta", "", csrf, "POST", "/api/resources/apply", sloBody("viewer-denied"), 403)
	h.call(editor, "alpha", "", csrf, "POST", "/api/slos/editor-created/rules/apply", "{}", 403)
	h.call(editor, "alpha", "", "", "GET", "/api/team-access", "", 403)
	config := h.nativeLogin(editor, csrf, "alpha", "approve")
	h.nativeStatus(config, "alpha", 0, "editor")
	h.nativeStatus(config, "beta", 3, "")
	token := h.cliGrant(editor, csrf, "alpha", "approve")
	h.call(browser(), "beta", token, "", "GET", "/api/slos", "", 403)
	// Native client and browser sessions survive an actual repository reopen.
	h.restart()
	h.call(editor, "alpha", "", "", "GET", "/api/slos", "", 200)
	h.nativeStatus(config, "alpha", 0, "editor")
	owner, ownerCSRF := h.oidc(idp, "alice", map[string]any{"roles": []string{"Reliability.Admin"}})
	h.nativeStatus(config, "alpha", 0, "owner")
	h.call(editor, "beta", "", "", "GET", "/api/slos", "", 403)
	h.call(owner, "alpha", "", ownerCSRF, "POST", "/api/team-access/members", `{"provider":"company","subject":"outsider","role":"viewer","expectedVersion":0}`, 200)
	// A new verified, empty claims snapshot removes mappings from old sessions.
	h.oidc(idp, "alice", map[string]any{})
	h.call(editor, "alpha", "", "", "GET", "/api/slos", "", 403)
	h.call(owner, "alpha", "", "", "GET", "/api/slos", "", 403)
	h.nativeStatus(config, "alpha", 3, "")
	h.call(browser(), "alpha", token, "", "GET", "/api/slos", "", 401)
	editor, csrf = h.oidc(idp, "alice", map[string]any{"groups": []string{"group-engineers"}})
	h.nativeStatus(config, "alpha", 0, "editor")
	h.call(editor, "", "", csrf, "POST", "/auth/logout-all", "", 200)
	h.call(editor, "alpha", "", "", "GET", "/api/slos", "", 401)
	h.nativeStatus(config, "alpha", 3, "")
	t.Log("Verified signed OIDC, native CLI approval/exchange, tenant RBAC, persistent restart, live role loss and logout-all revocation")
}

func TestRealIdentityOIDCClaimExpiry(t *testing.T) {
	h, idp := oidcHarness(t)
	editor, csrf := h.oidc(idp, "alice", map[string]any{"groups": []string{"group-engineers"}})
	config := h.nativeLogin(editor, csrf, "alpha", "approve")
	actor, _ := h.session(editor)
	if !actor.Expires.After(time.Now()) || actor.Expires.After(time.Now().Add(5*time.Minute)) {
		t.Fatal("mapped session exceeded verified-claim lifetime")
	}
	h.nativeStatus(config, "alpha", 0, "editor")
	// Advance only this fixture's clock, without sleeps or rewinding an expired credential.
	h.offset.Store(int64(6 * time.Minute))
	h.call(editor, "alpha", "", "", "GET", "/api/slos", "", 401)
	h.nativeStatus(config, "alpha", 3, "")
	t.Log("Verified mapped browser/CLI expiry precedes the token and configured session ceiling")
}

func TestRealIdentityOIDCClaimsBoundaries(t *testing.T) {
	requireFixture(t)
	for index, claims := range []map[string]any{
		{}, {"roles": []string{"Reliability.Admin.extra"}},
		{"groups": []string{"group-engineers"}, "hasgroups": false},
		{"groups": []string{"group-engineers"}, "_claim_names": map[string]any{"groups": "upstream"}, "_claim_sources": map[string]any{"upstream": map[string]string{"endpoint": "http://127.0.0.1:1/must-not-fetch"}}},
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			h, idp := oidcHarness(t)
			outsider, _ := h.oidc(idp, "alice", claims)
			h.call(outsider, "alpha", "", "", "GET", "/api/slos", "", 403)
			h.call(outsider, "beta", "", "", "GET", "/api/slos", "", 403)
		})
	}
}

func TestRealIdentityTeamsAndProviderBinding(t *testing.T) {
	h, idp := oidcHarness(t)
	admin, adminCSRF := h.oidc(idp, "bob", map[string]any{})
	alice, aliceCSRF := h.oidc(idp, "alice", map[string]any{"groups": []string{"group-engineers"}})
	config := h.nativeLogin(alice, aliceCSRF, "alpha", "approve")
	member := decode[identity.Member](t, h.call(admin, "alpha", "", adminCSRF, "POST", "/api/team-access/members", `{"provider":"company","subject":"alice","role":"viewer","expectedVersion":0}`, 200))
	team, _ := json.Marshal(map[string]any{"id": "acceptance-team", "name": "Acceptance", "role": "editor", "members": []string{member.UserID}, "expectedVersion": 0})
	h.call(admin, "alpha", "", adminCSRF, "POST", "/api/team-access/teams", string(team), 200)
	// Empty SSO claims do not revoke independently reviewed direct/team grants.
	h.oidc(idp, "alice", map[string]any{})
	h.nativeStatus(config, "alpha", 0, "editor")
	team, _ = json.Marshal(map[string]any{"id": "acceptance-team", "name": "Acceptance", "role": "editor", "members": []string{}, "expectedVersion": 1})
	h.call(admin, "alpha", "", adminCSRF, "POST", "/api/team-access/teams", string(team), 200)
	h.nativeStatus(config, "alpha", 0, "viewer")
	h.call(alice, "alpha", "", aliceCSRF, "POST", "/api/resources/apply", sloBody("team-removed"), 403)
	directory := decode[struct {
		Members []identity.MemberView `json:"members"`
	}](t, h.call(admin, "alpha", "", "", "GET", "/api/team-access", "", 200))
	version := int64(0)
	for _, m := range directory.Members {
		if m.UserID == member.UserID {
			version = m.Version
		}
	}
	if version == 0 {
		t.Fatal("member revision unavailable")
	}
	remove, err := http.NewRequest("DELETE", h.server.URL+"/api/team-access/members/"+member.UserID, nil)
	if err != nil {
		t.Fatal(err)
	}
	remove.Header.Set("X-Nines-Tenant", "alpha")
	remove.Header.Set("X-CSRF-Token", adminCSRF)
	remove.Header.Set("If-Match", fmt.Sprintf(`"%d"`, version))
	response, err := admin.Do(remove)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("membership removal failed")
	}
	h.nativeStatus(config, "alpha", 3, "")
	h.call(alice, "alpha", "", "", "GET", "/api/slos", "", 403)
	config = h.nativeLogin(admin, adminCSRF, "alpha", "approve")
	h.nativeStatus(config, "alpha", 0, "owner")
	audit := h.call(admin, "alpha", "", "", "GET", "/api/team-access/audit", "", 200)
	credentials, err := os.ReadFile(config + ".credentials.json")
	if err != nil {
		t.Fatal("native credential unavailable for audit redaction check")
	}
	saved := decode[map[string]struct {
		Token string `json:"token"`
	}](t, credentials)
	for _, secret := range []string{saved["acceptance"].Token, adminCSRF, aliceCSRF, "fixture-secret", "group-engineers", fixtureSecret(t, "user-password"), fixtureSecret(t, "reader-password")} {
		if secret != "" && bytes.Contains(audit, []byte(secret)) {
			t.Fatal("identity audit exposed credential or raw authorization claims")
		}
	}
	// Removing the authenticating client identity invalidates even direct owners.
	h.config.Providers[0].ClientID = "replacement-client"
	h.restart()
	h.call(admin, "alpha", "", "", "GET", "/api/slos", "", 401)
	h.nativeStatus(config, "alpha", 3, "")
	t.Log("Verified exact claims/overage denial, independent direct/team membership, role reduction and provider identity changes")
}

func TestRealIdentityCLIApprovalCancellation(t *testing.T) {
	h, idp := oidcHarness(t)
	editor, csrf := h.oidc(idp, "alice", map[string]any{"groups": []string{"group-engineers"}})
	h.nativeLogin(editor, csrf, "alpha", "deny")
	h.cliGrant(editor, csrf, "alpha", "deny")
	// Every authorization page discloses exact server, tenant and loopback return.
	verifier := opaque(t)
	body := fmt.Sprintf(`{"tenant":"alpha","redirectURI":"http://127.0.0.1:54321/callback","challenge":%q,"state":%q}`, verifier, opaque(t))
	flow := decode[struct {
		URL string `json:"authorizationURL"`
	}](t, h.call(browser(), "", "", "", "POST", "/auth/cli/start", body, 200))
	u, _ := url.Parse(flow.URL)
	response, err := editor.Get(flow.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	response, err = editor.PostForm(h.server.URL+"/auth/cli/approve", url.Values{"request": {u.Query().Get("request")}, "csrf": {"wrong"}, "decision": {"approve"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("CLI approval accepted invalid CSRF")
	}
	// The rejected action cannot invalidate or auto-approve the legitimate request.
	callback := h.approve(editor, csrf, flow.URL, "alpha", "deny")
	if callback.Query().Get("code") != "" {
		t.Fatal("denial issued grant")
	}
	for _, uri := range []string{"https://example.test/callback", "http://127.0.0.1:54321/callback?token=x", "http://localhost:54321/callback"} {
		invalid := strings.Replace(body, "http://127.0.0.1:54321/callback", uri, 1)
		h.call(browser(), "", "", "", "POST", "/auth/cli/start", invalid, 400)
	}
	t.Log("Verified actual CLI cancellation leaves no credential, consumed flow replay fails, scope is visible and CSRF/callback rejection preserves deny-by-default")
}
