package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RemoteConfig is server-owned. No OAuth token, tool argument or client header
// can select the Nines endpoint, tenant, introspection service or API credential.
type RemoteConfig struct {
	APIVersion             string          `yaml:"apiVersion"`
	Kind                   string          `yaml:"kind"`
	PublicURL              string          `yaml:"publicURL"`
	Server                 string          `yaml:"server"`
	Tenant                 string          `yaml:"tenant"`
	Issuer                 string          `yaml:"issuer"`
	IntrospectionURL       string          `yaml:"introspectionURL"`
	IntrospectionClientID  string          `yaml:"introspectionClientID"`
	IntrospectionSecretEnv string          `yaml:"introspectionSecretEnv"`
	AllowedOrigins         []string        `yaml:"allowedOrigins,omitempty"`
	AllowWrites            bool            `yaml:"allowWrites,omitempty"`
	AllowRuleDeployment    bool            `yaml:"allowRuleDeployment,omitempty"`
	AllowRuleWithdrawal    bool            `yaml:"allowRuleWithdrawal,omitempty"`
	Bindings               []RemoteBinding `yaml:"bindings"`
	secret                 string
	// Tests inject a local-only introspection client. Production always uses a
	// bounded client with redirect refusal; this field is never configuration.
	http   *http.Client
	logger *slog.Logger
}
type RemoteBinding struct {
	Subject     string `yaml:"subject"`
	ClientID    string `yaml:"clientID"`
	APITokenEnv string `yaml:"apiTokenEnv"`
	token       string
}

var remoteEnv = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func LoadRemoteConfig(path string, lookup func(string) string) (RemoteConfig, error) {
	var c RemoteConfig
	file, e := os.Open(path)
	if e != nil {
		return c, errors.New("cannot read remote MCP configuration")
	}
	defer func() { _ = file.Close() }()
	data, e := io.ReadAll(io.LimitReader(file, (128<<10)+1))
	if e != nil {
		return c, errors.New("cannot read remote MCP configuration")
	}
	if len(data) > 128<<10 {
		return c, errors.New("remote MCP configuration exceeds 128 KiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, errors.New("remote MCP requires one strict YAML configuration")
	}
	if !remoteEnv.MatchString(c.IntrospectionSecretEnv) {
		return c, errors.New("introspectionSecretEnv must identify a private environment variable")
	}
	c.secret = lookup(c.IntrospectionSecretEnv)
	for i := range c.Bindings {
		if !remoteEnv.MatchString(c.Bindings[i].APITokenEnv) {
			return c, errors.New("apiTokenEnv must identify a private environment variable")
		}
		c.Bindings[i].token = lookup(c.Bindings[i].APITokenEnv)
	}
	return c, c.validate()
}
func remoteURL(raw string) (*url.URL, error) {
	if strings.ContainsAny(raw, " \t\r\n") {
		return nil, errors.New("remote MCP URLs must be canonical and contain no whitespace")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("remote MCP URLs require HTTPS without credentials, query or fragment")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, errors.New("HTTP is allowed only for loopback development")
		}
	}
	return u, nil
}
func remoteText(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for _, ch := range s {
		if ch < 33 || ch == 127 {
			return false
		}
	}
	return true
}
func (c RemoteConfig) validate() error {
	if c.APIVersion != "nines.dev/v1alpha1" || c.Kind != "RemoteMCP" {
		return errors.New("remote MCP requires nines.dev/v1alpha1 kind RemoteMCP")
	}
	public, e := remoteURL(c.PublicURL)
	if e != nil {
		return e
	}
	if public.Path == "" || public.Path == "/" || strings.HasSuffix(public.Path, "/") || strings.Contains(public.Path, "//") || strings.Contains(public.Path, "..") || strings.HasPrefix(public.Path, "/.well-known/") {
		return errors.New("publicURL must include one canonical MCP path without a trailing slash")
	}
	if _, e = remoteURL(c.Issuer); e != nil {
		return e
	}
	if _, e = remoteURL(c.IntrospectionURL); e != nil {
		return e
	}
	if !remoteText(c.IntrospectionClientID, 512) || !remoteText(c.secret, 8192) {
		return errors.New("configured introspection client credentials are required")
	}
	if _, e = newAPI(Config{Server: c.Server, Tenant: c.Tenant, AllowWrites: c.AllowWrites, AllowRuleDeployment: c.AllowRuleDeployment, AllowRuleWithdrawal: c.AllowRuleWithdrawal}); e != nil {
		return e
	}
	if len(c.Bindings) == 0 || len(c.Bindings) > 1000 || len(c.AllowedOrigins) > 32 {
		return errors.New("configure 1–1000 distinct client/user bindings and at most 32 origins")
	}
	seen := map[string]bool{}
	tokens := map[[32]byte]bool{}
	for _, b := range c.Bindings {
		if !remoteText(b.Subject, 512) || !remoteText(b.ClientID, 512) || !remoteText(b.token, 8192) {
			return errors.New("each remote binding requires a subject, clientID and private Nines API credential")
		}
		key := b.Subject + "\x00" + b.ClientID
		tokenHash := sha256.Sum256([]byte(b.token))
		if seen[key] || tokens[tokenHash] {
			return errors.New("remote bindings must have distinct client/user pairs and distinct Nines API credentials")
		}
		seen[key] = true
		tokens[tokenHash] = true
	}
	for _, origin := range c.AllowedOrigins {
		u, e := remoteURL(origin)
		if e != nil || u.Path != "" {
			return errors.New("allowedOrigins must contain exact HTTP(S) origins")
		}
	}
	return nil
}
