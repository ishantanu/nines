package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/examples"
)

func cliEnv(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "nines", "config.yaml")
	for _, name := range []string{"NINES_API_KEY", "API_TOKEN", "NINES_URL", "NINES_TENANT", "NINES_CONTEXT"} {
		t.Setenv(name, "")
	}
	t.Setenv("NINES_CONFIG", p)
	return p
}
func TestCobraHelpCompletionOfflineAndInterspersedFlags(t *testing.T) {
	cliEnv(t)
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(file, []byte(examples.APIAvailability), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINES_URL", "invalid-origin")
	var out bytes.Buffer
	for _, args := range [][]string{{"--offline", "--agent", "validate", "--file", file}, {"rules", "generate", "--offline", "--tenant", "alpha", "-f", file}, {"help", "intent", "conversations", "approve"}, {"completion", "zsh"}, {"--offline", "capabilities", "--agent"}, {"skills", "show", "nines-cli", "--offline", "--agent"}} {
		out.Reset()
		if e := run(args, &out); e != nil {
			t.Fatal(args, e)
		}
		if out.Len() == 0 {
			t.Fatal("empty result", args)
		}
	}
	for _, args := range [][]string{{"--offline", "get", "slos"}, {"validate", "-f", file, "--output", "csv"}, {"get", "slos", "--window", "1d"}, {"intent", "draft", "--binding", "ignored", "-f", file}} {
		if e := run(args, &bytes.Buffer{}); e == nil {
			t.Fatal("accepted invalid invocation", args)
		}
	}
	out.Reset()
	if e := run([]string{"--agent", "capabilities"}, &out); e != nil {
		t.Fatal(e)
	}
	var catalog map[string]any
	if json.Unmarshal(out.Bytes(), &catalog) != nil {
		t.Fatal(out.String())
	}
	if !strings.Contains(out.String(), `"effect": "deploy"`) || !strings.Contains(out.String(), `"command": "nines auth status"`) {
		t.Fatal("missing capability boundaries")
	}
}
func TestAPIKeyLoginAndContextCredentialsNeverCrossServerOrTenant(t *testing.T) {
	p := cliEnv(t)
	var seen []string
	var mu sync.Mutex
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization")+"/"+r.Header.Get("X-Nines-Tenant"))
		mu.Unlock()
		_, _ = fmt.Fprint(w, `[{"id":"alpha","name":"Alpha","role":"viewer"}]`)
	}))
	defer api.Close()
	t.Setenv("NINES_API_KEY", "primary-key")
	t.Setenv("API_TOKEN", "legacy-key")
	var out bytes.Buffer
	if e := run([]string{"login", "--method", "api-key", "--context", "prod", "--server", api.URL, "--tenant", "alpha", "--agent"}, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "primary-key") {
		t.Fatal("key printed")
	}
	b, _ := os.ReadFile(p)
	if bytes.Contains(b, []byte("primary-key")) {
		t.Fatal("key persisted")
	}
	if _, e := os.Stat(p + ".credentials.json"); !os.IsNotExist(e) {
		t.Fatal("API-key login created credentials")
	}
	mu.Lock()
	if seen[0] != "Bearer primary-key/alpha" {
		t.Fatal(seen)
	}
	mu.Unlock()
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	// A stored browser token must match both the exact server and tenant.
	t.Setenv("NINES_API_KEY", "")
	t.Setenv("API_TOKEN", "")
	e := withConfigLock(p, func() error {
		return credentialsSave(p, map[string]credential{"prod": {api.URL, "alpha", "stored-browser-key", time.Now().Add(time.Hour)}})
	})
	if e != nil {
		t.Fatal(e)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credential sent to another origin")
		}
		_, _ = fmt.Fprint(w, `[]`)
	}))
	defer other.Close()
	if e := run([]string{"get", "slos", "--server", other.URL, "--tenant", "alpha", "--agent"}, &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"get", "slos", "--tenant", "beta", "--agent"}, &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	last := seen[len(seen)-1]
	mu.Unlock()
	if last != "/beta" {
		t.Fatal("credential crossed tenant", last)
	}
	t.Setenv("NINES_API_KEY", "env-override")
	if e := run([]string{"get", "slos", "--tenant", "beta", "--agent"}, &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	last = seen[len(seen)-1]
	mu.Unlock()
	if last != "Bearer env-override/beta" {
		t.Fatal(last)
	}
}
func TestOfflineValidationDoesNotReadCredentialsAndErrorsAreStructured(t *testing.T) {
	p := cliEnv(t)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("broken: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".credentials.json", []byte("do-not-read"), 0644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(file, []byte(examples.APIAvailability), 0600); err != nil {
		t.Fatal(err)
	}
	if e := run([]string{"validate", "--offline", "-f", file, "--agent"}, &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	var stderr bytes.Buffer
	writeError(&stderr, &cliError{"conflict", "refresh the reviewed digest", 5}, true)
	var v map[string]any
	if json.Unmarshal(stderr.Bytes(), &v) != nil || !strings.Contains(stderr.String(), `"exitCode":5`) {
		t.Fatal(stderr.String())
	}
}
func TestBrowserLoginCallbackPKCECredentialPersistenceAndAgentBoundary(t *testing.T) {
	p := cliEnv(t)
	var lock sync.Mutex
	var start map[string]string
	var exchanged atomic.Bool
	var api *httptest.Server
	api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/cli/start":
			var in map[string]string
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
				return
			}
			lock.Lock()
			start = in
			lock.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"authorizationURL": api.URL + "/auth/cli/authorize?request=" + strings.Repeat("a", 43), "expiresAt": time.Now().Add(time.Minute)})
		case "/auth/cli/exchange":
			var in map[string]string
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
				return
			}
			sum := sha256.Sum256([]byte(in["codeVerifier"]))
			lock.Lock()
			expected := start["challenge"]
			redirect := start["redirectURI"]
			lock.Unlock()
			if base64.RawURLEncoding.EncodeToString(sum[:]) != expected || in["redirectURI"] != redirect || in["code"] != strings.Repeat("c", 43) {
				t.Error("PKCE exchange not bound")
			}
			exchanged.Store(true)
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "wp_cli_" + strings.Repeat("t", 43), "tenant": "alpha", "expiresAt": time.Now().Add(50 * time.Minute)})
		case "/api/tenants":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer wp_cli_") {
				t.Error("missing CLI token")
			}
			_, _ = fmt.Fprint(w, `[{"id":"alpha","role":"editor"}]`)
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	prior := launchBrowser
	t.Cleanup(func() { launchBrowser = prior })
	launchBrowser = func(raw string) error {
		lock.Lock()
		callback := start["redirectURI"]
		state := start["state"]
		lock.Unlock()
		bad, e := http.Get(callback + "?state=wrong&code=" + strings.Repeat("c", 43))
		if e != nil {
			return e
		}
		_ = bad.Body.Close()
		if bad.StatusCode != 400 {
			t.Error("invalid state accepted")
		}
		request, _ := http.NewRequest("GET", callback+"?state="+url.QueryEscape(state)+"&code="+strings.Repeat("c", 43), nil)
		request.Header.Set("Origin", "null") // Browser POST-redirect navigation may retain an opaque origin.
		res, e := http.DefaultClient.Do(request)
		if e != nil {
			return e
		}
		defer func() {
			_ = res.Body.Close()
		}()
		_, e = io.Copy(io.Discard, res.Body)
		return e
	}
	var out, stderr bytes.Buffer
	if e := execute(context.Background(), []string{"login", "--method", "sso", "--context", "team", "--tenant", "alpha", "--server", api.URL}, &out, &stderr); e != nil {
		t.Fatal(e)
	}
	if !exchanged.Load() || strings.Contains(out.String(), "wp_cli_") {
		t.Fatal("credential missing or disclosed")
	}
	m, e := credentialsLoad(p)
	if e != nil || m["team"].Tenant != "alpha" {
		t.Fatal(m, e)
	}
	out.Reset()
	if e = run([]string{"auth", "status", "--context", "team", "--agent"}, &out); e != nil || !strings.Contains(out.String(), `"role": "editor"`) {
		t.Fatal(e, out.String())
	}
	if e = run([]string{"login", "--method", "sso", "--agent", "--context", "team"}, &bytes.Buffer{}); e == nil {
		t.Fatal("agent launched browser")
	}
}
func TestConfigLockAndUnsafePaths(t *testing.T) {
	p := cliEnv(t)
	if e := withConfigLock(p, func() error { return withConfigLock(p, func() error { return nil }) }); e == nil || exitCode(e) != 5 {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
	if e := withConfigLock(p, func() error { return privateWrite(p, []byte("overwritten")) }); e == nil {
		t.Fatal("symlink followed")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "original" {
		t.Fatal("symlink target modified")
	}
}

func TestMachineOutputPreservesLargeRevisionAndYAMLFieldNames(t *testing.T) {
	cliEnv(t)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"revision":9007199254740993,"apiVersion":"nines.dev/v1alpha1"}`)
	}))
	defer s.Close()
	var out bytes.Buffer
	if e := run([]string{"status", "checkout", "--server", s.URL, "--agent"}, &out); e != nil || !strings.Contains(out.String(), "9007199254740993") {
		t.Fatal(e, out.String())
	}
	out.Reset()
	if e := run([]string{"status", "checkout", "--server", s.URL, "-o", "yaml"}, &out); e != nil || !strings.Contains(out.String(), "9007199254740993") {
		t.Fatal(e, out.String())
	}
	out.Reset()
	if e := render(&out, struct {
		Version string `json:"apiVersion"`
	}{"nines.dev/v1alpha1"}, "yaml", "test"); e != nil || !strings.Contains(out.String(), "apiVersion:") {
		t.Fatal(e, out.String())
	}
	if e := render(io.Discard, []any{map[string]any{"slo": "malformed"}}, "table", "slos"); e == nil {
		t.Fatal("malformed server table accepted")
	}
}

func TestAgentArtifactReceiptAndExclusiveWrite(t *testing.T) {
	cliEnv(t)
	path := filepath.Join(t.TempDir(), "generated.yaml")
	var out bytes.Buffer
	if e := run([]string{"init", "slo", "--name", "checkout", "--offline", "--agent", "--out", path}, &out); e != nil {
		t.Fatal(e)
	}
	var receipt map[string]any
	if e := json.Unmarshal(out.Bytes(), &receipt); e != nil || receipt["kind"] != "CLIArtifact" || receipt["path"] != path {
		t.Fatal(e, out.String())
	}
	before, _ := os.ReadFile(path)
	out.Reset()
	if e := run([]string{"init", "slo", "--name", "changed", "--offline", "--agent", "--out", path}, &out); e == nil {
		t.Fatal("overwrote existing artifact")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) || out.Len() != 0 {
		t.Fatal("failed artifact write changed output")
	}
}

func TestLogoutRevokesSavedSessionEvenWhenEnvironmentKeyIsSet(t *testing.T) {
	p := cliEnv(t)
	var revoked atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/cli/logout" || r.Header.Get("Authorization") != "Bearer saved-session" {
			t.Error("revoked wrong credential", r.URL.Path)
		}
		revoked.Store(true)
		_, _ = fmt.Fprint(w, `{"status":"signed_out"}`)
	}))
	defer s.Close()
	e := withConfigLock(p, func() error {
		c := savedConfig{APIVersion: "nines.dev/v1alpha1", Kind: "CLIConfig", Current: "prod", Contexts: map[string]savedContext{"prod": {s.URL, "alpha"}}}
		if e := configSave(p, c); e != nil {
			return e
		}
		return credentialsSave(p, map[string]credential{"prod": {s.URL, "alpha", "saved-session", time.Now().Add(time.Hour)}})
	})
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("NINES_API_KEY", "environment-key")
	if e = run([]string{"auth", "logout", "--agent"}, &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	if !revoked.Load() {
		t.Fatal("saved session not revoked")
	}
	m, e := credentialsLoad(p)
	if e != nil || len(m) != 0 {
		t.Fatal(m, e)
	}
}

func TestOfflineIntentValidationUsesSharedSchema(t *testing.T) {
	cliEnv(t)
	t.Setenv("NINES_TENANT", "invalid tenant ignored for local validation")
	f := filepath.Join("..", "..", "examples", "api-success.intent.yaml")
	var out bytes.Buffer
	if e := run([]string{"validate", "--offline", "--agent", "-f", f}, &out); e != nil || !strings.Contains(out.String(), "ServiceLevelObjectiveIntent") {
		t.Fatal(e, out.String())
	}
	if e := run([]string{"validate", "--offline", "-f", f, "--target", "0.99"}, &bytes.Buffer{}); e == nil {
		t.Fatal("intent silently ignored an SLO override")
	}
}

func TestIntentAuditReadAndRollbackExportDoNotRequireAFile(t *testing.T) {
	cliEnv(t)
	t.Setenv("NINES_API_KEY", "fixture-key")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/intent-conversations/chat/audit" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("unscoped audit read", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","conversation":"chat","integrity":"verified","events":[],"changeSets":[]}`)
	}))
	defer server.Close()
	var out bytes.Buffer
	if e := run([]string{"intent", "conversations", "audit", "chat", "--server", server.URL, "--tenant", "alpha", "--agent"}, &out); e != nil {
		t.Fatal(e, out.String())
	}
	if calls.Load() != 1 || !strings.Contains(out.String(), `"integrity": "verified"`) {
		t.Fatal(out.String(), calls.Load())
	}
	for _, action := range []string{"audit", "rollback-kubernetes"} {
		out.Reset()
		if e := run([]string{"intent", "conversations", action, "--help"}, &out); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(out.String(), "--file") {
			t.Fatal("read/export inherited required write file", action, out.String())
		}
	}
}
