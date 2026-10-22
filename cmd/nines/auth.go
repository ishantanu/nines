package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/tenancy"
)

func authRequest(ctx context.Context, h *http.Client, server, tenant, token, method, path string, in, out any) error {
	var body []byte
	var e error
	if in != nil {
		body, e = json.Marshal(in)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, server+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tenant != "" {
		req.Header.Set("X-Nines-Tenant", tenant)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, e := h.Do(req)
	if e != nil {
		return fmt.Errorf("nines connection failed: %w", e)
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != 200 {
		return &client.APIError{StatusCode: res.StatusCode}
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (128<<10)+1))
	if e != nil {
		return e
	}
	if len(b) > 128<<10 {
		return fmt.Errorf("identity response exceeds 128 KiB")
	}
	if out == nil {
		return nil
	}
	if json.Unmarshal(b, out) != nil {
		return fmt.Errorf("invalid Nines identity response")
	}
	return nil
}
func (o *options) authStatus(ctx context.Context) (map[string]any, error) {
	if o.Token == "" {
		return nil, &cliError{"unauthenticated", "No CLI credential; run nines login or set NINES_API_KEY", 3}
	}
	var list []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if e := authRequest(ctx, o.httpClient(), o.Server, o.Tenant, o.Token, "GET", "/api/tenants", nil, &list); e != nil {
		return nil, e
	}
	for _, t := range list {
		if t.ID == o.Tenant && (t.Role == "owner" || t.Role == "editor" || t.Role == "agent" || t.Role == "viewer") {
			return map[string]any{"authenticated": true, "context": o.Context, "server": o.Server, "tenant": o.Tenant, "role": t.Role, "credentialSource": o.credentialSource}, nil
		}
	}
	return nil, &cliError{"forbidden", "Credential does not grant the selected tenant namespace", 4}
}
func authCommand(o *options) *cobra.Command {
	root := &cobra.Command{Use: "auth", Short: "Inspect or revoke CLI authentication"}
	root.AddCommand(mark(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		v, e := o.authStatus(c.Context())
		if e != nil {
			return e
		}
		return render(c.OutOrStdout(), v, o.format("json"), "auth")
	}}, "server", "read", "Check effective tenant access without printing credentials"))
	logout := mark(&cobra.Command{Use: "logout", Args: cobra.NoArgs}, "login", "logout", "Revoke and remove a saved browser session")
	local := logout.Flags().Bool("local", false, "Remove the local credential without contacting Nines")
	logout.RunE = func(c *cobra.Command, args []string) error {
		if o.Context == "" {
			return usage("logout requires a selected --context")
		}
		p, e := o.configPath()
		if e != nil {
			return e
		}
		stored, e := credentialsLoad(p)
		if e != nil {
			return e
		}
		saved, exists := stored[o.Context]
		if !*local && exists {
			if saved.Server != o.Server || saved.Tenant != o.Tenant {
				return usage("saved browser credential belongs to a different server or tenant; select its original context or use --local")
			}
			e = authRequest(c.Context(), o.httpClient(), saved.Server, saved.Tenant, saved.Token, "POST", "/auth/cli/logout", map[string]any{}, nil)
			if ae, ok := e.(*client.APIError); ok && (ae.StatusCode == 401 || ae.StatusCode == 403 || ae.StatusCode == 404) {
				e = nil
			}
			if e != nil {
				return e
			}
		}
		e = withConfigLock(p, func() error {
			m, e := credentialsLoad(p)
			if e != nil {
				return e
			}
			delete(m, o.Context)
			return credentialsSave(p, m)
		})
		if e != nil {
			return e
		}
		note := "Saved browser credential removed"
		if o.credentialSource == "NINES_API_KEY" || o.credentialSource == "API_TOKEN" {
			note = "Environment API key remains active; unset it in your shell. API keys are not revoked by logout."
		}
		return render(c.OutOrStdout(), map[string]any{"context": o.Context, "status": "signed_out", "localOnly": *local, "note": note}, o.format("json"), "auth")
	}
	root.AddCommand(logout)
	return root
}
func loginCommand(o *options) *cobra.Command {
	cmd := mark(&cobra.Command{Use: "login", Args: cobra.NoArgs}, "login", "login", "Authenticate one tenant context via API key or browser SSO")
	method := cmd.Flags().String("method", "auto", "auto, api-key or sso")
	noBrowser := cmd.Flags().Bool("no-browser", false, "Print the sign-in URL instead of opening a browser")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		selected := *method
		if selected == "auto" {
			selected = "sso"
			if o.credentialSource == "NINES_API_KEY" || o.credentialSource == "API_TOKEN" {
				selected = "api-key"
			}
		}
		if selected != "api-key" && selected != "sso" {
			return usage("method must be auto, api-key or sso")
		}
		if o.Context == "" {
			o.Context = o.Tenant
		}
		if !tenancy.ValidID(o.Context) {
			return usage("invalid context name")
		}
		var cred *credential
		if selected == "api-key" {
			if o.credentialSource != "NINES_API_KEY" && o.credentialSource != "API_TOKEN" {
				return usage("API-key login requires NINES_API_KEY or API_TOKEN")
			}
		} else {
			if o.Agent {
				return usage("agent mode cannot perform browser login; supply NINES_API_KEY or a pre-authenticated context")
			}
			if v, _ := environmentToken(); v != "" {
				return usage("unset NINES_API_KEY and API_TOKEN before choosing browser SSO")
			}
			v, e := browserLogin(c.Context(), o, *noBrowser, c.ErrOrStderr())
			if e != nil {
				return e
			}
			cred = &v
			o.Token = v.Token
			o.credentialSource = "browser-session"
		}
		v, e := o.authStatus(c.Context())
		if e != nil {
			return e
		}
		p, e := o.configPath()
		if e != nil {
			return e
		}
		e = withConfigLock(p, func() error {
			cfg, e := configLoad(p)
			if e != nil {
				return e
			}
			cfg.Contexts[o.Context] = savedContext{o.Server, o.Tenant}
			cfg.Current = o.Context
			if cred != nil {
				m, e := credentialsLoad(p)
				if e != nil {
					return e
				}
				m[o.Context] = *cred
				if e = credentialsSave(p, m); e != nil {
					return e
				}
			}
			return configSave(p, cfg)
		})
		if e != nil {
			return e
		}
		return render(c.OutOrStdout(), v, o.format("json"), "auth")
	}
	return cmd
}
func nonce() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

var launchBrowser = func(raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Run()
}

func browserLogin(ctx context.Context, o *options, noBrowser bool, errOut io.Writer) (credential, error) {
	var zero credential
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return zero, e
	}
	defer func() {
		_ = ln.Close()
	}()
	redirect := "http://" + ln.Addr().String() + "/callback"
	state, e := nonce()
	if e != nil {
		return zero, e
	}
	verifier, e := nonce()
	if e != nil {
		return zero, e
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	var started struct {
		URL     string    `json:"authorizationURL"`
		Expires time.Time `json:"expiresAt"`
	}
	e = authRequest(ctx, o.httpClient(), o.Server, "", "", "POST", "/auth/cli/start", map[string]string{"tenant": o.Tenant, "redirectURI": redirect, "challenge": challenge, "state": state}, &started)
	if e != nil {
		return zero, e
	}
	u, e := url.Parse(started.URL)
	base, _ := url.Parse(o.Server)
	if e != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.Fragment != "" || u.Path != "/auth/cli/authorize" || len(u.Query()["request"]) != 1 || len(u.Query()) != 1 || len(u.Query().Get("request")) != 43 || !started.Expires.After(time.Now()) || started.Expires.After(time.Now().Add(6*time.Minute)) {
		return zero, fmt.Errorf("invalid Nines CLI authorization response")
	}
	loginCtx, cancel := context.WithDeadline(ctx, started.Expires)
	defer cancel()
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	var once sync.Once
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q := r.URL.Query()
		if r.Method != "GET" || r.URL.Path != "/callback" || r.Host != ln.Addr().String() || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid login callback", 400)
			return
		}
		v := result{code: q.Get("code")}
		if q.Get("error") != "" {
			v.err = &cliError{"unauthenticated", "Browser sign-in was cancelled", 3}
		} else if len(q["code"]) != 1 || len(v.code) != 43 {
			http.Error(w, "Invalid login code", 400)
			return
		}
		once.Do(func() { done <- v })
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// The login result was delivered; a disconnected browser cannot change it.
		_, _ = fmt.Fprintln(w, "Nines CLI sign-in received. Return to your terminal to check the result. You can close this tab.")
	})}
	go func() { _ = server.Serve(ln) }()
	defer func() {
		_ = server.Close()
	}()
	if _, e = fmt.Fprintf(errOut, "Sign in to tenant %s at %s\n%s\n", o.Tenant, o.Server, started.URL); e != nil {
		return zero, e
	}
	if !noBrowser {
		if e = launchBrowser(started.URL); e != nil {
			if _, e = fmt.Fprintln(errOut, "Browser did not open. Open the URL above on this computer."); e != nil {
				return zero, e
			}
		}
	}
	var callback result
	select {
	case callback = <-done:
	case <-loginCtx.Done():
		return zero, fmt.Errorf("cLI sign-in did not complete: %w", loginCtx.Err())
	}
	if callback.err != nil {
		return zero, callback.err
	}
	var exchanged struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expiresAt"`
		Tenant  string    `json:"tenant"`
	}
	e = authRequest(loginCtx, o.httpClient(), o.Server, "", "", "POST", "/auth/cli/exchange", map[string]string{"code": callback.code, "codeVerifier": verifier, "redirectURI": redirect}, &exchanged)
	if e != nil {
		return zero, e
	}
	if !strings.HasPrefix(exchanged.Token, "wp_cli_") || len(exchanged.Token) != 50 || exchanged.Tenant != o.Tenant || !exchanged.Expires.After(time.Now()) || exchanged.Expires.After(time.Now().Add(61*time.Minute)) {
		return zero, fmt.Errorf("invalid Nines CLI credential response")
	}
	return credential{o.Server, o.Tenant, exchanged.Token, exchanged.Expires}, nil
}
