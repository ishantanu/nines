// Package testauth contains loopback-only protocol fixtures for authentication
// tests and local UI verification. It is not wired into the application server.
package testauth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type OIDC struct {
	Server        *httptest.Server
	mu            sync.Mutex
	redirects     map[string]url.URL
	codes         map[string]oidcCode
	key           *rsa.PrivateKey
	BadNonce      bool
	BadAudience   bool
	BadIssuer     bool
	Expired       bool
	BadSignature  bool
	authorization map[string]any
}
type oidcCode struct{ nonce, challenge, redirect, subject string }

func NewOIDC() (*OIDC, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	o := &OIDC{key: key, redirects: map[string]url.URL{}, codes: map[string]oidcCode{}}
	o.Server = httptest.NewServer(http.HandlerFunc(o.serve))
	return o, nil
}

// SetAuthorizationClaims supplies synthetic signed authorization claims. It
// cannot override identity, signature or flow binding fields.
func (o *OIDC) SetAuthorizationClaims(claims map[string]any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.authorization = map[string]any{}
	for _, k := range []string{"groups", "roles", "hasgroups", "_claim_names", "_claim_sources"} {
		if v, ok := claims[k]; ok {
			b, _ := json.Marshal(v)
			var copied any
			_ = json.Unmarshal(b, &copied)
			o.authorization[k] = copied
		}
	}
}
func (o *OIDC) Close() { o.Server.Close() }

// AllowRedirect registers a trusted callback before authorization begins.
func (o *OIDC) AllowRedirect(uri string) error {
	u, err := url.Parse(uri)
	if err != nil || u.User != nil || u.Host == "" || u.Path == "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback()) {
		return fmt.Errorf("fixture redirect must be a loopback HTTP(S) callback without credentials, query or fragment")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.redirects[uri] = *u
	return nil
}
func (o *OIDC) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": o.Server.URL, "authorization_endpoint": o.Server.URL + "/authorize", "token_endpoint": o.Server.URL + "/token", "jwks_uri": o.Server.URL + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"}})
	case "/keys":
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &o.key.PublicKey, KeyID: "fixture-key", Algorithm: "RS256", Use: "sig"}}})
	case "/authorize":
		q := r.URL.Query()
		o.mu.Lock()
		callback, allowed := o.redirects[q.Get("redirect_uri")]
		o.mu.Unlock()
		if !allowed || q.Get("client_id") != "fixture-client" || q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" {
			http.Error(w, "invalid fixture authorization", 400)
			return
		}
		sub := q.Get("login_as")
		if sub != "alice" && sub != "bob" && sub != "outsider" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, "<!doctype html><html><head><title>Local identity fixture</title></head><body><h1>Local identity fixture</h1><p>Synthetic accounts for protocol verification.</p>")
			for _, name := range []string{"alice", "bob", "outsider"} {
				copyQ := r.URL.Query()
				copyQ.Set("login_as", name)
				_, _ = fmt.Fprintf(w, "<p><a href=\"%s\">Sign in as %s</a></p>", template.HTMLEscapeString("/authorize?"+copyQ.Encode()), name)
			}
			_, _ = fmt.Fprint(w, "</body></html>")
			return
		}
		code := fmt.Sprintf("fixture-%d", time.Now().UnixNano())
		o.mu.Lock()
		o.codes[code] = oidcCode{q.Get("nonce"), q.Get("code_challenge"), callback.String(), sub}
		o.mu.Unlock()
		// Build the destination from the registered URL, never from request data.
		u := callback
		values := u.Query()
		values.Set("state", q.Get("state"))
		values.Set("code", code)
		u.RawQuery = values.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	case "/token":
		if r.Method != "POST" {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "form", http.StatusBadRequest)
			return
		}
		client, secret, ok := r.BasicAuth()
		if !ok {
			client = r.Form.Get("client_id")
			secret = r.Form.Get("client_secret")
		}
		if client != "fixture-client" || secret != "fixture-secret" {
			http.Error(w, "client", http.StatusUnauthorized)
			return
		}
		o.mu.Lock()
		code, ok := o.codes[r.Form.Get("code")]
		delete(o.codes, r.Form.Get("code"))
		o.mu.Unlock()
		challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(challenge[:]) != code.challenge || r.Form.Get("redirect_uri") != code.redirect {
			http.Error(w, "invalid grant", 400)
			return
		}
		nonce, issuer, aud := code.nonce, o.Server.URL, "fixture-client"
		exp := time.Now().Add(10 * time.Minute)
		if o.BadNonce {
			nonce = "other"
		}
		if o.BadIssuer {
			issuer = "https://other.invalid"
		}
		if o.BadAudience {
			aud = "other"
		}
		if o.Expired {
			exp = time.Now().Add(-time.Hour)
		}
		key := o.key
		if o.BadSignature {
			key, _ = rsa.GenerateKey(rand.Reader, 2048)
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "fixture-key"))
		if err != nil {
			panic(err)
		}
		values := map[string]any{"iss": issuer, "sub": code.subject, "aud": aud, "exp": exp.Unix(), "iat": time.Now().Unix(), "nonce": nonce, "name": map[string]string{"alice": "Alice Example", "bob": "Bob Example", "outsider": "No Membership"}[code.subject], "email": code.subject + "@example.test", "email_verified": true}
		o.mu.Lock()
		for k, v := range o.authorization {
			values[k] = v
		}
		o.mu.Unlock()
		claims, _ := json.Marshal(values)
		signed, err := signer.Sign(claims)
		if err != nil {
			panic(err)
		}
		compact, err := signed.CompactSerialize()
		if err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-unused-access-token", "token_type": "Bearer", "expires_in": 600, "id_token": compact})
	default:
		http.NotFound(w, r)
	}
}
