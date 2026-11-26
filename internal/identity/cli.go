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
	DeviceHash string    `json:"deviceHash,omitempty"`
	UserCode   string    `json:"userCode,omitempty"`
	Tenant     string    `json:"tenant"`
	Redirect   string    `json:"redirectURI"`
	Challenge  string    `json:"challenge"`
	State      string    `json:"state"`
	Expires    time.Time `json:"expiresAt"`
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
	var in CLIStartInput
	if e := decode(w, r, &in); e != nil {
		authFailure(w, e)
		return
	}
	if !tenancy.ValidID(in.Tenant) || !validCallback(in.Redirect) || !validOpaque(in.Challenge) || !validOpaque(in.State) {
		authFailure(w, ErrInvalid)
		return
	}
	f := cliFlow{Tenant: in.Tenant, Redirect: in.Redirect, Challenge: in.Challenge, State: in.State, Expires: s.Now().Add(5 * time.Minute)}
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
	authJSON(w, 200, CLIStarted{AuthorizationURL: s.config.PublicURL + "/auth/cli/authorize?request=" + ticket, Expires: f.Expires})
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

var cliPage = template.Must(template.New("cli").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Nines CLI sign-in</title><style>
:root{color-scheme:light dark;--bg:#f7f7f5;--surface:#fff;--ink:#292731;--muted:#625f6c;--rule:#dfdce4;--field:#827b8d;--accent:#62508f;--on-accent:#fff}
@media(prefers-color-scheme:dark){:root{--bg:#1c1c1e;--surface:#242426;--ink:#f5f5f7;--muted:#b0b0b5;--rule:#3a3a3c;--field:#9a9aa3;--accent:#b5a1df;--on-accent:#21192e}}
*{box-sizing:border-box}body{margin:0;font:15px/1.6 system-ui,sans-serif;background:var(--bg);color:var(--ink)}header{padding:20px max(24px,calc((100vw - 1040px)/2));border-bottom:1px solid var(--rule);font-size:20px;font-weight:650;letter-spacing:-.02em}main{width:min(100% - 32px,520px);margin:48px auto;padding:clamp(20px,5vw,32px);border:1px solid var(--rule);border-radius:8px;background:var(--surface)}h1{font-size:24px;line-height:1.3;margin:0 0 20px;letter-spacing:-.02em}p{color:var(--muted);overflow-wrap:anywhere}strong{color:var(--ink)}label{display:block;margin:20px 0 8px;font-weight:550}input{width:100%;min-height:44px;margin-top:8px;padding:10px 12px;border:1px solid var(--field);border-radius:5px;background:var(--surface);color:var(--ink);font:inherit}input[type=hidden]{display:none}button,a{display:inline-flex;align-items:center;justify-content:center;min-height:44px;padding:10px 16px;margin:16px 8px 0 0;border:1px solid var(--field);border-radius:5px;background:transparent;color:var(--ink);font:inherit;font-weight:550;text-decoration:none}button{cursor:pointer}button[value=approve],button:not([name]){background:var(--accent);color:var(--on-accent);border-color:var(--accent)}button:hover,a:hover{filter:brightness(.94)}:focus-visible{outline:3px solid var(--accent);outline-offset:3px}code{font-size:.9em;overflow-wrap:anywhere}.device-code{display:block;width:fit-content;margin:14px 0;padding:10px 16px;border:1px solid var(--rule);background:var(--bg);font:600 24px/1.3 ui-monospace,monospace;letter-spacing:.12em;user-select:all}input[name=userCode]{font-family:ui-monospace,monospace;letter-spacing:.1em;text-transform:uppercase}form+form{margin-top:24px;padding-top:4px;border-top:1px solid var(--rule)}@media(max-width:420px){main{margin-block:24px}button,a{width:100%;margin-right:0}}::selection{background:var(--accent);color:var(--on-accent)}
</style></head><body><header>Nines</header><main><h1>Sign in to Nines CLI</h1><p>Server: <code>{{.Server}}</code><br>Tenant namespace: <strong>{{.Tenant}}</strong></p>{{if .Authenticated}}<p>Continue as <strong>{{.Name}}</strong> with your current <strong>{{.Role}}</strong> role. This signs in the command-line client for up to one hour. Each request rechecks your tenant access.</p>{{if .UserCode}}<p>Confirm the code from your terminal: <strong class="device-code">{{.UserCode}}</strong>Do not authorize a request started by someone else.</p>{{end}}{{if .UserCode}}<p>Only approve if you started <code>nines login --method device</code> in your own terminal. The terminal can be on another computer.</p>{{else}}<p>Only approve if you started <code>nines login</code> on this computer. Return address: <code>{{.Redirect}}</code></p>{{end}}<form method="post" action="/auth/cli/approve"><input type="hidden" name="request" value="{{.Ticket}}"><input type="hidden" name="csrf" value="{{.CSRF}}">{{if .UserCode}}<label for="device-user-code">Code from your terminal<input id="device-user-code" name="userCode" autocomplete="off" autocapitalize="characters" spellcheck="false" maxlength="9" required aria-describedby="device-code-help"></label><p id="device-code-help">Enter the code shown by your own CLI. Cancel if you did not start this request.</p>{{end}}<button type="submit" name="decision" value="approve">Authorize CLI</button><button type="submit" name="decision" value="deny">Cancel</button></form>{{else}}<p>Sign in with your configured identity provider. Your tenant membership controls CLI access.</p>{{range .Providers}}{{if eq .Type "oidc"}}<a href="/auth/oidc/{{.ID}}/login">Sign in with {{.Name}}</a>{{else}}<form method="post" action="/auth/cli/password"><input type="hidden" name="provider" value="{{.ID}}"><input type="hidden" name="csrf" value="{{$.CSRF}}"><label>Username<input name="username" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><button type="submit">Sign in with {{.Name}}</button></form>{{end}}{{end}}{{end}}</main></body></html>`))

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
	data := map[string]any{"Tenant": f.Tenant, "Server": s.config.PublicURL, "Redirect": f.Redirect, "Ticket": ticket, "Providers": s.Providers(), "UserCode": f.UserCode}
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
	formAction := "'self'"
	if f.DeviceHash == "" {
		callback, _ := url.Parse(f.Redirect)
		formAction += " " + callback.Scheme + "://" + callback.Host
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action "+formAction+"; frame-ancestors 'none'; base-uri 'none'")
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
		if f.DeviceHash != "" {
			if r.PostForm.Get("decision") == "approve" && !equal(strings.ToUpper(strings.TrimSpace(r.PostForm.Get("userCode"))), f.UserCode) {
				return ErrForbidden
			}
			device, e := read[cliDevice](tx, "cli_device", "global", f.DeviceHash)
			if e != nil {
				return e
			}
			device.Status = "access_denied"
			if r.PostForm.Get("decision") == "approve" {
				device.Status = "approved"
				device.SessionHash = hash(session)
			}
			if e = put(tx, "cli_device", "global", f.DeviceHash, device, device.Expires); e != nil {
				return e
			}
			return tx.Delete("cli_flow", "global", hash(ticket))
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
	if f.DeviceHash != "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Nines CLI decision recorded. Return to the original terminal. You can close this tab."))
		return
	}
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
	var in CLIExchangeInput
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
	authJSON(w, 200, CLIToken{Token: token, Expires: g.Expires, Tenant: g.Tenant, Actor: a, Role: a.Grants[g.Tenant]})
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
	authJSON(w, 200, StatusResult{Status: "signed_out"})
}
