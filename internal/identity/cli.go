package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/tenancy"
)

type cliFlow struct {
	Tenant    string    `json:"tenant"`
	Redirect  string    `json:"redirectURI"`
	Challenge string    `json:"challenge"`
	State     string    `json:"state"`
	Expires   time.Time `json:"expiresAt"`
}
type cliGrant struct {
	SessionHash string    `json:"sessionHash"`
	Tenant      string    `json:"tenant"`
	Redirect    string    `json:"redirectURI"`
	Challenge   string    `json:"challenge"`
	Expires     time.Time `json:"expiresAt"`
}

func validCallback(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/callback" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	port, e := strconv.Atoi(u.Port())
	return e == nil && port > 0 && port < 65536 && ip != nil && ip.IsLoopback()
}
func validOpaque(s string) bool {
	b, e := base64.RawURLEncoding.DecodeString(s)
	return e == nil && len(b) == 32 && len(s) == 43
}
func (s *Service) cliStart(w http.ResponseWriter, r *http.Request) {
	if !s.rateLogin(w, r) {
		return
	}
	var f cliFlow
	if e := decode(w, r, &f); e != nil {
		authFailure(w, e)
		return
	}
	if !tenancy.ValidID(f.Tenant) || !validCallback(f.Redirect) || !validOpaque(f.Challenge) || !validOpaque(f.State) || !f.Expires.IsZero() {
		authFailure(w, ErrInvalid)
		return
	}
	f.Expires = s.Now().Add(5 * time.Minute)
	ticket := randomToken()
	e := s.repo.Update(r.Context(), "cli:requests", func(tx Tx) error {
		rows, e := tx.List("cli_flow", "global")
		if e != nil {
			return e
		}
		if len(rows) >= 1000 {
			return ErrConflict
		}
		return put(tx, "cli_flow", "global", hash(ticket), f, f.Expires)
	})
	if e != nil {
		authFailure(w, e)
		return
	}
	authJSON(w, 200, map[string]any{"authorizationURL": s.config.PublicURL + "/auth/cli/authorize?request=" + ticket, "expiresAt": f.Expires})
}
func (s *Service) readCLIFlow(ctx context.Context, ticket string) (cliFlow, error) {
	var f cliFlow
	if !validOpaque(ticket) {
		return f, ErrInvalid
	}
	e := s.repo.View(ctx, func(tx Tx) error { var e error; f, e = read[cliFlow](tx, "cli_flow", "global", hash(ticket)); return e })
	if e == nil && !f.Expires.After(s.Now()) {
		e = ErrForbidden
	}
	return f, e
}
func (s *Service) cliReturn(r *http.Request) string {
	ticket, e := s.cookie(r, "cli_flow")
	if e != nil {
		return ""
	}
	if _, e = s.readCLIFlow(r.Context(), ticket); e != nil {
		return ""
	}
	return s.config.PublicURL + "/auth/cli/authorize?request=" + ticket
}

var cliPage = template.Must(template.New("cli").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Nines CLI sign-in</title><style>body{margin:0;font:15px system-ui;background:#111318;color:#e5e7eb}header{padding:20px 32px;border-bottom:1px solid #30343b;font-weight:650}main{max-width:500px;margin:64px auto;padding:28px;border:1px solid #30343b;border-radius:8px}h1{font-size:24px;margin:0 0 16px}p{line-height:1.6;color:#b6becb}label{display:block;margin:16px 0 6px}input{box-sizing:border-box;width:100%;padding:10px;border:1px solid #525966;border-radius:5px;background:#171b22;color:white}button,a{display:inline-block;padding:10px 16px;margin:12px 8px 0 0;border:1px solid #525966;border-radius:5px;background:#252b35;color:white;font:inherit;text-decoration:none}button{cursor:pointer}code{word-break:break-all}</style></head><body><header>Nines</header><main><h1>Sign in to Nines CLI</h1><p>Server: <code>{{.Server}}</code><br>Tenant namespace: <strong>{{.Tenant}}</strong></p>{{if .Authenticated}}<p>Continue as <strong>{{.Name}}</strong> with your current <strong>{{.Role}}</strong> role. This signs in the command-line client for up to one hour. Each request rechecks your tenant access.</p><p>Only approve if you started <code>nines login</code> on this computer. Return address: <code>{{.Redirect}}</code></p><form method="post" action="/auth/cli/approve"><input type="hidden" name="request" value="{{.Ticket}}"><input type="hidden" name="csrf" value="{{.CSRF}}"><button type="submit" name="decision" value="approve">Authorize CLI</button><button type="submit" name="decision" value="deny">Cancel</button></form>{{else}}<p>Sign in with your configured identity provider. Your tenant membership controls CLI access.</p>{{range .Providers}}{{if eq .Type "oidc"}}<a href="/auth/oidc/{{.ID}}/login">Sign in with {{.Name}}</a>{{else}}<form method="post" action="/auth/cli/password"><input type="hidden" name="provider" value="{{.ID}}"><input type="hidden" name="csrf" value="{{$.CSRF}}"><label>Username<input name="username" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><button type="submit">Sign in with {{.Name}}</button></form>{{end}}{{end}}{{end}}</main></body></html>`))

func (s *Service) cliAuthorize(w http.ResponseWriter, r *http.Request) {
	ticket := r.URL.Query().Get("request")
	f, e := s.readCLIFlow(r.Context(), ticket)
	if e != nil {
		authFailure(w, e)
		return
	}
	s.setCookie(w, "cli_flow", ticket, f.Expires)
	session, _ := s.cookie(r, "session")
	a, e := s.SessionActor(r.Context(), session)
	data := map[string]any{"Tenant": f.Tenant, "Server": s.config.PublicURL, "Redirect": f.Redirect, "Ticket": ticket, "Providers": s.Providers()}
	if e == nil {
		role := a.Grants[f.Tenant]
		if role == "" {
			authFailure(w, ErrForbidden)
			return
		}
		data["Authenticated"] = true
		data["Name"] = a.Name
		data["Role"] = role
		data["CSRF"] = hash("csrf:" + session)
	} else {
		if !errors.Is(e, ErrNotFound) && !errors.Is(e, ErrForbidden) {
			authFailure(w, e)
			return
		}
		login, _ := s.cookie(r, "login")
		if !validOpaque(login) {
			login = randomToken()
			s.setCookie(w, "login", login, f.Expires)
		}
		data["CSRF"] = hash("login:" + login)
	}
	// Native form navigation needs a non-null Origin. The auth handler's
	// no-referrer policy would suppress it on a POST. Send only the origin,
	// never the authorization request URL or code, while retaining CSRF checks.
	w.Header().Set("Referrer-Policy", "strict-origin")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Some browsers apply form-action to the final redirect as well as the
	// POST. Permit only this request's already validated loopback destination.
	callback, _ := url.Parse(f.Redirect)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self' "+callback.Scheme+"://"+callback.Host+"; frame-ancestors 'none'; base-uri 'none'")
	_ = cliPage.Execute(w, data)
}
func (s *Service) cliLDAP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if e := r.ParseForm(); e != nil {
		authFailure(w, ErrInvalid)
		return
	}
	login, e := s.cookie(r, "login")
	if e != nil || !equal(r.PostForm.Get("csrf"), hash("login:"+login)) || s.cliReturn(r) == "" {
		authFailure(w, ErrForbidden)
		return
	}
	p, ok := s.providers[r.PostForm.Get("provider")]
	if !ok || (p.config.Type != "ldap" && p.config.Type != "local") {
		authFailure(w, ErrForbidden)
		return
	}
	if !s.rateLogin(w, r) {
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		authJSON(w, 429, map[string]string{"error": "Sign-in busy"})
		return
	}
	var u externalUser
	if p.config.Type == "local" {
		u, e = s.authenticateLocal(r.Context(), p.config.ID, r.PostForm.Get("username"), r.PostForm.Get("password"))
	} else {
		u, e = p.authenticateLDAP(r.Context(), r.PostForm.Get("username"), r.PostForm.Get("password"))
	}
	if e != nil {
		authFailure(w, ErrForbidden)
		return
	}
	s.finishLogin(w, r, p.config.ID, u, true)
}
func (s *Service) cliApprove(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if e := r.ParseForm(); e != nil {
		authFailure(w, ErrInvalid)
		return
	}
	ticket := r.PostForm.Get("request")
	cookie, e := s.cookie(r, "cli_flow")
	if e != nil || !equal(ticket, cookie) {
		authFailure(w, ErrForbidden)
		return
	}
	session, e := s.cookie(r, "session")
	if e != nil || !equal(r.PostForm.Get("csrf"), hash("csrf:"+session)) {
		authFailure(w, ErrForbidden)
		return
	}
	a, e := s.SessionActor(r.Context(), session)
	if e != nil {
		authFailure(w, e)
		return
	}
	code := randomToken()
	var f cliFlow
	e = s.repo.Update(r.Context(), "cli:flow:"+hash(ticket), func(tx Tx) error {
		var e error
		f, e = read[cliFlow](tx, "cli_flow", "global", hash(ticket))
		if e != nil {
			return e
		}
		if !f.Expires.After(s.Now()) || a.Grants[f.Tenant] == "" {
			return ErrForbidden
		}
		if e = tx.Delete("cli_flow", "global", hash(ticket)); e != nil {
			return e
		}
		if r.PostForm.Get("decision") != "approve" {
			return nil
		}
		g := cliGrant{hash(session), f.Tenant, f.Redirect, f.Challenge, s.Now().Add(time.Minute)}
		return put(tx, "cli_code", "global", hash(code), g, g.Expires)
	})
	if e != nil {
		authFailure(w, e)
		return
	}
	s.setCookie(w, "cli_flow", "", time.Time{})
	u, _ := url.Parse(f.Redirect)
	q := url.Values{"state": {f.State}}
	if r.PostForm.Get("decision") == "approve" {
		q.Set("code", code)
	} else {
		q.Set("error", "access_denied")
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
func (s *Service) cliExchange(w http.ResponseWriter, r *http.Request) {
	if !s.rateLogin(w, r) {
		return
	}
	var in struct {
		Code     string `json:"code"`
		Verifier string `json:"codeVerifier"`
		Redirect string `json:"redirectURI"`
	}
	if e := decode(w, r, &in); e != nil {
		authFailure(w, e)
		return
	}
	if !validOpaque(in.Code) || !validOpaque(in.Verifier) || !validCallback(in.Redirect) {
		authFailure(w, ErrInvalid)
		return
	}
	sum := sha256.Sum256([]byte(in.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	token := "wp_cli_" + randomToken()
	var g cliGrant
	e := s.repo.Update(r.Context(), "cli:code:"+hash(in.Code), func(tx Tx) error {
		var e error
		g, e = read[cliGrant](tx, "cli_code", "global", hash(in.Code))
		if e != nil {
			return e
		}
		if !g.Expires.After(s.Now()) || g.Redirect != in.Redirect || !equal(g.Challenge, challenge) {
			return ErrForbidden
		}
		a, e := s.sessionActorTx(tx, g.SessionHash)
		if e != nil || a.Grants[g.Tenant] == "" {
			return ErrForbidden
		}
		g.Expires = minTime(a.Expires, s.Now().Add(time.Hour))
		if e = tx.Delete("cli_code", "global", hash(in.Code)); e != nil {
			return e
		}
		return put(tx, "cli_token", "global", hash(token), g, g.Expires)
	})
	if e != nil {
		authFailure(w, e)
		return
	}
	a, e := s.CLIActor(r.Context(), token)
	if e != nil {
		authFailure(w, e)
		return
	}
	if e = s.RecordAudit(r.Context(), g.Tenant, a, "cli_sign_in", "/auth/cli/exchange", "succeeded", "", ""); e != nil {
		_ = s.revokeCLI(r.Context(), token)
		authFailure(w, e)
		return
	}
	authJSON(w, 200, map[string]any{"token": token, "expiresAt": g.Expires, "tenant": g.Tenant, "actor": a, "role": a.Grants[g.Tenant]})
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (s *Service) CLIActor(ctx context.Context, token string) (Actor, error) {
	var a Actor
	if !strings.HasPrefix(token, "wp_cli_") || !validOpaque(strings.TrimPrefix(token, "wp_cli_")) {
		return a, ErrForbidden
	}
	e := s.repo.View(ctx, func(tx Tx) error {
		g, e := read[cliGrant](tx, "cli_token", "global", hash(token))
		if e != nil {
			return e
		}
		if !g.Expires.After(s.Now()) {
			return ErrForbidden
		}
		a, e = s.sessionActorTx(tx, g.SessionHash)
		if e != nil {
			return e
		}
		role := a.Grants[g.Tenant]
		if role == "" {
			return ErrForbidden
		}
		a.Grants = map[string]string{g.Tenant: role}
		a.Session = false
		a.Expires = minTime(a.Expires, g.Expires)
		return nil
	})
	return a, e
}
func (s *Service) revokeCLI(ctx context.Context, token string) error {
	return s.repo.Update(ctx, "cli:token:"+hash(token), func(tx Tx) error { return tx.Delete("cli_token", "global", hash(token)) })
}
func (s *Service) cliLogout(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	a, e := s.CLIActor(r.Context(), token)
	if e != nil {
		authFailure(w, e)
		return
	}
	for t := range a.Grants {
		if e = s.RecordAudit(r.Context(), t, a, "cli_sign_out", "/auth/cli/logout", "succeeded", "", ""); e != nil {
			authFailure(w, e)
			return
		}
	}
	if e = s.revokeCLI(r.Context(), token); e != nil {
		authFailure(w, e)
		return
	}
	authJSON(w, 200, map[string]string{"status": "signed_out"})
}
