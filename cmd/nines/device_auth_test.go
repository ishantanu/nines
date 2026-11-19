package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type memoryKeychain struct {
	mu    sync.Mutex
	data  map[string]string
	calls int
	fail  bool
}

func (k *memoryKeychain) Get(s, a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls++
	if k.fail {
		return "", errors.New("fixture failure")
	}
	v, ok := k.data[s+a]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}
func (k *memoryKeychain) Set(s, a, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls++
	if k.fail {
		return errors.New("fixture failure")
	}
	k.data[s+a] = v
	return nil
}
func (k *memoryKeychain) Delete(s, a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls++
	if k.fail {
		return errors.New("fixture failure")
	}
	delete(k.data, s+a)
	return nil
}
func fakeKeychain(t *testing.T) *memoryKeychain {
	t.Helper()
	old := osCredentials
	k := &memoryKeychain{data: map[string]string{}}
	osCredentials = k
	t.Cleanup(func() { osCredentials = old })
	return k
}
func TestKeychainIdentityBindingAndNoSilentFallback(t *testing.T) {
	p := cliEnv(t)
	k := fakeKeychain(t)
	c := credential{Server: "https://nines.example", Tenant: "alpha", Identity: "alice", Token: "private-fixture-token", Expires: time.Now().Add(time.Hour)}
	entry, err := secureCredential(p, c, "keychain")
	if err != nil || entry.Token != "" {
		t.Fatal("secret not separated", err)
	}
	value, err := storedToken(p, entry, false)
	if err != nil || value != c.Token {
		t.Fatal("keychain roundtrip", err)
	}
	for _, change := range []func(*credential){func(c *credential) { c.Server = "https://another.example" }, func(c *credential) { c.Tenant = "beta" }, func(c *credential) { c.Identity = "bob" }} {
		altered := entry
		change(&altered)
		if _, err = storedToken(p, altered, false); err == nil {
			t.Fatal("credential crossed identity scope")
		}
	}
	before := k.calls
	if _, err = storedToken(p, entry, true); exitCode(err) != 3 || k.calls != before {
		t.Fatal("agent accessed OS credentials", err)
	}
	k.fail = true
	if _, err = secureCredential(p, c, "keychain"); exitCode(err) != 6 {
		t.Fatal("missing explicit store failure", err)
	}
	if _, err = os.Stat(p + ".credentials.json"); !os.IsNotExist(err) {
		t.Fatal("keychain failure silently wrote credentials")
	}
}
func deviceFixtureServer(t *testing.T, tokenStatus string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var server *httptest.Server
	var cancel atomic.Int64
	var mu sync.Mutex
	var challenge string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/cli/capabilities":
			_ = json.NewEncoder(w).Encode(v1.CLIAuthenticationCapabilities{APIVersion: "nines.dev/v1alpha1", Kind: "CLIAuthenticationCapabilities", Device: true, Browser: true, APIKey: true, SessionMaxSeconds: 3600})
		case "/auth/cli/device/start":
			var in v1.CLIDeviceStart
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Tenant != "alpha" {
				t.Error("wrong tenant")
			}
			mu.Lock()
			challenge = in.Challenge
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(v1.CLIDeviceAuthorization{DeviceCode: strings.Repeat("d", 43), UserCode: "ABCD-EFGH", VerificationURI: server.URL + "/auth/cli/authorize?request=" + strings.Repeat("r", 43), ExpiresAt: time.Now().Add(5 * time.Minute), IntervalSeconds: 1})
		case "/auth/cli/device/token":
			var in v1.CLIDevicePoll
			_ = json.NewDecoder(r.Body).Decode(&in)
			sum := sha256.Sum256([]byte(in.CodeVerifier))
			mu.Lock()
			expected := challenge
			mu.Unlock()
			if expected != base64.RawURLEncoding.EncodeToString(sum[:]) || in.DeviceCode != strings.Repeat("d", 43) {
				t.Error("verifier binding lost")
			}
			result := v1.CLIDeviceResult{Status: tokenStatus, IntervalSeconds: 1}
			if tokenStatus == "authorized" {
				result.Token = "wp_cli_" + strings.Repeat("t", 43)
				result.Tenant = "alpha"
				result.Identity = "fixture-alice"
				result.ExpiresAt = time.Now().Add(time.Hour)
			}
			_ = json.NewEncoder(w).Encode(result)
		case "/auth/cli/device/cancel":
			cancel.Add(1)
			_, _ = fmt.Fprint(w, `{"status":"cancelled"}`)
		case "/api/tenants":
			_, _ = fmt.Fprint(w, `[{"id":"alpha","name":"Alpha","role":"viewer"}]`)
		case "/auth/cli/logout":
			_, _ = fmt.Fprint(w, `{"status":"signed_out"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &cancel
}
func TestDeviceLoginStoresOnlyNinesIdentityAndLogout(t *testing.T) {
	p := cliEnv(t)
	fakeKeychain(t)
	server, cancels := deviceFixtureServer(t, "authorized")
	old := launchBrowser
	launchBrowser = func(string) error { t.Error("device login launched browser"); return nil }
	defer func() { launchBrowser = old }()
	var out, diagnostic bytes.Buffer
	err := execute(context.Background(), []string{"login", "--method", "device", "--server", server.URL, "--tenant", "alpha", "--context", "prod"}, &out, &diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := credentialsLoad(p)
	if err != nil {
		t.Fatal(err)
	}
	saved := entries["prod"]
	if saved.Identity != "fixture-alice" || saved.Method != "device" || saved.Tenant != "alpha" || saved.Token == "" {
		t.Fatal("missing identity scope")
	}
	if strings.Contains(out.String()+diagnostic.String(), saved.Token) || strings.Contains(diagnostic.String(), strings.Repeat("d", 43)) {
		t.Fatal("secret in output")
	}
	if cancels.Load() != 0 {
		t.Fatal("successful grant cancelled")
	}
	if err = execute(context.Background(), []string{"auth", "logout", "--context", "prod"}, &out, &diagnostic); err != nil {
		t.Fatal(err)
	}
	entries, _ = credentialsLoad(p)
	if len(entries) != 0 {
		t.Fatal("logout retained credential")
	}
}
func TestDeviceLoginDenialCancellationAndAgentRestrictions(t *testing.T) {
	for _, scenario := range []string{"access_denied", "timeout", "agent"} {
		t.Run(scenario, func(t *testing.T) {
			cliEnv(t)
			k := fakeKeychain(t)
			server, cancels := deviceFixtureServer(t, "access_denied")
			args := []string{"login", "--method", "device", "--server", server.URL, "--tenant", "alpha", "--context", "prod"}
			ctx := context.Background()
			if scenario == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			if scenario == "agent" {
				args = append(args, "--agent")
			}
			var out bytes.Buffer
			err := execute(ctx, args, &out, &out)
			if err == nil {
				t.Fatal("inactive login succeeded")
			}
			if scenario == "agent" {
				if exitCode(err) != 2 || cancels.Load() != 0 || k.calls != 0 {
					t.Fatal("agent initiated interactive authentication", err)
				}
			} else if exitCode(err) != 3 || cancels.Load() != 1 {
				t.Fatal("grant not cancelled", err, cancels.Load())
			}
		})
	}
}
func TestKeychainAgentDiagnosticsAndEnvironmentPrecedence(t *testing.T) {
	p := cliEnv(t)
	k := fakeKeychain(t)
	server, _ := deviceFixtureServer(t, "authorized")
	c := credential{Server: server.URL, Tenant: "alpha", Identity: "alice", Token: "private-keychain-token", Expires: time.Now().Add(time.Hour)}
	entry, err := secureCredential(p, c, "keychain")
	if err != nil {
		t.Fatal(err)
	}
	if err = withConfigLock(p, func() error {
		if e := configSave(p, savedConfig{APIVersion: "nines.dev/v1alpha1", Kind: "CLIConfig", Current: "prod", Contexts: map[string]savedContext{"prod": {server.URL, "alpha"}}}); e != nil {
			return e
		}
		return credentialsSave(p, map[string]credential{"prod": entry})
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p + ".credentials.json")
	if bytes.Contains(raw, []byte(c.Token)) {
		t.Fatal("keychain secret in index")
	}
	before := k.calls
	var out bytes.Buffer
	if err = execute(context.Background(), []string{"auth", "status", "--agent"}, &out, &out); exitCode(err) != 3 {
		t.Fatal("agent read keychain", err)
	}
	if err = execute(context.Background(), []string{"auth", "diagnose", "--agent"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if k.calls != before {
		t.Fatal("diagnostics unlocked keychain")
	}
	t.Setenv("NINES_API_KEY", "fixture-env-api-key")
	if err = execute(context.Background(), []string{"auth", "status", "--agent"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if k.calls != before || strings.Contains(out.String(), c.Token) || strings.Contains(out.String(), "fixture-env-api-key") {
		t.Fatal("environment key precedence or redaction failed")
	}
}

func TestConcurrentDeviceLoginsDoNotOverwriteContext(t *testing.T) {
	p := cliEnv(t)
	var server *httptest.Server
	var starts, polls, revoked atomic.Int64
	both := make(chan struct{})
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/cli/capabilities":
			_ = json.NewEncoder(w).Encode(v1.CLIAuthenticationCapabilities{APIVersion: "nines.dev/v1alpha1", Kind: "CLIAuthenticationCapabilities", Device: true, APIKey: true})
		case "/auth/cli/device/start":
			id := starts.Add(1)
			_ = json.NewEncoder(w).Encode(v1.CLIDeviceAuthorization{DeviceCode: fmt.Sprintf("%043d", id), UserCode: "ABCD-EFGH", VerificationURI: server.URL + "/auth/cli/authorize?request=" + strings.Repeat("r", 43), ExpiresAt: time.Now().Add(time.Minute), IntervalSeconds: 1})
		case "/auth/cli/device/token":
			var in v1.CLIDevicePoll
			_ = json.NewDecoder(r.Body).Decode(&in)
			if polls.Add(1) == 2 {
				close(both)
			}
			select {
			case <-both:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(v1.CLIDeviceResult{Status: "authorized", Token: "wp_cli_" + in.DeviceCode, Tenant: "alpha", Identity: "alice", ExpiresAt: time.Now().Add(time.Hour)})
		case "/api/tenants":
			_, _ = fmt.Fprint(w, `[{"id":"alpha","role":"viewer"}]`)
		case "/auth/cli/logout":
			revoked.Add(1)
			_, _ = fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			var out bytes.Buffer
			results <- execute(ctx, []string{"login", "--method", "device", "--server", server.URL, "--tenant", "alpha", "--context", "prod"}, &out, &out)
		}()
	}
	a, b := <-results, <-results
	if a != nil && b != nil || a == nil && b == nil {
		t.Fatal("expected one accepted login and one context conflict", a, b)
	}
	if a != nil && exitCode(a) != 5 || b != nil && exitCode(b) != 5 {
		t.Fatal("lost concurrent-login conflict", a, b)
	}
	entries, err := credentialsLoad(p)
	if err != nil || len(entries) != 1 || entries["prod"].Token == "" {
		t.Fatal("missing accepted credential", err)
	}
	if revoked.Load() != 1 {
		t.Fatal("rejected login credential was not revoked", revoked.Load())
	}
}

func TestDeviceCapabilityFallbackDoesNotStartAuthentication(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			cliEnv(t)
			keychain := fakeKeychain(t)
			var writes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes.Add(1)
				}
				if legacy {
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(v1.CLIAuthenticationCapabilities{APIVersion: "nines.dev/v1alpha1", Kind: "CLIAuthenticationCapabilities", APIKey: true})
			}))
			defer server.Close()
			var out bytes.Buffer
			err := execute(context.Background(), []string{"login", "--method", "device", "--server", server.URL, "--tenant", "alpha"}, &out, &out)
			if exitCode(err) != 6 || !strings.Contains(err.Error(), "NINES_API_KEY") {
				t.Fatal("missing explicit API-key fallback", err)
			}
			if writes.Load() != 0 || keychain.calls != 0 {
				t.Fatal("unsupported discovery caused a side effect")
			}
		})
	}
}

func TestDeviceLoginRejectsUntrustedVerificationOrigin(t *testing.T) {
	cliEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/cli/capabilities" {
			_ = json.NewEncoder(w).Encode(v1.CLIAuthenticationCapabilities{APIVersion: "nines.dev/v1alpha1", Kind: "CLIAuthenticationCapabilities", Device: true})
			return
		}
		_ = json.NewEncoder(w).Encode(v1.CLIDeviceAuthorization{DeviceCode: strings.Repeat("d", 43), UserCode: "ABCD-EFGH", VerificationURI: "https://untrusted.example/auth/cli/authorize?request=" + strings.Repeat("r", 43), ExpiresAt: time.Now().Add(time.Minute), IntervalSeconds: 1})
	}))
	defer server.Close()
	var out bytes.Buffer
	err := execute(context.Background(), []string{"login", "--method", "device", "--server", server.URL, "--tenant", "alpha"}, &out, &out)
	if err == nil || exitCode(err) != 2 || strings.Contains(out.String(), "untrusted.example") {
		t.Fatal("untrusted approval URL was presented", err)
	}
}
