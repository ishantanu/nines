package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/testauth"
)

func mappingFixture(t *testing.T, repo Repository) *Service {
	t.Helper()
	p := ProviderConfig{ID: "microsoft", Type: "oidc", Name: "Microsoft", IssuerURL: "https://login.microsoftonline.com/test/v2.0", ClientID: "app", ClaimsMaxAgeMinutes: 15, RoleMappings: []RoleMapping{
		{ID: "readers", Tenant: "alpha", Claim: "roles", Value: "Reliability.Reader", Role: "viewer"},
		{ID: "engineers", Tenant: "alpha", Claim: "groups", Value: "group-engineers", Role: "editor"},
		{ID: "admins", Tenant: "alpha", Claim: "roles", Value: "Reliability.Admin", Role: "owner"},
		{ID: "beta-reader", Tenant: "beta", Claim: "roles", Value: "Reliability.Reader", Role: "viewer"},
	}}
	return &Service{repo: repo, providers: map[string]provider{"microsoft": {config: p}}, tenants: map[string]bool{"alpha": true, "beta": true}, config: Config{SessionHours: 12}, Now: time.Now}
}
func evidence(groups, roles []string) *authorizationClaims {
	return &authorizationClaims{Groups: groups, Roles: roles, IssuedAt: time.Now().Add(-time.Second), TokenExpires: time.Now().Add(time.Hour)}
}
func mappedLogin(t *testing.T, s *Service, sub string, claims *authorizationClaims) (string, Actor) {
	t.Helper()
	token, a, err := s.login(context.Background(), "microsoft", externalUser{Subject: sub, Name: sub, Authorization: claims})
	if err != nil {
		t.Fatal(err)
	}
	return token, a
}
func TestSSOMappingPrecedenceIsolationRevocationAndExpiry(t *testing.T) {
	ctx := context.Background()
	s := mappingFixture(t, NewMemory())
	token, a := mappedLogin(t, s, "alice", evidence([]string{"group-engineers"}, []string{"Reliability.Reader"}))
	if a.Grants["alpha"] != "editor" || a.Grants["beta"] != "viewer" || len(a.Grants) != 2 {
		t.Fatal(a)
	}
	if a.Expires.After(s.Now().Add(15 * time.Minute)) {
		t.Fatal("unbounded mapped session")
	}
	if err :=
		// No direct membership is created by federation.
		s.repo.View(ctx, func(tx Tx) error {
			if _, e := tx.Get("member", "alpha", a.ID); !errors.Is(e, ErrNotFound) {
				t.Fatal("SSO copied into direct membership")
			}
			return nil
		}); err !=
		// Local grants are independent and survive losing an upstream assignment.
		nil {
		t.Fatal(err)
	}

	err := s.repo.Update(ctx, "tenant:alpha", func(tx Tx) error {
		return s.saveMember(tx, "alpha", Member{UserID: a.ID, Role: "agent", EffectiveRole: "agent", Version: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	_, b := mappedLogin(t, s, "alice", evidence(nil, nil))
	old, err := s.SessionActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if old.Grants["alpha"] != "agent" || b.Grants["alpha"] != "agent" || old.Grants["beta"] != "" {
		t.Fatal("old session retained revoked claims", old, b)
	}
	// No fallback by domain/name or wildcard; another provider cannot inherit.
	_, outsider := mappedLogin(t, s, "outsider", evidence([]string{"group-engineers-extra"}, []string{"Reliability.*"}))
	if len(outsider.Grants) != 0 {
		t.Fatal(outsider)
	}
	_, admin := mappedLogin(t, s, "admin", evidence(nil, []string{"Reliability.Admin"}))
	if admin.Grants["alpha"] != "owner" {
		t.Fatal(admin)
	}
	if err = s.repo.View(ctx, func(tx Tx) error { return s.checkOwner(tx, "alpha", admin) }); err != nil {
		t.Fatal("mapped owner rejected", err)
	}
	mappedLogin(t, s, "admin", evidence(nil, nil))
	if err = s.repo.View(ctx, func(tx Tx) error { return s.checkOwner(tx, "alpha", admin) }); !errors.Is(err, ErrForbidden) {
		t.Fatal("stale owner accepted", err)
	}
	// Expired federation does not survive via a longer browser cookie.
	expiredToken, _ := mappedLogin(t, s, "expiry", evidence(nil, []string{"Reliability.Reader"}))
	s.Now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if _, err = s.SessionActor(ctx, expiredToken); !errors.Is(err, ErrForbidden) {
		t.Fatal("expired session retained mapped grant", err)
	}
}
func TestSSOMappingPolicyChangeProviderBindingAndDirectory(t *testing.T) {
	ctx := context.Background()
	s := mappingFixture(t, NewMemory())
	token, a := mappedLogin(t, s, "alice", evidence([]string{"group-engineers"}, []string{"Reliability.Admin", "Reliability.Reader"}))
	directory, err := s.Directory(ctx, a, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(directory)
	if strings.Contains(string(b), "beta-reader") || strings.Contains(string(b), "tokenHash") {
		t.Fatal("cross-tenant policy or secret leaked", string(b))
	}
	view := directory.(map[string]any)
	if len(view["ssoMappings"].([]mappingView)) != 3 || len(view["ssoMembers"].([]federatedMember)) != 1 {
		t.Fatal(view)
	}
	// Reloaded server policy is applied to an existing claims snapshot immediately.
	p := s.providers["microsoft"]
	p.config.RoleMappings = []RoleMapping{{ID: "reader", Tenant: "alpha", Claim: "roles", Value: "Reliability.Reader", Role: "viewer"}}
	s.providers["microsoft"] = p
	current, err := s.SessionActor(ctx, token)
	if err != nil || current.Grants["alpha"] != "viewer" || len(current.Grants) != 1 {
		t.Fatal(current, err)
	}
	p.config.ClientID = "another-app"
	s.providers["microsoft"] = p
	current, err = s.SessionActor(ctx, token)
	if !errors.Is(err, ErrForbidden) {
		t.Fatal("session crossed client boundary", current, err)
	}
	p.config.ClientID = "app"
	p.config.IssuerURL = "https://another.invalid"
	s.providers["microsoft"] = p
	current, err = s.SessionActor(ctx, token)
	if !errors.Is(err, ErrForbidden) {
		t.Fatal("session crossed issuer boundary", current, err)
	}
}
func TestSSOMappingDirectOwnerAndTeamRemainIndependent(t *testing.T) {
	ctx := context.Background()
	s := mappingFixture(t, NewMemory())
	_, owner := mappedLogin(t, s, "owner", evidence(nil, nil))
	if err := s.repo.Update(ctx, "tenant:alpha", func(tx Tx) error { return s.saveMember(tx, "alpha", Member{owner.ID, "owner", "owner", 1}) }); err != nil {
		t.Fatal(err)
	}
	token, person := mappedLogin(t, s, "person", evidence([]string{"group-engineers"}, nil))
	member, err := s.Grant(ctx, owner, "alpha", "microsoft", "person", "viewer", 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveTeam(ctx, owner, "alpha", Team{ID: "analysts", Name: "Analysts", Role: "agent", Members: []string{person.ID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.SessionActor(ctx, token)
	if err != nil || a.Grants["alpha"] != "editor" {
		t.Fatal(a, err)
	}
	mappedLogin(t, s, "person", evidence(nil, nil))
	a, err = s.SessionActor(ctx, token)
	if err != nil || a.Grants["alpha"] != "agent" {
		t.Fatal(a, err)
	}
	if err :=
		// CAS version changed by team recomputation; get the actual persisted version.
		s.repo.View(ctx, func(tx Tx) error { member, err = read[Member](tx, "member", "alpha", person.ID); return err }); err != nil {
		t.Fatal(err)
	}
	if err = s.RemoveMember(ctx, owner, "alpha", person.ID, member.Version); err != nil {
		t.Fatal(err)
	}
	a, err = s.SessionActor(ctx, token)
	if err != nil || len(a.Grants) != 0 {
		t.Fatal(a, err)
	}
	if err = s.RemoveMember(ctx, owner, "alpha", owner.ID, 1); err == nil {
		t.Fatal("last direct owner lost protection")
	}
}
func TestSSOMappingConfigurationValidation(t *testing.T) {
	base := `publicURL: https://slo.example.test
providers:
- id: microsoft
  name: Microsoft Entra ID
  type: oidc
  issuerURL: https://login.microsoftonline.com/directory/v2.0
  clientID: app
  roleMappings:
  - id: readers
    tenant: alpha
    claim: roles
    value: Reliability.Reader
    role: viewer
`
	c, err := ParseConfig([]byte(base), func(string) string { return "" })
	if err != nil || c.Providers[0].ClaimsMaxAgeMinutes != 15 {
		t.Fatal(c, err)
	}
	for name, body := range map[string]string{
		"unknown role":    strings.Replace(base, "role: viewer", "role: root", 1),
		"wrong claim":     strings.Replace(base, "claim: roles", "claim: email", 1),
		"wildcard":        strings.Replace(base, "Reliability.Reader", "Reliability.*", 1),
		"implicit tenant": strings.Replace(base, "tenant: alpha", "tenant: '*'", 1),
		"unknown field":   base + "    admin: true\n",
		"duplicate id":    base + "  - id: readers\n    tenant: beta\n    claim: roles\n    value: Other\n    role: owner\n",
		"duplicate match": base + "  - id: another\n    tenant: alpha\n    claim: roles\n    value: Reliability.Reader\n    role: owner\n",
		"age upper":       strings.Replace(base, "  clientID: app", "  clientID: app\n  claimsMaxAgeMinutes: 61", 1),
		"age negative":    strings.Replace(base, "  clientID: app", "  clientID: app\n  claimsMaxAgeMinutes: -1", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(body), func(string) string { return "" }); err == nil {
				t.Fatal("unsafe mapping accepted")
			}
		})
	}
	// This must fail before attempting provider discovery.
	if _, err = New(context.Background(), c, NewMemory(), map[string]bool{"beta": true}); err == nil || !strings.Contains(err.Error(), "unconfigured tenant") {
		t.Fatal(err)
	}
	if err = validateMappings(&ProviderConfig{Type: "ldap", RoleMappings: c.Providers[0].RoleMappings}); err == nil {
		t.Fatal("LDAP mappings accepted")
	}
	b, err := os.ReadFile("../../examples/auth-entra.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseConfig(b, func(string) string { return "synthetic-test-secret" }); err != nil {
		t.Fatal("shipped Entra example is invalid", err)
	}
}

func completeSSO(t *testing.T, client *http.Client, base string) int {
	t.Helper()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Get(base + "/auth/oidc/company/login")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	u, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("login_as", "alice")
	u.RawQuery = q.Encode()
	res, err = client.Get(u.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res, err = client.Get(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}
func TestSSOClaimsFromSignedOIDCTokens(t *testing.T) {
	for _, tc := range []struct {
		name       string
		claims     map[string]any
		wantStatus int
		want       string
	}{
		{"group", map[string]any{"groups": []string{"group-engineers"}}, 303, "editor"},
		{"role", map[string]any{"roles": []string{"Reliability.Reader"}}, 303, "viewer"},
		{"highest", map[string]any{"groups": []string{"group-engineers"}, "roles": []string{"Reliability.Admin"}}, 303, "owner"},
		{"unmapped", map[string]any{"groups": []string{"unknown"}}, 303, ""},
		{"hasgroups partial", map[string]any{"groups": []string{"group-engineers"}, "hasgroups": true}, 303, ""},
		{"overage app role", map[string]any{"groups": []string{"group-engineers"}, "roles": []string{"Reliability.Reader"}, "_claim_names": map[string]string{"groups": "src1"}, "_claim_sources": map[string]any{"src1": map[string]string{"endpoint": "https://must-not-fetch.invalid"}}}, 303, "viewer"},
		{"string roles", map[string]any{"roles": "Reliability.Admin"}, 403, ""},
		{"mixed roles", map[string]any{"roles": []any{"Reliability.Admin", 7}}, 403, ""},
		{"null groups", map[string]any{"groups": nil}, 403, ""},
		{"too many groups", map[string]any{"groups": slices.Repeat([]string{"x"}, 201)}, 403, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idp, err := testauth.NewOIDC()
			if err != nil {
				t.Fatal(err)
			}
			defer idp.Close()
			idp.SetAuthorizationClaims(tc.claims)
			h := httptest.NewUnstartedServer(nil)
			public := "http://" + h.Listener.Addr().String()
			if err := idp.AllowRedirect(public + "/auth/oidc/company/callback"); err != nil {
				t.Fatal(err)
			}
			s := mappingFixture(t, NewMemory())
			p := s.providers["microsoft"].config
			p.ID = "company"
			p.IssuerURL = idp.Server.URL
			p.ClientID = "fixture-client"
			p.secret = "fixture-secret"
			c := Config{PublicURL: public, SessionHours: 12, Providers: []ProviderConfig{p}}
			s, err = New(context.Background(), c, NewMemory(), map[string]bool{"alpha": true, "beta": true})
			if err != nil {
				t.Fatal(err)
			}
			h.Config.Handler = s.Handler()
			h.Start()
			defer h.Close()
			client := newClient()
			if status := completeSSO(t, client, h.URL); status != tc.wantStatus {
				t.Fatal(status, tc.wantStatus)
			}
			u, _ := url.Parse(h.URL)
			foundSession := false
			for _, cookie := range client.Jar.Cookies(u) {
				if cookie.Name == s.cookieName("session") {
					foundSession = true
					a, e := s.SessionActor(context.Background(), cookie.Value)
					if e != nil || a.Grants["alpha"] != tc.want {
						t.Fatal(a, e)
					}
					if tc.wantStatus != 303 {
						t.Fatal("invalid claims created a session")
					}
				}
			}
			if (tc.wantStatus == 303) != foundSession {
				t.Fatal("session issuance did not match authentication outcome")
			}
		})
	}
}
func TestSSOMappingAuditFailureRollsBackLogin(t *testing.T) {
	repo := &auditFailRepository{Repository: NewMemory()}
	s := mappingFixture(t, repo)
	_, _, err := s.login(context.Background(), "microsoft", externalUser{Subject: "alice", Authorization: evidence(nil, []string{"Reliability.Admin"})})
	if err == nil {
		t.Fatal("login succeeded without required mapping audit")
	}
	if err := repo.View(context.Background(), func(tx Tx) error {
		rows, _ := tx.List("user", "global")
		if len(rows) != 0 {
			t.Fatal("failed login persisted user")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type auditFailRepository struct{ Repository }
type auditFailTx struct{ Tx }

func (r *auditFailRepository) Update(ctx context.Context, key string, f func(Tx) error) error {
	return r.Repository.Update(ctx, key, func(tx Tx) error { return f(auditFailTx{tx}) })
}
func (t auditFailTx) Put(kind, scope, id string, b json.RawMessage, expires time.Time) error {
	if kind == "audit" {
		return fmt.Errorf("synthetic audit failure")
	}
	return t.Tx.Put(kind, scope, id, b, expires)
}

func TestSSOMappingRejectsDelayedCallbacksAndBoundsEvidence(t *testing.T) {
	ctx := context.Background()
	s := mappingFixture(t, NewMemory())
	issued := time.Now().Add(-time.Second).Truncate(time.Second)
	fresh := evidence(nil, nil)
	fresh.IssuedAt = issued
	fresh.FlowStarted = time.Now()
	token, _ := mappedLogin(t, s, "alice", fresh)
	stale := evidence(nil, []string{"Reliability.Admin"})
	stale.IssuedAt = issued
	stale.FlowStarted = fresh.FlowStarted.Add(-time.Second)
	if _, _, err := s.login(ctx, "microsoft", externalUser{Subject: "alice", Authorization: stale}); !errors.Is(err, ErrForbidden) {
		t.Fatal("delayed equal-iat callback restored removed owner", err)
	}
	stale.IssuedAt = issued.Add(-time.Second)
	if _, _, err := s.login(ctx, "microsoft", externalUser{Subject: "alice", Authorization: stale}); !errors.Is(err, ErrForbidden) {
		t.Fatal("older token restored removed owner", err)
	}
	a, err := s.SessionActor(ctx, token)
	if err != nil || len(a.Grants) != 0 {
		t.Fatal(a, err)
	}
	old := evidence(nil, []string{"Reliability.Admin"})
	old.IssuedAt = time.Now().Add(-16 * time.Minute)
	if _, _, err = s.login(ctx, "microsoft", externalUser{Subject: "expired", Authorization: old}); !errors.Is(err, ErrForbidden) {
		t.Fatal("stale evidence created fresh grant", err)
	}
	short := evidence(nil, []string{"Reliability.Reader"})
	short.TokenExpires = time.Now().Add(time.Minute)
	_, a = mappedLogin(t, s, "short", short)
	if a.Expires.After(short.TokenExpires) {
		t.Fatal("session exceeded token expiry")
	}
	// Reloading a shorter policy changes both enforcement and the directory deadline.
	p := s.providers["microsoft"]
	p.config.ClaimsMaxAgeMinutes = 1
	s.providers["microsoft"] = p
	deadline := mappingExpiry(p.config, federation{Claims: *fresh, Expires: issued.Add(15 * time.Minute)})
	if deadline.After(issued.Add(time.Minute)) {
		t.Fatal("directory displays stale policy deadline")
	}
}
