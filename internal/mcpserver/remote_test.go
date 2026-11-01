package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

type remoteFixture struct {
	handler             *RemoteHandler
	server, issuer, api *httptest.Server
	mu                  sync.Mutex
	grants              map[string]map[string]any
	introspections      atomic.Int64
	apiCalls            atomic.Int64
	block               chan struct{}
	entered             chan struct{}
	cancelled           chan struct{}
}

func newRemoteFixture(t *testing.T, writes bool) *remoteFixture {
	t.Helper()
	f := &remoteFixture{grants: map[string]map[string]any{}, entered: make(chan struct{}, 20), cancelled: make(chan struct{}, 20)}
	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.apiCalls.Add(1)
		token := r.Header.Get("Authorization")
		if r.Header.Get("X-Nines-Tenant") != "alpha" || (token != "Bearer api-alice" && token != "Bearer api-bob") {
			t.Error("credential or tenant escaped binding")
		}
		if f.block != nil {
			f.entered <- struct{}{}
			select {
			case <-r.Context().Done():
				f.cancelled <- struct{}{}
				return
			case <-f.block:
			}
		}
		if r.Method != "GET" {
			w.WriteHeader(403)
			_, _ = fmt.Fprint(w, `{"error":"RBAC denied"}`)
			return
		}
		user := "alice"
		if token == "Bearer api-bob" {
			user = "bob"
		}
		remoteJSON(w, 200, map[string]any{"tenant": map[string]string{"id": "alpha"}, "role": "viewer", "user": user})
	}))
	t.Cleanup(f.api.Close)
	f.server = httptest.NewUnstartedServer(nil)
	publicURL := "http://" + f.server.Listener.Addr().String() + "/mcp"
	codes := map[string]url.Values{}
	var codeID int
	f.issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			remoteJSON(w, 200, map[string]any{"issuer": f.issuer.URL, "authorization_endpoint": f.issuer.URL + "/authorize", "token_endpoint": f.issuer.URL + "/token", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "authorization_response_iss_parameter_supported": true, "scopes_supported": []string{RemoteReadScope}})
		case "/authorize":
			q := r.URL.Query()
			if q.Get("client_id") != "managed" || q.Get("resource") != publicURL || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
				t.Error("OAuth resource or PKCE missing")
			}
			f.mu.Lock()
			codeID++
			code := fmt.Sprintf("synthetic-code-%d", codeID)
			codes[code] = q
			f.mu.Unlock()
			callback, _ := url.Parse(q.Get("redirect_uri"))
			callback.RawQuery = url.Values{"code": {code}, "state": {q.Get("state")}, "iss": {f.issuer.URL}}.Encode()
			http.Redirect(w, r, callback.String(), http.StatusSeeOther)
		case "/token":
			if e := r.ParseForm(); e != nil {
				t.Error(e)
			}
			q := r.PostForm
			f.mu.Lock()
			code, ok := codes[q.Get("code")]
			delete(codes, q.Get("code"))
			f.mu.Unlock()
			sum := sha256.Sum256([]byte(q.Get("code_verifier")))
			if !ok || q.Get("resource") != publicURL || q.Get("redirect_uri") != code.Get("redirect_uri") || base64.RawURLEncoding.EncodeToString(sum[:]) != code.Get("code_challenge") {
				remoteJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			remoteJSON(w, 200, map[string]any{"access_token": "alice-read", "token_type": "Bearer", "expires_in": 300, "scope": RemoteReadScope})
		case "/introspect":
			f.introspections.Add(1)
			id, secret, ok := r.BasicAuth()
			if !ok || id != "resource" || secret != "introspection-secret" {
				t.Error("introspection service credential missing")
			}
			if e := r.ParseForm(); e != nil {
				t.Error(e)
			}
			f.mu.Lock()
			grant := f.grants[r.PostForm.Get("token")]
			data, _ := json.Marshal(grant)
			f.mu.Unlock()
			if grant == nil {
				remoteJSON(w, 200, map[string]bool{"active": false})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.issuer.Close)
	for _, user := range []string{"alice", "bob"} {
		f.grants[user+"-read"] = map[string]any{"active": true, "sub": user, "client_id": "managed", "iss": f.issuer.URL, "aud": publicURL, "scope": RemoteReadScope, "exp": time.Now().Add(10 * time.Minute).Unix()}
	}
	config := RemoteConfig{APIVersion: "nines.dev/v1alpha1", Kind: "RemoteMCP", PublicURL: publicURL, Server: f.api.URL, Tenant: "alpha", Issuer: f.issuer.URL, IntrospectionURL: f.issuer.URL + "/introspect", IntrospectionClientID: "resource", secret: "introspection-secret", AllowWrites: writes, AllowRuleDeployment: writes, AllowRuleWithdrawal: writes, Bindings: []RemoteBinding{{Subject: "alice", ClientID: "managed", token: "api-alice"}, {Subject: "bob", ClientID: "managed", token: "api-bob"}}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var e error
	f.handler, e = NewRemote(config)
	if e != nil {
		t.Fatal(e)
	}
	f.server.Config.Handler = f.handler
	f.server.Start()
	t.Cleanup(f.server.Close)
	return f
}

type remoteBearer string

func (b remoteBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}
func (f *remoteFixture) connect(t *testing.T, token string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "remote-acceptance", Version: "1"}, nil)
	session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: f.server.URL + "/mcp", HTTPClient: &http.Client{Transport: remoteBearer(token)}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}
func (f *remoteFixture) post(t *testing.T, token string, headers map[string]string, body io.Reader) *http.Response {
	t.Helper()
	if body == nil {
		body = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	}
	req, e := http.NewRequest("POST", f.server.URL+"/mcp", body)
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	res, e := f.server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}
func TestRemoteOAuthDiscoveryPKCEAndSDKClient(t *testing.T) {
	f := newRemoteFixture(t, false)
	unauth := f.post(t, "", nil, nil)
	if unauth.StatusCode != 401 || !strings.Contains(unauth.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatal("missing OAuth discovery challenge")
	}
	handler, e := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{RedirectURL: "http://127.0.0.1:54321/callback", PreregisteredClient: &oauthex.ClientCredentials{ClientID: "managed"}, AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, e := http.NewRequestWithContext(ctx, "GET", args.URL, nil)
		if e != nil {
			return nil, e
		}
		res, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer func() { _ = res.Body.Close() }()
		u, e := res.Location()
		if e != nil {
			return nil, e
		}
		return &auth.AuthorizationResult{Code: u.Query().Get("code"), State: u.Query().Get("state"), Iss: u.Query().Get("iss")}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "managed-oauth", Version: "1"}, nil)
	session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: f.server.URL + "/mcp", OAuthHandler: handler}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = session.Close() }()
	var out struct{ User string }
	dataInto(t, call(t, ctx, session, "workspace", map[string]any{}), &out)
	if out.User != "alice" {
		t.Fatal("OAuth user did not select its own Nines binding")
	}
	tools, e := session.ListTools(ctx, nil)
	if e != nil || hasTool(tools.Tools, "apply_slo") || !hasTool(tools.Tools, "native_slo_results") {
		t.Fatal("default remote tool surface differs", e)
	}
	if f.introspections.Load() < 2 || f.apiCalls.Load() != 1 {
		t.Fatal("grants cached or extra API calls")
	}
	// Replay of an already consumed AS authorization code is rejected by the fixture AS.
	res, e := http.PostForm(f.issuer.URL+"/token", url.Values{"code": {"synthetic-code-1"}, "resource": {f.handler.config.PublicURL}})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 400 {
		t.Fatal("authorization code replay accepted")
	}
}
func TestRemoteIdentitiesRevocationAndReconnection(t *testing.T) {
	f := newRemoteFixture(t, false)
	alice, ctx := f.connect(t, "alice-read")
	bob, bobctx := f.connect(t, "bob-read")
	for _, tc := range []struct {
		s    *mcp.ClientSession
		ctx  context.Context
		want string
	}{{alice, ctx, "alice"}, {bob, bobctx, "bob"}} {
		var out struct{ User string }
		dataInto(t, call(t, tc.ctx, tc.s, "workspace", map[string]any{}), &out)
		if out.User != tc.want {
			t.Fatal("remote identity shared", out)
		}
	}
	f.mu.Lock()
	f.grants["alice-read"]["active"] = false
	f.mu.Unlock()
	before := f.apiCalls.Load()
	rejected(t, ctx, alice, "workspace", map[string]any{})
	if f.apiCalls.Load() != before {
		t.Fatal("revoked grant reached Nines")
	}
	f.mu.Lock()
	f.grants["alice-read"]["active"] = true
	f.mu.Unlock()
	reconnected, rectx := f.connect(t, "alice-read")
	call(t, rectx, reconnected, "workspace", map[string]any{})
}
func TestRemoteRejectsInvalidGrantsOriginsAndReplay(t *testing.T) {
	for _, change := range []string{"issuer", "audience", "expired", "future", "scope", "subject", "client", "inactive", "long-lived"} {
		t.Run(change, func(t *testing.T) {
			f := newRemoteFixture(t, false)
			f.mu.Lock()
			g := f.grants["alice-read"]
			switch change {
			case "issuer":
				g["iss"] = "https://other.example"
			case "audience":
				g["aud"] = []string{"https://other.example/mcp"}
			case "expired":
				g["exp"] = time.Now().Add(-time.Minute).Unix()
			case "future":
				g["nbf"] = time.Now().Add(time.Minute).Unix()
			case "scope":
				g["scope"] = "nines:write"
			case "subject":
				g["sub"] = "mallory"
			case "client":
				g["client_id"] = "other-client"
			case "inactive":
				g["active"] = false
			case "long-lived":
				g["exp"] = time.Now().Add(24 * time.Hour).Unix()
			}
			f.mu.Unlock()
			res := f.post(t, "alice-read", nil, nil)
			if res.StatusCode != 401 && res.StatusCode != 403 {
				t.Fatal(change, res.StatusCode)
			}
			if f.apiCalls.Load() != 0 {
				t.Fatal("invalid grant reached API")
			}
		})
	}
	f := newRemoteFixture(t, false)
	for _, headers := range []map[string]string{{"Origin": "https://evil.example"}, {"Origin": "null"}, {"Host": "evil.example"}, {"Mcp-Session-Id": "stolen-session"}, {"Last-Event-ID": "stolen-event"}} {
		res := f.post(t, "alice-read", headers, nil)
		if res.StatusCode != 400 && res.StatusCode != 403 {
			t.Fatal("unsafe HTTP metadata accepted", headers, res.StatusCode)
		}
	}
	if f.introspections.Load() != 0 {
		t.Fatal("invalid origins/session replay spent introspection calls")
	}
	res := f.post(t, "alice-read", nil, strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	if res.StatusCode != 413 {
		t.Fatal("oversized request accepted", res.StatusCode)
	}
}
func TestRemoteOAuthScopesRequireOperatorOptInAndAPIRBAC(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		f := newRemoteFixture(t, enabled)
		f.mu.Lock()
		f.grants["alice-read"]["scope"] = "nines:read nines:write nines:deploy nines:withdraw"
		f.mu.Unlock()
		session, ctx := f.connect(t, "alice-read")
		tools, e := session.ListTools(ctx, nil)
		if e != nil || hasTool(tools.Tools, "apply_slo") != enabled || hasTool(tools.Tools, "deploy_slo_rules") != enabled {
			t.Fatal("scope bypassed operator policy", e)
		}
	}
	f := newRemoteFixture(t, true)
	f.mu.Lock()
	f.grants["alice-read"]["scope"] = "nines:read nines:write"
	f.mu.Unlock()
	session, ctx := f.connect(t, "alice-read")
	tools, e := session.ListTools(ctx, nil)
	if e != nil || !hasTool(tools.Tools, "apply_slo") || hasTool(tools.Tools, "deploy_slo_rules") {
		t.Fatal("deploy scope not enforced", e)
	}
}
func TestRemoteInterruptionCancelsUpstreamAndBoundsConcurrency(t *testing.T) {
	f := newRemoteFixture(t, false)
	f.block = make(chan struct{})
	session, ctx := f.connect(t, "alice-read")
	callctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(callctx, &mcp.CallToolParams{Name: "workspace", Arguments: map[string]any{}})
	}()
	select {
	case <-f.entered:
	case <-ctx.Done():
		t.Fatal("upstream call did not start")
	}
	cancel()
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP interruption did not cancel the upstream API")
	}
	<-done
	close(f.block)
}
func TestRemotePerClientConcurrencyAndNoCredentialLeak(t *testing.T) {
	f := newRemoteFixture(t, false)
	for i := 0; i < 4; i++ {
		f.handler.bindings["alice\x00managed"].slots <- struct{}{}
	}
	res := f.post(t, "alice-read", nil, nil)
	if res.StatusCode != 429 {
		t.Fatal("per-client concurrency unbounded", res.StatusCode)
	}
	for i := 0; i < 4; i++ {
		<-f.handler.bindings["alice\x00managed"].slots
	}
	bad := f.post(t, "unknown-token", nil, nil)
	body, e := io.ReadAll(bad.Body)
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"unknown-token", "api-alice", "introspection-secret"} {
		if bytes.Contains(body, []byte(secret)) || strings.Contains(bad.Header.Get("WWW-Authenticate"), secret) {
			t.Fatal("credential leaked in denial")
		}
	}
}

func TestRemoteOAuthDoesNotBypassActualNinesRBAC(t *testing.T) {
	f := newRemoteFixture(t, true)
	svc := service.New(store.NewMemory(), nil)
	handler, e := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Grants: []tenancy.Grant{{Token: "api-alice", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	api := httptest.NewServer(handler)
	defer api.Close()
	f.handler.config.Server = api.URL
	f.mu.Lock()
	f.grants["alice-read"]["scope"] = "nines:read nines:write nines:deploy"
	f.mu.Unlock()
	session, ctx := f.connect(t, "alice-read")
	rejected(t, ctx, session, "apply_slo", map[string]any{"manifest": examples.APIAvailability, "expectedRevision": 0})
	records, e := svc.Store.List(ctx)
	if e != nil || len(records) != 0 {
		t.Fatal("OAuth scope bypassed Nines RBAC", e)
	}
}
func TestRemoteLegacyStatelessMetadataAndOAuthOutage(t *testing.T) {
	f := newRemoteFixture(t, false)
	response := f.post(t, "alice-read", nil, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"legacy-fixture","version":"1"},"capabilities":{}}}`))
	if response.StatusCode != 200 || response.Header.Get("Mcp-Session-Id") != "" {
		t.Fatal("legacy profile created a shared session", response.StatusCode)
	}
	var init map[string]any
	if e := json.NewDecoder(response.Body).Decode(&init); e != nil {
		t.Fatal(e)
	}
	result, ok := init["result"].(map[string]any)
	if !ok || result["protocolVersion"] != "2025-11-25" {
		t.Fatal("legacy protocol not negotiated", init)
	}
	response = f.post(t, "alice-read", nil, strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"workspace","arguments":{}}}`))
	if response.StatusCode != 200 {
		t.Fatal("legacy stateless read failed", response.StatusCode)
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		res, e := f.server.Client().Get(f.server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		var metadata map[string]any
		e = json.NewDecoder(res.Body).Decode(&metadata)
		_ = res.Body.Close()
		if e != nil || metadata["resource"] != f.server.URL+"/mcp" {
			t.Fatal("resource discovery differs", metadata, e)
		}
	}
	f.issuer.Close()
	response = f.post(t, "alice-read", nil, nil)
	if response.StatusCode != 503 || response.Header.Get("WWW-Authenticate") != "" {
		t.Fatal("issuer outage forced a new sign-in", response.StatusCode)
	}
}

func TestRemoteLegacyInterruptionCancelsUpstream(t *testing.T) {
	f := newRemoteFixture(t, false)
	f.block = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace","arguments":{}}}`))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Authorization", "Bearer alice-read")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	done := make(chan struct{})
	go func() {
		defer close(done)
		res, err := f.server.Client().Do(req)
		if err == nil {
			_ = res.Body.Close()
		}
	}()
	select {
	case <-f.entered:
	case <-ctx.Done():
		t.Fatal("legacy upstream call did not start")
	}
	cancel()
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy HTTP interruption did not cancel upstream")
	}
	<-done
	close(f.block)
}

func TestRemoteChunkedBodyLimit(t *testing.T) {
	f := newRemoteFixture(t, false)
	req, e := http.NewRequest(http.MethodPost, f.server.URL+"/mcp", io.NopCloser(strings.NewReader(strings.Repeat(" ", (1<<20)+1))))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Authorization", "Bearer alice-read")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	res, e := f.server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked body exceeded limit: status %d", res.StatusCode)
	}
	if f.apiCalls.Load() != 0 {
		t.Fatal("oversized body reached the Nines API")
	}
}
