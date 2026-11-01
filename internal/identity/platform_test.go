package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/systemone"
)

func platformConfig(t *testing.T) Config {
	t.Helper()
	c, err := ParseConfig([]byte(`publicURL: http://127.0.0.1
providers:
- id: local
  name: Local
  type: local
  bootstrapUsername: admin
  bootstrapPasswordEnv: PLATFORM_TEST_PASSWORD
platformAdmins:
- provider: local
  subject: admin
`), func(string) string { return "platform-fixture-password" })
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func platformFixture(t *testing.T, repo Repository) (*Service, *httptest.Server) {
	t.Helper()
	s, err := New(context.Background(), platformConfig(t), repo, map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigurePlatform([]PlatformTenant{{ID: "alpha", Name: "Alpha", SourceKind: "prometheus", SystemOne: &v1.SystemOneRoute{Provider: "typesafe", Model: "jev-fixture", ContractDigest: "fixture"}}, {ID: "beta", Name: "Beta", SourceKind: "manual"}})
	mux := http.NewServeMux()
	mux.Handle("/auth/", s.Handler())
	mux.Handle("/api/platform/", s.PlatformHandler())
	h := httptest.NewServer(mux)
	t.Cleanup(h.Close)
	return s, h
}
func platformLogin(t *testing.T, h *httptest.Server, username, password string) (*http.Client, string) {
	t.Helper()
	c := newClient()
	info := request(t, c, "GET", h.URL+"/auth/session", "", "", 200)
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	out := request(t, c, "POST", h.URL+"/auth/local/local/login", textField(t, info, "csrfToken"), string(body), 200)
	return c, textField(t, out, "csrfToken")
}
func platformActor(t *testing.T, s *Service, h *httptest.Server, c *http.Client) Actor {
	t.Helper()
	u, _ := url.Parse(h.URL)
	for _, cookie := range c.Jar.Cookies(u) {
		if cookie.Name == s.cookieName("session") {
			a, err := s.SessionActor(context.Background(), cookie.Value)
			if err != nil {
				t.Fatal(err)
			}
			a.platformOperation = true
			a.platformReason = "Fixture administrative change"
			return a
		}
	}
	t.Fatal("missing session")
	return Actor{}
}
func TestPlatformAdminScopeUserManagementAndAudit(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	c, csrf := platformLogin(t, h, "admin", "platform-fixture-password")
	info := request(t, c, "GET", h.URL+"/auth/session", "", "", 200)
	var actor Actor
	if json.Unmarshal(info["actor"], &actor) != nil || !actor.PlatformAdmin {
		t.Fatal("admin marker missing")
	}
	a := platformActor(t, s, h, c)
	if len(a.Grants) != 0 {
		t.Fatal("platform admin gained tenant grants")
	}
	if err := s.EnsureAdministration(context.Background(), nil); err != nil {
		t.Fatal("platform-only bootstrap blocked", err)
	}
	overview := request(t, c, "GET", h.URL+"/api/platform/overview", "", "", 200)
	if !strings.Contains(string(overview["tenants"]), "beta") {
		t.Fatal("global inventory missing")
	}
	request(t, newClient(), "GET", h.URL+"/api/platform/users", "", "", 401)
	created := request(t, c, "POST", h.URL+"/api/platform/users/create", csrf, `{"reason":"Provision initial tenant owner","tenant":"alpha","user":{"provider":"local","username":"alice","name":"Alice","password":"alice-fixture-password","role":"owner","expectedVersion":0}}`, 200)
	id := textField(t, created, "id")
	owner, _ := platformLogin(t, h, "alice", "alice-fixture-password")
	request(t, owner, "GET", h.URL+"/api/platform/overview", "", "", 403)
	request(t, c, "POST", h.URL+"/api/platform/memberships/save", csrf, `{"reason":"Share access with second tenant","tenant":"beta","provider":"local","subject":"alice","role":"viewer","expectedVersion":0}`, 200)
	request(t, c, "POST", h.URL+"/api/platform/users/update", csrf, fmt.Sprintf(`{"reason":"Correct profile","tenant":"alpha","userId":%q,"user":{"name":"Alice Updated","email":"alice@example.test","expectedVersion":1}}`, id), 200)
	request(t, c, "POST", h.URL+"/api/platform/users/update", csrf, fmt.Sprintf(`{"reason":"Stale profile attempt","tenant":"alpha","userId":%q,"user":{"name":"stale","expectedVersion":1}}`, id), 409)
	request(t, c, "POST", h.URL+"/api/platform/memberships/remove", csrf, fmt.Sprintf(`{"reason":"Must preserve last owner","tenant":"alpha","userId":%q,"expectedVersion":1}`, id), 400)
	page := request(t, c, "GET", h.URL+"/api/platform/audit", "", "", 200)
	raw := string(page["items"])
	for _, sensitive := range []string{"alice-fixture-password", "platform-fixture-password", "tokenHash", "localCredentialHash"} {
		if strings.Contains(raw, sensitive) {
			t.Fatal("audit disclosed credentials")
		}
	}
	for _, action := range []string{"user.create", "membership.save", "user.update", "failed"} {
		if !strings.Contains(raw, action) {
			t.Fatal("missing platform audit", action)
		}
	}
	users := request(t, c, "GET", h.URL+"/api/platform/users", "", "", 200)
	if !strings.Contains(string(users["items"]), "Alice Updated") {
		t.Fatal("user directory stale")
	}
	request(t, c, "POST", h.URL+"/api/platform/users/revoke-sessions", csrf, fmt.Sprintf(`{"reason":"Revoke access after incident","userId":%q,"expectedVersion":2}`, id), 200)
	loggedOut := request(t, owner, "GET", h.URL+"/auth/session", "", "", 200)
	if string(loggedOut["authenticated"]) != "false" {
		t.Fatal("revoked browser session remained authenticated")
	}
}
func TestPlatformAdminCSRFIdentityAndRevisionFences(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	c, csrf := platformLogin(t, h, "admin", "platform-fixture-password")
	body := `{"reason":"Pause provider","tenant":"alpha","policy":{"version":0,"enabled":false,"dailyRequestLimit":10}}`
	request(t, c, "POST", h.URL+"/api/platform/system-one/save", "", body, 403)
	request(t, c, "POST", h.URL+"/api/platform/system-one/save", csrf, `{"reason":"","tenant":"alpha","policy":{"version":0,"enabled":false,"dailyRequestLimit":10}}`, 400)
	for _, header := range []string{"Origin", "X-Nines-Tenant", "Authorization"} {
		r, _ := http.NewRequest("GET", h.URL+"/api/platform/overview", nil)
		value := "alpha"
		if header == "Origin" {
			value = "https://other.example"
		}
		if header == "Authorization" {
			value = "Bearer legacy-owner-token"
		}
		r.Header.Set(header, value)
		res, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode < 400 {
			t.Fatal("ambiguous identity/tenant/origin accepted", header)
		}
	}
	request(t, c, "GET", h.URL+"/api/platform/users?before=a&before=b", "", "", 400)
	request(t, c, "GET", h.URL+"/api/platform/users?unexpected=a", "", "", 400)
	request(t, c, "POST", h.URL+"/api/platform/system-one/save", csrf, body, 200)
	request(t, c, "POST", h.URL+"/api/platform/system-one/save", csrf, body, 409)
	request(t, c, "POST", h.URL+"/api/platform/system-one/save", csrf, `{"reason":"Unconfigured provider cannot be enabled","tenant":"beta","policy":{"version":0,"enabled":true,"dailyRequestLimit":10}}`, 400)
	a := platformActor(t, s, h, c)
	forged := a
	forged.sessionHash = ""
	if err := s.repo.View(context.Background(), func(tx Tx) error { return s.platformAdminTx(tx, forged) }); err == nil {
		t.Fatal("forged admin marker bypassed session proof")
	}
	s.config.PlatformAdmins = nil
	request(t, c, "GET", h.URL+"/api/platform/overview", "", "", 403)
}
func TestPlatformAdminSelfResetAndRevocationRemainAudited(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	c, csrf := platformLogin(t, h, "admin", "platform-fixture-password")
	a := platformActor(t, s, h, c)
	request(t, c, "POST", h.URL+"/api/platform/users/update", csrf, fmt.Sprintf(`{"reason":"Rotate administrator password","tenant":"alpha","userId":%q,"user":{"name":"Administrator","password":"rotated-admin-password","expectedVersion":1}}`, a.ID), 200)
	info := request(t, c, "GET", h.URL+"/auth/session", "", "", 200)
	if string(info["authenticated"]) != "false" {
		t.Fatal("reset did not revoke admin session")
	}
	c, csrf = platformLogin(t, h, "admin", "rotated-admin-password")
	request(t, c, "POST", h.URL+"/api/platform/users/revoke-sessions", csrf, fmt.Sprintf(`{"reason":"Revoke all my sessions","userId":%q,"expectedVersion":2}`, a.ID), 200)
	request(t, c, "GET", h.URL+"/api/platform/overview", "", "", 403)
	c, _ = platformLogin(t, h, "admin", "rotated-admin-password")
	page := request(t, c, "GET", h.URL+"/api/platform/audit", "", "", 200)
	if !strings.Contains(string(page["items"]), "sessions.revoke") {
		t.Fatal("self revocation missing audit")
	}
}

type failPlatformAudit struct{ Repository }
type failingPlatformTx struct{ Tx }

func (t failingPlatformTx) Put(kind, scope, id string, b json.RawMessage, expires time.Time) error {
	if kind == "platform_audit" {
		return errors.New("fixture audit unavailable")
	}
	return t.Tx.Put(kind, scope, id, b, expires)
}
func (r failPlatformAudit) Update(ctx context.Context, key string, f func(Tx) error) error {
	return r.Repository.Update(ctx, key, func(tx Tx) error { return f(failingPlatformTx{tx}) })
}
func TestPlatformAuditFailureRollsBackUserCreation(t *testing.T) {
	repo := NewMemory()
	s, h := platformFixture(t, repo)
	c, _ := platformLogin(t, h, "admin", "platform-fixture-password")
	a := platformActor(t, s, h, c)
	s.repo = failPlatformAudit{repo}
	_, err := s.CreateLocalUser(context.Background(), a, "alpha", LocalUserInput{Provider: "local", Username: "new-user", Name: "New", Password: "fixture-new-user-password", Role: "viewer"})
	if err == nil {
		t.Fatal("mutation committed without audit")
	}
	id := s.user("local", externalUser{Subject: "new-user"}).ID
	err = repo.View(context.Background(), func(tx Tx) error { _, e := tx.Get("user", "global", id); return e })
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("user write survived failed audit", err)
	}
}

type systemOneFixture struct{ calls atomic.Int32 }

func (f *systemOneFixture) Descriptor() v1.SystemOneRoute {
	return v1.SystemOneRoute{Provider: "typesafe", Model: "jev-fixture"}
}
func (f *systemOneFixture) Assess(context.Context, systemone.Input) (v1.SystemOneAssessment, error) {
	f.calls.Add(1)
	return v1.SystemOneAssessment{Status: "assessed"}, nil
}
func (f *systemOneFixture) ContainsCredential(b []byte) bool {
	return strings.Contains(string(b), "fixture-model-key")
}
func TestPlatformJevPolicyAtomicQuotaPauseAndUTCRollover(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	c, _ := platformLogin(t, h, "admin", "platform-fixture-password")
	a := platformActor(t, s, h, c)
	ctx := context.Background()
	if err := s.savePlatformAI(ctx, a, "alpha", PlatformAIPolicy{Enabled: true, DailyRequestLimit: 3}); err != nil {
		t.Fatal(err)
	}
	inner := &systemOneFixture{}
	wrapper := s.ControlSystemOne("alpha", inner)
	if !wrapper.(interface{ ContainsCredential([]byte) bool }).ContainsCredential([]byte("fixture-model-key")) {
		t.Fatal("quota wrapper lost credential guard")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = wrapper.Assess(ctx, systemone.Input{Description: "synthetic"}) }()
	}
	wg.Wait()
	if inner.calls.Load() != 3 {
		t.Fatal("concurrent quota bypass", inner.calls.Load())
	}
	if err := s.savePlatformAI(ctx, a, "alpha", PlatformAIPolicy{Version: 1, Enabled: false, DailyRequestLimit: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapper.Assess(ctx, systemone.Input{Description: "synthetic"}); err == nil || inner.calls.Load() != 3 {
		t.Fatal("paused provider called")
	}
	if err := s.savePlatformAI(ctx, a, "alpha", PlatformAIPolicy{Version: 2, Enabled: true, DailyRequestLimit: 3}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.Now = func() time.Time { return now.Add(24 * time.Hour) }
	// Session expiry is independent of the daily quota. Start a fresh admin session.
	_, err := wrapper.Assess(ctx, systemone.Input{Description: "synthetic"})
	if err != nil || inner.calls.Load() != 4 {
		t.Fatal("new UTC day failed to reset quota", err)
	}
}
func TestPlatformConfigurationRejectsAmbiguousAdmins(t *testing.T) {
	c := platformConfig(t)
	for _, admins := range [][]PlatformAdmin{{{Provider: "missing", Subject: "admin"}}, {{Provider: "local", Subject: ""}}, {{Provider: "local", Subject: "admin"}, {Provider: "local", Subject: "admin"}}} {
		c.PlatformAdmins = admins
		if validatePlatformAdmins(c) == nil {
			t.Fatal("invalid platform allowlist accepted")
		}
	}
}
func TestPostgresPlatformAdminPersistence(t *testing.T) {
	db := os.Getenv("TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set TEST_DATABASE_URL for platform admin persistence")
	}
	ctx := context.Background()
	base := fmt.Sprintf("platform_fixture_%d", time.Now().UnixNano())
	u, err := url.Parse(db)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", base)
	u.RawQuery = q.Encode()
	repo, err := NewPostgres(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(base))
	schema := "waypoint_identity_" + hex.EncodeToString(sum[:6])
	t.Cleanup(func() {
		conn, e := pgx.Connect(ctx, db)
		if e != nil {
			t.Error(e)
			return
		}
		defer func() { _ = conn.Close(ctx) }()
		if _, e = conn.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); e != nil {
			t.Error(e)
		}
	})
	s, h := platformFixture(t, repo)
	c, _ := platformLogin(t, h, "admin", "platform-fixture-password")
	a := platformActor(t, s, h, c)
	if err = s.savePlatformAI(ctx, a, "alpha", PlatformAIPolicy{Enabled: false, DailyRequestLimit: 25}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	reopened, err := NewPostgres(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := New(ctx, platformConfig(t), reopened, map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		t.Fatal(err)
	}
	after.ConfigurePlatform([]PlatformTenant{{ID: "alpha"}})
	result, err := after.platformAI(ctx, a, "alpha")
	if err != nil {
		t.Fatal("session did not survive restart", err)
	}
	body, _ := json.Marshal(result)
	if !strings.Contains(string(body), `"enabled":false`) || !strings.Contains(string(body), `"dailyRequestLimit":25`) {
		t.Fatal("policy not persisted")
	}
	var events []json.RawMessage
	if err = reopened.View(ctx, func(tx Tx) error { var e error; events, e = tx.Page("platform_audit", "global", "", 100); return e }); err != nil || len(events) != 2 {
		t.Fatal("audit not persisted", err, len(events))
	}
}

func TestPlatformSSOIdentityIsIndependentFromTenantOwnerClaims(t *testing.T) {
	s := mappingFixture(t, NewMemory())
	s.config.PlatformAdmins = []PlatformAdmin{{Provider: "microsoft", Subject: "platform-only"}}
	_, admin := mappedLogin(t, s, "platform-only", evidence(nil, nil))
	if !admin.PlatformAdmin || len(admin.Grants) != 0 {
		t.Fatal("platform SSO identity gained tenant grants", admin)
	}
	if err := s.repo.View(context.Background(), func(tx Tx) error { return s.platformAdminTx(tx, admin) }); err != nil {
		t.Fatal(err)
	}
	_, owner := mappedLogin(t, s, "tenant-admin", evidence(nil, []string{"Reliability.Admin"}))
	if owner.PlatformAdmin || owner.Grants["alpha"] != "owner" {
		t.Fatal("tenant owner claims became platform privileges")
	}
	if err := s.repo.View(context.Background(), func(tx Tx) error { return s.platformAdminTx(tx, owner) }); !errors.Is(err, ErrForbidden) {
		t.Fatal("SSO tenant owner entered platform", err)
	}
	s.config.PlatformAdmins = []PlatformAdmin{{Provider: "different", Subject: "platform-only"}}
	if err := s.repo.View(context.Background(), func(tx Tx) error { return s.platformAdminTx(tx, admin) }); !errors.Is(err, ErrForbidden) {
		t.Fatal("platform access ignored provider binding", err)
	}
}
func TestPlatformRejectsScopedMachineCredentialsAndSecretReasons(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	c, csrf := platformLogin(t, h, "admin", "platform-fixture-password")
	a := platformActor(t, s, h, c)
	ctx := context.Background()
	if _, err := s.Grant(ctx, a, "alpha", "local", "admin", "owner", 0); err != nil {
		t.Fatal(err)
	}
	cliToken := "wp_cli_" + randomToken()
	if err := s.repo.Update(ctx, "fixture-cli", func(tx Tx) error {
		return put(tx, "cli_token", "global", hash(cliToken), cliGrant{SessionHash: a.sessionHash, Tenant: "alpha", Expires: s.Now().Add(time.Hour)}, s.Now().Add(time.Hour))
	}); err != nil {
		t.Fatal(err)
	}
	_, serviceToken, err := s.CreateAccount(ctx, a, "alpha", "platform-test", "Scoped agent", "agent", s.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{cliToken, serviceToken} {
		r, _ := http.NewRequest("GET", h.URL+"/api/platform/users", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		res, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatal("scoped credential entered platform", res.StatusCode)
		}
	}
	guard := &systemOneFixture{}
	s.ConfigurePlatform([]PlatformTenant{{ID: "alpha", SystemOne: &v1.SystemOneRoute{Model: "jev-fixture"}}}, guard.ContainsCredential)
	for _, reason := range []string{"platform-fixture-password", "fixture-model-key"} {
		body, _ := json.Marshal(map[string]any{"tenant": "alpha", "reason": reason, "policy": PlatformAIPolicy{Enabled: false, DailyRequestLimit: 10}})
		request(t, c, "POST", h.URL+"/api/platform/system-one/save", csrf, string(body), 400)
	}
}

func (f *systemOneFixture) VisualEnabled() bool                 { return true }
func (f *systemOneFixture) VisualDescriptor() v1.SystemOneRoute { return f.Descriptor() }
func (f *systemOneFixture) AssessVisual(context.Context, systemone.VisualInput) (v1.VisualAssessment, error) {
	f.calls.Add(1)
	return v1.VisualAssessment{Status: "assessed"}, nil
}
func TestVisualRequestsShareAdminQuotaAndPause(t *testing.T) {
	s, h := platformFixture(t, NewMemory())
	cookie, _ := platformLogin(t, h, "admin", "platform-fixture-password")
	actor := platformActor(t, s, h, cookie)
	ctx := context.Background()
	if e := s.savePlatformAI(ctx, actor, "alpha", PlatformAIPolicy{Enabled: true, DailyRequestLimit: 1}); e != nil {
		t.Fatal(e)
	}
	inner := &systemOneFixture{}
	wrapped := s.ControlSystemOne("alpha", inner)
	visual := wrapped.(systemone.VisualAssessor)
	if !visual.VisualEnabled() {
		t.Fatal("visual capability lost")
	}
	if _, e := visual.AssessVisual(ctx, systemone.VisualInput{}); e != nil {
		t.Fatal(e)
	}
	if _, e := wrapped.Assess(ctx, systemone.Input{Description: "synthetic"}); !errors.Is(e, systemone.ErrBudgetExhausted) {
		t.Fatal("separate text/image quota", e)
	}
	if e := s.savePlatformAI(ctx, actor, "alpha", PlatformAIPolicy{Version: 1, Enabled: false, DailyRequestLimit: 10}); e != nil {
		t.Fatal(e)
	}
	if _, e := visual.AssessVisual(ctx, systemone.VisualInput{}); !errors.Is(e, systemone.ErrPaused) || inner.calls.Load() != 1 {
		t.Fatal("pause bypassed", e, inner.calls.Load())
	}
}
