package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

const RemoteReadScope = "nines:read"

var errIntrospectionUnavailable = errors.New("OAuth introspection unavailable")

// RemoteHandler implements the stateless Streamable HTTP profile. Authentication
// and scope resolution happen again on every HTTP request; no server session,
// bearer token cache or admin credential is shared across clients.
type RemoteHandler struct {
	limiter     *rate.Limiter
	config      RemoteConfig
	endpoint    *url.URL
	metadataURL string
	client      *http.Client
	slots       chan struct{}
	bindings    map[string]*remoteBinding
	now         func() time.Time
	logger      *slog.Logger
}
type remoteBinding struct {
	limiter *rate.Limiter
	RemoteBinding
	slots chan struct{}
}
type introspection struct {
	Active    bool            `json:"active"`
	Subject   string          `json:"sub"`
	ClientID  string          `json:"client_id"`
	Issuer    string          `json:"iss"`
	Audience  json.RawMessage `json:"aud"`
	Scope     string          `json:"scope"`
	Expires   int64           `json:"exp"`
	NotBefore int64           `json:"nbf"`
}

func NewRemote(c RemoteConfig) (*RemoteHandler, error) {
	if e := c.validate(); e != nil {
		return nil, e
	}
	c.AllowedOrigins = slices.Clone(c.AllowedOrigins)
	c.Bindings = slices.Clone(c.Bindings)
	u, _ := url.Parse(c.PublicURL)
	client := c.http
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	logger := c.logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &RemoteHandler{config: c, endpoint: u, metadataURL: u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource" + u.Path, client: client, slots: make(chan struct{}, 16), limiter: rate.NewLimiter(20, 40), bindings: map[string]*remoteBinding{}, now: time.Now, logger: logger}
	for _, b := range c.Bindings {
		h.bindings[b.Subject+"\x00"+b.ClientID] = &remoteBinding{RemoteBinding: b, slots: make(chan struct{}, 4), limiter: rate.NewLimiter(5, 10)}
	}
	return h, nil
}
func (h *RemoteHandler) token(ctx context.Context, token string) (introspection, error) {
	var out introspection
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	form := url.Values{"token": {token}, "token_type_hint": {"access_token"}}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, h.config.IntrospectionURL, strings.NewReader(form.Encode()))
	if e != nil {
		return out, errIntrospectionUnavailable
	}
	req.SetBasicAuth(h.config.IntrospectionClientID, h.config.secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, e := h.client.Do(req)
	if e != nil {
		return out, errIntrospectionUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	data, e := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if e != nil || len(data) > 64<<10 || res.StatusCode != 200 {
		return out, errIntrospectionUnavailable
	}
	if json.Unmarshal(data, &out) != nil {
		return out, errIntrospectionUnavailable
	}
	var audience []string
	if json.Unmarshal(out.Audience, &audience) != nil {
		var one string
		if json.Unmarshal(out.Audience, &one) != nil {
			return out, errors.New("invalid audience")
		}
		audience = []string{one}
	}
	now := h.now().Unix()
	if !out.Active || out.Expires-now > 3600 || out.Issuer != h.config.Issuer || !slices.Contains(audience, h.config.PublicURL) || out.Expires <= now || out.NotBefore > now || !remoteText(out.Subject, 512) || !remoteText(out.ClientID, 512) || len(out.Scope) > 4096 {
		return out, errors.New("invalid access grant")
	}
	return out, nil
}
func (h *RemoteHandler) challenge(w http.ResponseWriter, status int, code string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q, error=%q`, h.metadataURL, RemoteReadScope, code))
	remoteJSON(w, status, map[string]string{"error": code})
}
func remoteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (h *RemoteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Host != h.endpoint.Host || len(r.Header.Values("Origin")) > 1 {
		remoteJSON(w, 403, map[string]string{"error": "origin_denied"})
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		if origin != h.endpoint.Scheme+"://"+h.endpoint.Host && !slices.Contains(h.config.AllowedOrigins, origin) {
			remoteJSON(w, 403, map[string]string{"error": "origin_denied"})
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, MCP-Protocol-Version")
	}
	if r.URL.RawQuery != "" || r.URL.RawPath != "" {
		remoteJSON(w, 400, map[string]string{"error": "query_and_path_overrides_forbidden"})
		return
	}
	if r.URL.Path == "/.well-known/oauth-protected-resource" || r.URL.Path == strings.TrimPrefix(h.metadataURL, h.endpoint.Scheme+"://"+h.endpoint.Host) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(405)
			return
		}
		remoteJSON(w, 200, map[string]any{"resource": h.config.PublicURL, "resource_name": "Nines tenant " + h.config.Tenant, "authorization_servers": []string{h.config.Issuer}, "scopes_supported": []string{RemoteReadScope}, "bearer_methods_supported": []string{"header"}})
		return
	}
	if r.URL.Path != h.endpoint.Path {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, MCP-Protocol-Version, MCP-Session-Id")
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(405)
		return
	}
	if len(r.Header.Values("Authorization")) != 1 || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		h.challenge(w, 401, "invalid_token")
		return
	}
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !remoteText(bearer, 8192) {
		h.challenge(w, 401, "invalid_token")
		return
	}
	// Sessions and event replay are deliberately not part of this profile.
	if r.Header.Get("Mcp-Session-Id") != "" || r.Header.Get("Last-Event-ID") != "" {
		remoteJSON(w, 400, map[string]string{"error": "stateless_transport_has_no_session_or_event_replay"})
		return
	}
	if r.ContentLength > 1<<20 {
		w.WriteHeader(413)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		return
	}
	if !h.limiter.Allow() {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		return
	}
	grant, e := h.token(r.Context(), bearer)
	if errors.Is(e, errIntrospectionUnavailable) {
		w.Header().Set("Retry-After", "1")
		remoteJSON(w, 503, map[string]string{"error": "authorization_service_unavailable"})
		return
	}
	if e != nil {
		h.challenge(w, 401, "invalid_token")
		return
	}
	scopes := strings.Fields(grant.Scope)
	if !slices.Contains(scopes, RemoteReadScope) {
		h.challenge(w, 403, "insufficient_scope")
		return
	}
	binding := h.bindings[grant.Subject+"\x00"+grant.ClientID]
	if binding == nil {
		h.challenge(w, 403, "insufficient_scope")
		return
	}
	if !binding.limiter.Allow() {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		return
	}
	select {
	case binding.slots <- struct{}{}:
		defer func() { <-binding.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		return
	}
	deadline := h.now().Add(30 * time.Second)
	if expiry := time.Unix(grant.Expires, 0); expiry.Before(deadline) {
		deadline = expiry
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	writes := h.config.AllowWrites && slices.Contains(scopes, "nines:write")
	deploy := writes && h.config.AllowRuleDeployment && slices.Contains(scopes, "nines:deploy")
	withdrawal := deploy && h.config.AllowRuleWithdrawal && slices.Contains(scopes, "nines:withdraw")
	s, e := New(Config{Server: h.config.Server, Tenant: h.config.Tenant, Token: binding.token, AllowWrites: writes, AllowRuleDeployment: deploy, AllowRuleWithdrawal: withdrawal, requestContext: ctx, protocolVersions: []string{"2026-07-28", "2025-11-25"}})
	if e != nil {
		remoteJSON(w, 503, map[string]string{"error": "adapter_unavailable"})
		return
	}
	// The exact canonical Host was validated above, replacing the SDK loopback
	// heuristic for TLS-terminating proxies. Each request gets its own server and identity. The official SDK handles MCP
	// versions, framing and JSON-RPC validation; no provider/tool arguments are logged.
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true, DisableLocalhostProtection: true})
	recorded := &remoteResponse{ResponseWriter: w, status: 200}
	transport.ServeHTTP(recorded, r.WithContext(ctx))
	digest := sha256.Sum256([]byte(grant.Subject))
	h.logger.InfoContext(context.WithoutCancel(ctx), "remote MCP request", "tenant", h.config.Tenant, "client", grant.ClientID, "subjectDigest", hex.EncodeToString(digest[:8]), "status", recorded.status)
}

type remoteResponse struct {
	http.ResponseWriter
	status int
}

func (w *remoteResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *remoteResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
