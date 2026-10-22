package identity

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/ishantanu/nines/internal/tenancy"
)

func (s *Service) cookieName(purpose string) string {
	prefix := "wp_"
	if strings.HasPrefix(s.config.PublicURL, "https://") {
		prefix = "__Host-wp_"
	}
	return prefix + purpose
}
func (s *Service) setCookie(w http.ResponseWriter, purpose, value string, expires time.Time) {
	age := int(time.Until(expires).Seconds())
	if value == "" {
		age = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(purpose), Value: value, Path: "/", Expires: expires, MaxAge: age, HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https://"), SameSite: http.SameSiteLaxMode})
}
func (s *Service) cookie(r *http.Request, purpose string) (string, error) {
	cookies := r.CookiesNamed(s.cookieName(purpose))
	if len(cookies) != 1 {
		return "", ErrForbidden
	}
	return cookies[0].Value, nil
}
func (s *Service) ValidOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == "" || r.Header.Get("Origin") == s.config.PublicURL
}
func equal(a, b string) bool { return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *Service) RequireCSRF(r *http.Request, a Actor) bool {
	if !a.Session {
		return true
	}
	token, err := s.cookie(r, "session")
	return err == nil && s.ValidOrigin(r) && equal(r.Header.Get("X-CSRF-Token"), hash("csrf:"+token))
}

// Resolve never combines a browser session with an unrelated bearer identity.
// Unknown legacy bearer tokens are checked by the existing server token grants.
func (s *Service) Resolve(r *http.Request) (Actor, bool, error) {
	authorization := r.Header.Get("Authorization")
	if authorization != "" {
		if strings.HasPrefix(authorization, "Bearer wp_cli_") {
			a, e := s.CLIActor(r.Context(), strings.TrimPrefix(authorization, "Bearer "))
			return a, true, e
		}
		if !strings.HasPrefix(authorization, "Bearer wp_sa_") {
			return Actor{}, false, nil
		}
		a, err := s.ServiceActor(r.Context(), strings.TrimPrefix(authorization, "Bearer "))
		return a, true, err
	}
	token, err := s.cookie(r, "session")
	if err != nil {
		return Actor{}, false, nil
	}
	a, err := s.SessionActor(r.Context(), token)
	return a, true, err
}
func authJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func authFailure(w http.ResponseWriter, err error) {
	code := 503
	msg := "Identity service unavailable"
	switch {
	case errors.Is(err, ErrInvalid):
		code = 400
		msg = err.Error()
	case errors.Is(err, ErrForbidden):
		code = 403
		msg = "Permission denied or invalid credentials"
	case errors.Is(err, ErrNotFound):
		code = 404
		msg = "Identity record not found"
	case errors.Is(err, ErrConflict):
		code = 409
		msg = "The record changed. Refresh before retrying."
	}
	authJSON(w, code, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || media != "application/json" {
		return fmt.Errorf("Content-Type must be application/json: %w", ErrInvalid)
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func (s *Service) SessionInfo(w http.ResponseWriter, r *http.Request) {
	token, _ := s.cookie(r, "session")
	a, err := s.SessionActor(r.Context(), token)
	if err == nil {
		authJSON(w, 200, map[string]any{"enabled": true, "authenticated": true, "actor": a, "csrfToken": hash("csrf:" + token), "providers": s.Providers()})
		return
	}
	if !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrNotFound) {
		authFailure(w, err)
		return
	}
	if token != "" {
		s.setCookie(w, "session", "", time.Time{})
	}
	login, _ := s.cookie(r, "login")
	if len(login) != 43 {
		login = randomToken()
		s.setCookie(w, "login", login, s.Now().Add(10*time.Minute))
	}
	authJSON(w, 200, map[string]any{"enabled": true, "authenticated": false, "csrfToken": hash("login:" + login), "providers": s.Providers()})
}
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/providers", func(w http.ResponseWriter, r *http.Request) {
		authJSON(w, 200, map[string]any{"enabled": true, "providers": s.Providers()})
	})
	mux.HandleFunc("GET /auth/session", s.SessionInfo)
	mux.HandleFunc("POST /auth/cli/start", s.cliStart)
	mux.HandleFunc("GET /auth/cli/authorize", s.cliAuthorize)
	mux.HandleFunc("POST /auth/cli/ldap", s.cliLDAP)
	mux.HandleFunc("POST /auth/cli/password", s.cliLDAP)
	mux.HandleFunc("POST /auth/cli/approve", s.cliApprove)
	mux.HandleFunc("POST /auth/cli/exchange", s.cliExchange)
	mux.HandleFunc("POST /auth/cli/logout", s.cliLogout)
	mux.HandleFunc("POST /auth/ldap/{provider}/login", s.ldapLogin)
	mux.HandleFunc("POST /auth/local/{provider}/login", s.localLogin)
	mux.HandleFunc("GET /auth/oidc/{provider}/login", s.oidcStart)
	mux.HandleFunc("GET /auth/oidc/{provider}/callback", s.oidcCallback)
	mux.HandleFunc("POST /auth/logout", s.logout)
	mux.HandleFunc("POST /auth/logout-all", s.logout)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !s.ValidOrigin(r) {
			authFailure(w, ErrForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func DisabledHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path == "/auth/session" || r.URL.Path == "/auth/providers") && r.Method == http.MethodGet {
			authJSON(w, 200, map[string]any{"enabled": false, "authenticated": false, "providers": []ProviderView{}})
			return
		}
		http.NotFound(w, r)
	})
}
func (s *Service) rateLogin(w http.ResponseWriter, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !s.allowLogin(ip) {
		w.Header().Set("Retry-After", "60")
		authJSON(w, 429, map[string]string{"error": "Too many sign-in attempts. Try again later."})
		return false
	}
	return true
}
func (s *Service) ldapLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.providers[r.PathValue("provider")]
	if !ok || p.config.Type != "ldap" {
		authFailure(w, ErrNotFound)
		return
	}
	login, err := s.cookie(r, "login")
	if err != nil || !equal(r.Header.Get("X-CSRF-Token"), hash("login:"+login)) {
		authFailure(w, ErrForbidden)
		return
	}
	if !s.rateLogin(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err = decode(w, r, &in); err != nil {
		authFailure(w, err)
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		authJSON(w, 429, map[string]string{"error": "Sign-in is busy. Try again shortly."})
		return
	}
	e, err := p.authenticateLDAP(r.Context(), in.Username, in.Password)
	in.Password = ""
	if err != nil {
		authJSON(w, 401, map[string]string{"error": "Sign-in failed. Check your credentials or contact your administrator."})
		return
	}
	s.finishLogin(w, r, p.config.ID, e, false)
}
func (s *Service) finishLogin(w http.ResponseWriter, r *http.Request, providerID string, e externalUser, redirect bool) {
	token, actor, err := s.login(r.Context(), providerID, e)
	if err != nil {
		authFailure(w, err)
		return
	}
	// Record a per-tenant sign-in event before issuing the session. No tokens,
	// passwords or identity-provider responses are placed in the audit payload.
	for tenant := range actor.Grants {
		if err = s.RecordAudit(r.Context(), tenant, actor, "sign_in", "/auth/session", "succeeded", "", ""); err != nil {
			_ = s.repo.Update(r.Context(), "session:"+hash(token), func(tx Tx) error { return tx.Delete("session", "global", hash(token)) })
			authFailure(w, err)
			return
		}
	}
	s.setCookie(w, "session", token, actor.Expires)
	s.setCookie(w, "login", "", time.Time{})
	if redirect {
		destination := s.config.PublicURL + "/"
		if next := s.cliReturn(r); next != "" {
			destination = next
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}
	authJSON(w, 200, map[string]any{"enabled": true, "authenticated": true, "actor": actor, "csrfToken": hash("csrf:" + token), "providers": s.Providers()})
}
func (s *Service) oidcStart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.providers[r.PathValue("provider")]
	if !ok || p.config.Type != "oidc" {
		authFailure(w, ErrNotFound)
		return
	}
	if !s.rateLogin(w, r) {
		return
	}
	state := randomToken()
	f := flow{p.config.ID, randomToken(), oauth2.GenerateVerifier(), s.Now().Add(5 * time.Minute)}
	err := s.repo.Update(r.Context(), "flow:"+hash(state), func(tx Tx) error { return put(tx, "flow", "global", hash(state), f, f.Expires) })
	if err != nil {
		authFailure(w, err)
		return
	}
	// One outstanding OIDC flow per browser. A new start invalidates the previous
	// cookie binding; server-side flows expire and are consumed once.
	s.setCookie(w, "flow", state, f.Expires)
	http.Redirect(w, r, p.oauth.AuthCodeURL(state, oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier)), http.StatusFound)
}
func (s *Service) oidcCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cookie, err := s.cookie(r, "flow")
	if err != nil || len(state) != 43 || !equal(state, cookie) {
		authFailure(w, ErrForbidden)
		return
	}
	var f flow
	err = s.repo.Update(r.Context(), "flow:"+hash(state), func(tx Tx) error {
		var e error
		f, e = read[flow](tx, "flow", "global", hash(state))
		if e != nil {
			return e
		}
		return tx.Delete("flow", "global", hash(state))
	})
	s.setCookie(w, "flow", "", time.Time{})
	if err != nil || !f.Expires.After(s.Now()) || f.Provider != r.PathValue("provider") {
		authFailure(w, ErrForbidden)
		return
	}
	p, ok := s.providers[f.Provider]
	if !ok || p.config.Type != "oidc" || r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		authFailure(w, ErrForbidden)
		return
	}
	ctx := oidc.ClientContext(r.Context(), providerHTTP())
	token, err := p.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		authFailure(w, ErrForbidden)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		authFailure(w, ErrForbidden)
		return
	}
	verified, err := p.verifier.Verify(ctx, raw)
	if err != nil || !equal(verified.Nonce, f.Nonce) {
		authFailure(w, ErrForbidden)
		return
	}
	var claims struct {
		Name          string `json:"name"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err = verified.Claims(&claims); err != nil {
		authFailure(w, ErrForbidden)
		return
	}
	if !claims.EmailVerified {
		claims.Email = ""
	}
	authorization, err := parseAuthorization(verified, s.Now())
	if err != nil {
		authFailure(w, ErrForbidden)
		return
	}
	// Flow expiry is created server-side with a fixed five-minute lifetime.
	// Preserve its start time to order callbacks whose tokens share an iat second.
	authorization.FlowStarted = f.Expires.Add(-5 * time.Minute)
	s.finishLogin(w, r, f.Provider, externalUser{Subject: verified.Subject, Name: claims.Name, Email: claims.Email, Authorization: authorization}, true)
}
func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	token, err := s.cookie(r, "session")
	if err != nil {
		authFailure(w, ErrForbidden)
		return
	}
	a, err := s.SessionActor(r.Context(), token)
	if err != nil {
		authFailure(w, ErrForbidden)
		return
	}
	if !s.RequireCSRF(r, a) {
		authFailure(w, ErrForbidden)
		return
	}
	err = s.repo.Update(r.Context(), "user:"+a.ID, func(tx Tx) error {
		if r.URL.Path == "/auth/logout-all" {
			u, e := read[User](tx, "user", "global", a.ID)
			if e != nil {
				return e
			}
			u.Epoch++
			if e = put(tx, "user", "global", a.ID, u, time.Time{}); e != nil {
				return e
			}
		}
		return tx.Delete("session", "global", hash(token))
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	s.setCookie(w, "session", "", time.Time{})
	authJSON(w, 200, map[string]string{"status": "signed_out"})
}
func (s *Service) TenantHandler() http.Handler {
	mux := http.NewServeMux()
	s.mountLocalUsers(mux)
	actor := func(r *http.Request) (Actor, string) {
		p, _ := tenancy.FromContext(r.Context())
		return Actor{ID: p.Subject, Kind: p.Kind, Name: p.Name, Grants: map[string]string{p.TenantID: p.Role}}, p.TenantID
	}
	mux.HandleFunc("GET /api/team-access", func(w http.ResponseWriter, r *http.Request) {
		a, t := actor(r)
		out, err := s.Directory(r.Context(), a, t)
		if err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, out)
	})
	mux.HandleFunc("GET /api/team-access/audit", func(w http.ResponseWriter, r *http.Request) {
		a, t := actor(r)
		before := r.URL.Query().Get("before")
		if len(before) > 100 {
			authFailure(w, ErrInvalid)
			return
		}
		out, err := s.AuditPage(r.Context(), a, t, before)
		if err != nil {
			authFailure(w, err)
			return
		}
		next := ""
		if len(out) == 200 {
			next = out[len(out)-1].ID
		}
		authJSON(w, 200, map[string]any{"items": out, "nextCursor": next})
	})
	mux.HandleFunc("POST /api/team-access/members", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Provider string `json:"provider"`
			Subject  string `json:"subject"`
			Role     string `json:"role"`
			Expected int64  `json:"expectedVersion"`
		}
		if err := decode(w, r, &in); err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		out, err := s.Grant(r.Context(), a, t, in.Provider, in.Subject, in.Role, in.Expected)
		if err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, out)
	})
	expected := func(r *http.Request) (int64, error) {
		raw := r.Header.Get("If-Match")
		n, e := strconv.ParseInt(strings.Trim(raw, `"`), 10, 64)
		if e != nil || n < 1 || raw != `"`+strconv.FormatInt(n, 10)+`"` {
			return 0, ErrInvalid
		}
		return n, nil
	}
	mux.HandleFunc("DELETE /api/team-access/members/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := expected(r)
		if err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		if err = s.RemoveMember(r.Context(), a, t, r.PathValue("id"), v); err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, map[string]string{"status": "removed"})
	})
	mux.HandleFunc("POST /api/team-access/teams", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Role     string   `json:"role"`
			Members  []string `json:"members"`
			Expected int64    `json:"expectedVersion"`
		}
		if err := decode(w, r, &in); err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		out, err := s.SaveTeam(r.Context(), a, t, Team{ID: in.ID, Name: in.Name, Role: in.Role, Members: in.Members}, in.Expected)
		if err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, out)
	})
	mux.HandleFunc("DELETE /api/team-access/teams/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := expected(r)
		if err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		if err = s.RemoveTeam(r.Context(), a, t, r.PathValue("id"), v); err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, map[string]string{"status": "removed"})
	})
	mux.HandleFunc("POST /api/team-access/service-accounts", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID      string    `json:"id"`
			Name    string    `json:"name"`
			Role    string    `json:"role"`
			Expires time.Time `json:"expiresAt"`
		}
		if err := decode(w, r, &in); err != nil {
			authFailure(w, err)
			return
		}
		a, t := actor(r)
		out, token, err := s.CreateAccount(r.Context(), a, t, in.ID, in.Name, in.Role, in.Expires)
		if err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 201, map[string]any{"account": out, "token": token})
	})
	mux.HandleFunc("DELETE /api/team-access/service-accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, t := actor(r)
		if err := s.RevokeAccount(r.Context(), a, t, r.PathValue("id")); err != nil {
			authFailure(w, err)
			return
		}
		authJSON(w, 200, map[string]string{"status": "revoked"})
	})
	return mux
}
