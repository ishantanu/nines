package identity_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testauth"
)

type harness struct {
	t        *testing.T
	server   *httptest.Server
	config   identity.Config
	repo     identity.Repository
	database string
	active   atomic.Pointer[http.Handler]
	offset   atomic.Int64
}

func requireFixture(t *testing.T) {
	t.Helper()
	if os.Getenv("NINES_IDENTITY_ACCEPTANCE") != "1" {
		t.Skip("use make test-identity for disposable providers")
	}
	for _, key := range []string{"NINES_IDENTITY_DATABASE_URL", "NINES_IDENTITY_FIXTURE_DIR", "NINES_IDENTITY_CLI", "NINES_IDENTITY_LDAP_URL", "NINES_IDENTITY_STARTTLS_URL"} {
		if os.Getenv(key) == "" {
			t.Fatal("identity fixture missing", key)
		}
	}
	u, err := url.Parse(os.Getenv("NINES_IDENTITY_DATABASE_URL"))
	if err != nil || u.Hostname() != "127.0.0.1" || u.User == nil || u.User.Username() != "nines_identity" || u.Path != "/nines_identity" {
		t.Fatal("identity acceptance requires its disposable loopback database")
	}
}
func opaque(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal("fixture randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func newHarness(t *testing.T, config func(string) identity.Config) *harness {
	t.Helper()
	requireFixture(t)
	h := &harness{t: t, server: httptest.NewUnstartedServer(nil)}
	h.config = config("http://" + h.server.Listener.Addr().String())
	// URL is set only by Start; configuration must use the reserved listener.
	h.config.PublicURL = "http://" + h.server.Listener.Addr().String()
	u, _ := url.Parse(os.Getenv("NINES_IDENTITY_DATABASE_URL"))
	q := u.Query()
	q.Set("search_path", "acceptance_"+strings.ReplaceAll(opaque(t), "-", "_"))
	u.RawQuery = q.Encode()
	h.database = u.String()
	h.start()
	h.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := h.active.Load()
		if handler == nil {
			http.Error(w, "Restarting", http.StatusServiceUnavailable)
			return
		}
		(*handler).ServeHTTP(w, r)
	})
	h.server.Start()
	t.Cleanup(func() { h.server.Close(); h.repo.Close() })
	return h
}
func (h *harness) start() {
	repo, err := identity.NewPostgres(context.Background(), h.database)
	if err != nil {
		h.t.Fatal("disposable identity database unavailable")
	}
	h.repo = repo
	auth, err := identity.New(context.Background(), h.config, repo, map[string]bool{"alpha": true, "beta": true})
	if err != nil {
		h.t.Fatal("fixture identity initialization failed", err)
	}
	auth.Now = func() time.Time { return time.Now().Add(time.Duration(h.offset.Load())) }
	handler, err := httpapi.NewMulti([]httpapi.Workspace{
		{ID: "alpha", Name: "Alpha acceptance", Service: service.New(store.NewMemory(), nil)},
		{ID: "beta", Name: "Beta acceptance", Service: service.New(store.NewMemory(), nil)},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), auth)
	if err != nil {
		h.t.Fatal("fixture HTTP initialization failed", err)
	}
	h.active.Store(&handler)
}
func (h *harness) restart() { h.active.Store(nil); h.repo.Close(); h.start() }
func browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (h *harness) call(c *http.Client, tenant, token, csrf, method, path, body string, want int) []byte {
	h.t.Helper()
	req, err := http.NewRequest(method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	if tenant != "" {
		req.Header.Set("X-Nines-Tenant", tenant)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if path == "/api/resources/apply" {
		req.Header.Set("Content-Type", "application/yaml")
		req.Header.Set("If-Match", `"0"`)
	}
	response, err := c.Do(req)
	if err != nil {
		h.t.Fatal("fixture HTTP request failed")
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		h.t.Fatal("fixture response unreadable")
	}
	if response.StatusCode != want {
		h.t.Fatalf("%s %s tenant=%s: status %d, expected %d", method, strings.Split(path, "?")[0], tenant, response.StatusCode, want)
	}
	return data
}
func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var out T
	if json.Unmarshal(b, &out) != nil {
		t.Fatal("invalid fixture JSON")
	}
	return out
}
func (h *harness) session(c *http.Client) (identity.Actor, string) {
	info := decode[struct {
		Actor identity.Actor `json:"actor"`
		CSRF  string         `json:"csrfToken"`
	}](h.t, h.call(c, "", "", "", "GET", "/auth/session", "", 200))
	return info.Actor, info.CSRF
}
func (h *harness) oidc(idp *testauth.OIDC, subject string, claims map[string]any) (*http.Client, string) {
	h.t.Helper()
	idp.SetAuthorizationClaims(claims)
	c := browser()
	response, err := c.Get(h.server.URL + "/auth/oidc/company/login")
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 302 {
		h.t.Fatal("OIDC start rejected")
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(location.String(), idp.Server.URL+"/authorize?") {
		h.t.Fatal("OIDC fixture redirect escaped issuer")
	}
	query := location.Query()
	query.Set("login_as", subject)
	location.RawQuery = query.Encode()
	response, err = c.Get(location.String())
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	_ = response.Body.Close()
	callback := response.Header.Get("Location")
	if !strings.HasPrefix(callback, h.server.URL+"/auth/oidc/company/callback?") {
		h.t.Fatal("OIDC callback escaped fixture")
	}
	response, err = c.Get(callback)
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 303 {
		h.t.Fatal("signed OIDC callback rejected", response.StatusCode)
	}
	_, csrf := h.session(c)
	return c, csrf
}
func (h *harness) cliGrant(c *http.Client, csrf, tenant, decision string) string {
	h.t.Helper()
	verifier := opaque(h.t)
	digest := sha256.Sum256([]byte(verifier))
	body, _ := json.Marshal(map[string]string{"tenant": tenant, "redirectURI": "http://127.0.0.1:54321/callback", "state": opaque(h.t), "challenge": base64.RawURLEncoding.EncodeToString(digest[:])})
	flow := decode[struct {
		URL string `json:"authorizationURL"`
	}](h.t, h.call(c, "", "", "", "POST", "/auth/cli/start", string(body), 200))
	callback := h.approve(c, csrf, flow.URL, tenant, decision)
	if decision != "approve" {
		if callback.Query().Get("code") != "" || callback.Query().Get("error") != "access_denied" {
			h.t.Fatal("cancelled flow issued a code")
		}
		h.call(c, "", "", "", "GET", strings.TrimPrefix(flow.URL, h.server.URL), "", 404)
		return ""
	}
	body, _ = json.Marshal(map[string]string{"code": callback.Query().Get("code"), "codeVerifier": verifier, "redirectURI": "http://127.0.0.1:54321/callback"})
	credential := decode[struct {
		Token string `json:"token"`
	}](h.t, h.call(browser(), "", "", "", "POST", "/auth/cli/exchange", string(body), 200))
	h.call(browser(), "", "", "", "POST", "/auth/cli/exchange", string(body), 404)
	if !strings.HasPrefix(credential.Token, "wp_cli_") {
		h.t.Fatal("missing CLI credential")
	}
	return credential.Token
}
func (h *harness) approve(c *http.Client, csrf, authorization, tenant, decision string) *url.URL {
	h.t.Helper()
	u, err := url.Parse(authorization)
	if err != nil || u.Scheme+"://"+u.Host != h.server.URL || u.Path != "/auth/cli/authorize" {
		h.t.Fatal("invalid fixture CLI URL")
	}
	response, err := c.Get(authorization)
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	page, _ := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	_ = response.Body.Close()
	actor, _ := h.session(c)
	tenants := decode[[]struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}](h.t, h.call(c, "", "", "", "GET", "/api/tenants", "", 200))
	role := ""
	for _, entry := range tenants {
		if entry.ID == tenant {
			role = entry.Role
		}
	}
	if actor.ID == "" || role == "" {
		h.t.Fatal("CLI approval has no authenticated tenant role")
	}
	for _, text := range []string{h.server.URL, "<strong>" + tenant + "</strong>", "<strong>" + html.EscapeString(actor.Name) + "</strong>", "<strong>" + role + "</strong>", "Authorize CLI", "Return address:"} {
		if !bytes.Contains(page, []byte(text)) {
			h.t.Fatal("CLI approval omitted server/tenant/identity boundary")
		}
	}
	marker := "Return address: <code>"
	start := strings.Index(string(page), marker)
	if start < 0 {
		h.t.Fatal("CLI approval omitted exact return address")
	}
	address := string(page)[start+len(marker):]
	end := strings.Index(address, "</code>")
	if end < 0 {
		h.t.Fatal("CLI approval has invalid return address")
	}
	address = html.UnescapeString(address[:end])
	response, err = c.PostForm(h.server.URL+"/auth/cli/approve", url.Values{"request": {u.Query().Get("request")}, "csrf": {csrf}, "decision": {decision}})
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 303 {
		h.t.Fatal("CLI decision rejected", response.StatusCode)
	}
	callback, err := url.Parse(response.Header.Get("Location"))
	if err != nil || callback.Scheme != "http" || callback.Hostname() != "127.0.0.1" || callback.Path != "/callback" {
		h.t.Fatal("CLI callback escaped loopback")
	}
	if callback.Scheme+"://"+callback.Host+callback.Path != address {
		h.t.Fatal("CLI returned to a different address than approved")
	}
	return callback
}
func cliEnvironment() []string {
	out := []string{}
	for _, key := range []string{"PATH", "SYSTEMROOT", "TMPDIR"} {
		if value := os.Getenv(key); value != "" {
			out = append(out, key+"="+value)
		}
	}
	return out
}
func (h *harness) nativeLogin(c *http.Client, csrf, tenant, decision string) string {
	h.t.Helper()
	config := filepath.Join(h.t.TempDir(), "config.yaml")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Getenv("NINES_IDENTITY_CLI"), "login", "--method", "sso", "--no-browser", "--server", h.server.URL, "--tenant", tenant, "--context", "acceptance", "--config", config)
	cmd.Env = cliEnvironment()
	var output bytes.Buffer
	cmd.Stdout = &output
	stderr, err := cmd.StderrPipe()
	if err != nil {
		h.t.Fatal("identity acceptance operation failed")
	}
	if err = cmd.Start(); err != nil {
		h.t.Fatal("native CLI failed to start")
	}
	completed := false
	defer func() {
		if !completed {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	scanner := bufio.NewScanner(stderr)
	authorization := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, h.server.URL+"/auth/cli/authorize?") {
			authorization = line
			break
		}
	}
	if authorization == "" {
		h.t.Fatal("native CLI omitted authorization URL")
	}
	callback := h.approve(c, csrf, authorization, tenant, decision)
	response, err := c.Get(callback.String())
	if err != nil {
		h.t.Fatal("native CLI callback failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		h.t.Fatal("native callback rejected")
	}
	_, _ = io.Copy(io.Discard, stderr)
	err = cmd.Wait()
	completed = true
	if decision != "approve" {
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 3 {
			h.t.Fatal("cancelled CLI login did not return unauthenticated")
		}
		if _, err = os.Stat(config + ".credentials.json"); !os.IsNotExist(err) {
			h.t.Fatal("cancelled CLI login saved credentials")
		}
		return config
	}
	if err != nil {
		h.t.Fatal("native CLI did not complete login")
	}
	if bytes.Contains(output.Bytes(), []byte("wp_cli_")) {
		h.t.Fatal("native CLI printed credentials")
	}
	for _, path := range []string{config, config + ".credentials.json"} {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != 0600 {
			h.t.Fatal("CLI did not use private credential files")
		}
	}
	data, readErr := os.ReadFile(config + ".credentials.json")
	if readErr != nil {
		h.t.Fatal("CLI credential readback unavailable")
	}
	saved := decode[map[string]struct {
		Server  string    `json:"server"`
		Tenant  string    `json:"tenant"`
		Token   string    `json:"token"`
		Expires time.Time `json:"expiresAt"`
	}](h.t, data)
	credential, ok := saved["acceptance"]
	actor, _ := h.session(c)
	if !ok || credential.Server != h.server.URL || credential.Tenant != tenant || !strings.HasPrefix(credential.Token, "wp_cli_") || !credential.Expires.After(time.Now()) || credential.Expires.After(actor.Expires) {
		h.t.Fatal("saved CLI credential escaped approved scope or session expiry")
	}
	return config
}
func (h *harness) nativeStatus(config, tenant string, want int, role string) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Getenv("NINES_IDENTITY_CLI"), "auth", "status", "--config", config, "--context", "acceptance", "--tenant", tenant, "--agent")
	cmd.Env = cliEnvironment()
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			h.t.Fatal("native CLI failed")
		}
		code = exit.ExitCode()
	}
	if code != want {
		h.t.Fatalf("CLI status exit %d, expected %d", code, want)
	}
	if bytes.Contains(output, []byte("wp_cli_")) {
		h.t.Fatal("CLI diagnostic printed credentials")
	}
	if want == 0 {
		result := decode[map[string]any](h.t, output)
		if result["tenant"] != tenant || result["role"] != role {
			h.t.Fatal("CLI returned incorrect tenant or role")
		}
	}
}
func fixtureSecret(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(os.Getenv("NINES_IDENTITY_FIXTURE_DIR"), name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("fixture secret unavailable")
	}
	return strings.TrimSpace(string(b))
}
func parseConfig(t *testing.T, body string, lookup func(string) string) identity.Config {
	t.Helper()
	config, err := identity.ParseConfig([]byte(body), lookup)
	if err != nil {
		t.Fatal("invalid acceptance config", err)
	}
	return config
}
func sloBody(name string) string {
	return fmt.Sprintf(`{"apiVersion":"nines.dev/v1alpha1","kind":"ServiceLevelObjective","metadata":{"name":%q,"project":"production"},"spec":{"displayName":"API availability","service":"api","target":0.999,"windowSeconds":3600,"stepSeconds":60}}`, name)
}
