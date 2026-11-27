package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	logout := mark(&cobra.Command{Use: "logout", Args: cobra.NoArgs}, "login", "logout", "Revoke and remove a saved CLI session")
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
				return usage("saved CLI credential belongs to a different server or tenant; select its original context or use --local")
			}
			token, err := storedToken(p, saved, o.Agent)
			if err != nil {
				return err
			}
			e = authRequest(c.Context(), o.httpClient(), saved.Server, saved.Tenant, token, "POST", "/auth/cli/logout", map[string]any{}, nil)
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
			current, ok := m[o.Context]
			if ok && current != saved {
				return &cliError{"conflict", "Credential changed during logout; retry", 5}
			}
			if e = deleteStoredCredential(p, current, o.Agent); e != nil {
				return e
			}
			delete(m, o.Context)
			return credentialsSave(p, m)
		})
		if e != nil {
			return e
		}
		note := "Saved CLI credential removed"
		if o.credentialSource == "NINES_API_KEY" || o.credentialSource == "API_TOKEN" {
			note = "Environment API key remains active; unset it in your shell. API keys are not revoked by logout."
		}
		return render(c.OutOrStdout(), map[string]any{"context": o.Context, "status": "signed_out", "localOnly": *local, "note": note}, o.format("json"), "auth")
	}
	root.AddCommand(logout)
	root.AddCommand(mark(&cobra.Command{Use: "diagnose", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		capabilities, e := authCapabilities(c.Context(), o)
		if e != nil {
			return e
		}
		return render(c.OutOrStdout(), map[string]any{"server": o.Server, "tenant": o.Tenant, "context": o.Context, "capabilities": capabilities, "credentialSource": o.credentialSource, "keychainAccessed": false, "fallback": "NINES_API_KEY; automatic refresh is not supported"}, o.format("json"), "auth")
	}}, "login", "read", "Discover login capabilities without reading OS credentials or printing secrets"))
	return root
}
func loginCommand(o *options) *cobra.Command {
	cmd := mark(&cobra.Command{Use: "login", Args: cobra.NoArgs}, "login", "login", "Authenticate one tenant context via API key, browser or device code")
	method := cmd.Flags().String("method", "auto", "auto, api-key, sso or device")
	storage := cmd.Flags().String("credential-store", "file", "file or opt-in keychain; API keys are never saved")
	noBrowser := cmd.Flags().Bool("no-browser", false, "Print the sign-in URL instead of opening a browser")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if *storage != "file" && *storage != "keychain" {
			return usage("credential-store must be file or keychain")
		}
		selected := *method
		if o.Agent && selected != "auto" && selected != "api-key" {
			return usage("agent mode cannot initiate interactive login; supply NINES_API_KEY")
		}
		if selected == "auto" {
			selected = "sso"
			if o.credentialSource == "NINES_API_KEY" || o.credentialSource == "API_TOKEN" {
				selected = "api-key"
			}
		}
		if selected != "api-key" && selected != "sso" && selected != "device" {
			return usage("method must be auto, api-key, sso or device")
		}
		if o.Context == "" {
			o.Context = o.Tenant
		}
		if !tenancy.ValidID(o.Context) {
			return usage("invalid context name")
		}
		p, e := o.configPath()
		if e != nil {
			return e
		}
		baseline, e := credentialSnapshot(p, o.Context)
		if e != nil {
			return e
		}
		var cred *credential
		savedSuccessfully := false
		defer func() {
			if cred != nil && !savedSuccessfully {
				cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				_ = authRequest(cleanup, o.httpClient(), cred.Server, cred.Tenant, cred.Token, "POST", "/auth/cli/logout", map[string]any{}, nil)
			}
		}()
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
			var v credential
			var e error
			if selected == "device" {
				v, e = deviceLogin(c.Context(), o, c.ErrOrStderr())
			} else {
				v, e = browserLogin(c.Context(), o, *noBrowser, c.ErrOrStderr())
			}
			if e != nil {
				return e
			}
			cred = &v
			o.Token = v.Token
			o.credentialSource = "browser-session"
		}
		v, e := o.authStatus(c.Context())
		var rejected *client.APIError
		if selected == "api-key" && errors.As(e, &rejected) && rejected.StatusCode == http.StatusUnauthorized {
			// Auto mode picked an environment key the user may not know is set.
			return &cliError{"unauthenticated", "The server rejected the API key in " + o.credentialSource + ". Unset NINES_API_KEY and API_TOKEN to sign in through the browser, or set a valid key", 3}
		}
		if e != nil {
			return e
		}
		var previous credential
		e = withConfigLock(p, func() error {
			current, err := credentialSnapshot(p, o.Context)
			if err != nil {
				return err
			}
			if current != baseline {
				return &cliError{"conflict", "Context or credential changed during login; retry using the current context", 5}
			}
			cfg, err := configLoad(p)
			if err != nil {
				return err
			}
			original, err := configLoad(p)
			if err != nil {
				return err
			}
			cfg.Contexts[o.Context] = savedContext{o.Server, o.Tenant}
			cfg.Current = o.Context
			if cred == nil {
				return configSave(p, cfg)
			}
			m, err := credentialsLoad(p)
			if err != nil {
				return err
			}
			previous = m[o.Context]
			entry, err := secureCredential(p, *cred, *storage)
			if err != nil {
				return err
			}
			committed := false
			defer func() {
				if !committed {
					_ = deleteStoredCredential(p, entry, false)
				}
			}()
			if err = configSave(p, cfg); err != nil {
				return err
			}
			m[o.Context] = entry
			if err = credentialsSave(p, m); err != nil {
				return errors.Join(err, configSave(p, original))
			}
			committed = true
			return nil
		})
		if e != nil {
			return e
		}
		savedSuccessfully = true
		if cred != nil {
			// Cleanup concerns must not turn a committed login into a failed one.
			if previous.Token != "" {
				cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
				_ = authRequest(cleanup, o.httpClient(), previous.Server, previous.Tenant, previous.Token, "POST", "/auth/cli/logout", map[string]any{}, nil)
				stop()
			}
			if err := deleteStoredCredential(p, previous, false); err != nil {
				v["warning"] = "Signed in; the previous OS credential could not be removed. Remove the stale nines.cli.v1 entry manually; its server credential expires normally."
			}
			v["credentialStore"] = *storage
			if selected == "device" {
				v["credentialSource"] = "device-session"
			}
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
		Actor   struct {
			ID string `json:"id"`
		} `json:"actor"`
	}
	e = authRequest(loginCtx, o.httpClient(), o.Server, "", "", "POST", "/auth/cli/exchange", map[string]string{"code": callback.code, "codeVerifier": verifier, "redirectURI": redirect}, &exchanged)
	if e != nil {
		return zero, e
	}
	if !strings.HasPrefix(exchanged.Token, "wp_cli_") || len(exchanged.Token) != 50 || exchanged.Tenant != o.Tenant || !exchanged.Expires.After(time.Now()) || exchanged.Expires.After(time.Now().Add(61*time.Minute)) {
		return zero, fmt.Errorf("invalid Nines CLI credential response")
	}
	return credential{Server: o.Server, Tenant: o.Tenant, Token: exchanged.Token, Expires: exchanged.Expires, Identity: exchanged.Actor.ID, Method: "browser"}, nil
}
