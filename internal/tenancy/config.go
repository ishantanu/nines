// Package tenancy loads server-owned tenant routing and credential grants.
// Tenant selection is never inferred from caller-supplied SLO/query content.
package tenancy

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/intentmodel"
)

const Header = "X-Nines-Tenant"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var envPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var mimirPattern = regexp.MustCompile(`^[a-zA-Z0-9_!.*'()-]{1,150}$`)

func ValidID(id string) bool      { return idPattern.MatchString(id) }
func ValidMimirID(id string) bool { return mimirPattern.MatchString(id) && id != "." && id != ".." }

type Config struct {
	Tenants []Tenant `json:"tenants" yaml:"tenants"`
}
type IntentModel = intentmodel.Config

type Tenant struct {
	ReviewModel     *IntentModel `json:"reviewModel,omitempty" yaml:"reviewModel,omitempty"`
	IntentModel     *IntentModel `json:"intentModel,omitempty" yaml:"intentModel,omitempty"`
	Connections     []Connection `json:"connections,omitempty" yaml:"connections,omitempty"`
	ID              string       `json:"id" yaml:"id"`
	Name            string       `json:"name" yaml:"name"`
	Access          []Access     `json:"access" yaml:"access"`
	Source          Source       `json:"source" yaml:"source"`
	AlertWebhookEnv string       `json:"alertWebhookEnv,omitempty" yaml:"alertWebhookEnv,omitempty"`
}
type Access struct {
	TokenEnv string `json:"tokenEnv" yaml:"tokenEnv"`
	Role     string `json:"role" yaml:"role"`
}
type RuleDestination struct {
	Mode             string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Publisher        string `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	Reload           bool   `json:"reload,omitempty" yaml:"reload,omitempty"`
	Namespace        string `json:"namespace" yaml:"namespace"`
	RulerURL         string `json:"rulerURL,omitempty" yaml:"rulerURL,omitempty"`
	Directory        string `json:"directory,omitempty" yaml:"directory,omitempty"`
	BearerTokenEnv   string `json:"bearerTokenEnv,omitempty" yaml:"bearerTokenEnv,omitempty"`
	BasicUsername    string `json:"basicUsername,omitempty" yaml:"basicUsername,omitempty"`
	BasicPasswordEnv string `json:"basicPasswordEnv,omitempty" yaml:"basicPasswordEnv,omitempty"`
}
type Connection struct {
	Name   string `json:"name" yaml:"name"`
	Source `yaml:",inline"`
}
type ResolvedConnection struct {
	Config             Connection
	BearerToken        string `json:"-" yaml:"-"`
	BasicPassword      string `json:"-" yaml:"-"`
	RulesBearerToken   string `json:"-" yaml:"-"`
	RulesBasicPassword string `json:"-" yaml:"-"`
}
type Source struct {
	Rules *RuleDestination `json:"rules,omitempty" yaml:"rules,omitempty"`

	Kind             string `json:"kind" yaml:"kind"`
	URL              string `json:"url,omitempty" yaml:"url,omitempty"`
	MimirOrgID       string `json:"mimirOrgID,omitempty" yaml:"mimirOrgID,omitempty"`
	BearerTokenEnv   string `json:"bearerTokenEnv,omitempty" yaml:"bearerTokenEnv,omitempty"`
	BasicUsername    string `json:"basicUsername,omitempty" yaml:"basicUsername,omitempty"`
	BasicPasswordEnv string `json:"basicPasswordEnv,omitempty" yaml:"basicPasswordEnv,omitempty"`
}

// Resolved values are intentionally excluded from JSON/YAML serialization.
type Resolved struct {
	IntentModelKey     string `json:"-" yaml:"-"`
	Connections        []ResolvedConnection
	RulesBearerToken   string `json:"-" yaml:"-"`
	RulesBasicPassword string `json:"-" yaml:"-"`
	Config             Tenant
	Grants             []Grant `json:"-" yaml:"-"`
	BearerToken        string  `json:"-" yaml:"-"`
	BasicPassword      string  `json:"-" yaml:"-"`
	Webhook            string  `json:"-" yaml:"-"`
}
type Grant struct {
	Token string `json:"-" yaml:"-"`
	Role  string `json:"role" yaml:"role"`
}

func Load(path string, lookup func(string) string) ([]Resolved, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	return Parse(b, lookup)
}
func Parse(b []byte, lookup func(string) string) ([]Resolved, error) { return parse(b, lookup, false) }
func LoadForIdentity(path string, lookup func(string) string) ([]Resolved, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(b, lookup, true)
}
func parse(b []byte, lookup func(string) string, identityEnabled bool) ([]Resolved, error) {
	if len(b) == 0 || len(b) > 1<<20 {
		return nil, fmt.Errorf("tenant configuration must contain 1–1048576 bytes")
	}
	var c Config
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("invalid tenant configuration: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("tenant configuration must contain exactly one document")
	}
	if len(c.Tenants) < 1 || len(c.Tenants) > 100 {
		return nil, fmt.Errorf("configure 1–100 tenants")
	}
	ids := map[string]bool{}
	out := make([]Resolved, 0, len(c.Tenants))
	secret := func(env string, required bool) (string, error) {
		if env == "" && !required {
			return "", nil
		}
		if !envPattern.MatchString(env) {
			return "", fmt.Errorf("invalid credential environment variable reference")
		}
		v := lookup(env)
		if v == "" {
			return "", fmt.Errorf("required environment variable %s is empty", env)
		}
		if strings.ContainsAny(v, "\r\n") {
			return "", fmt.Errorf("credential %s must not contain line breaks", env)
		}
		return v, nil
	}
	for _, t := range c.Tenants {
		if !ValidID(t.ID) || ids[t.ID] || strings.TrimSpace(t.Name) == "" || len(t.Name) > 200 {
			return nil, fmt.Errorf("tenants need unique valid IDs and names of at most 200 bytes")
		}
		ids[t.ID] = true
		if len(t.Access) < 1 && !identityEnabled || len(t.Access) > 20 {
			return nil, fmt.Errorf("tenant %s needs 1–20 token grants", t.ID)
		}
		r := Resolved{Config: t}
		if t.IntentModel != nil {
			key, err := secret(t.IntentModel.APIKeyEnv, false)
			if err != nil {
				return nil, err
			}
			r.IntentModelKey = key
		}
		for _, a := range t.Access {
			if a.Role != "owner" && a.Role != "editor" && a.Role != "agent" && a.Role != "viewer" {
				return nil, fmt.Errorf("tenant %s role must be owner, editor, agent or viewer", t.ID)
			}
			token, err := secret(a.TokenEnv, true)
			if err != nil {
				return nil, err
			}
			if len(token) < 24 {
				return nil, fmt.Errorf("tenant %s API credentials must contain at least 24 characters", t.ID)
			}
			r.Grants = append(r.Grants, Grant{Token: token, Role: a.Role})
		}
		s := t.Source
		if s.Kind == "" {
			s.Kind = "manual"
			r.Config.Source.Kind = s.Kind
		}
		if s.Kind != "manual" && s.Kind != "prometheus" && s.Kind != "mimir" {
			return nil, fmt.Errorf("tenant %s source must be manual, prometheus or mimir", t.ID)
		}
		if s.Kind == "manual" {
			if s.Rules != nil || s.URL != "" || s.MimirOrgID != "" || s.BearerTokenEnv != "" || s.BasicUsername != "" || s.BasicPasswordEnv != "" {
				return nil, fmt.Errorf("manual source cannot contain upstream configuration")
			}
		} else {
			u, err := url.Parse(s.URL)
			if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
				return nil, fmt.Errorf("tenant %s has invalid source URL", t.ID)
			}
			if s.Kind == "mimir" && !ValidMimirID(s.MimirOrgID) {
				return nil, fmt.Errorf("tenant %s needs one valid Mimir org ID; federation is disabled", t.ID)
			}
			if s.Kind != "mimir" && s.MimirOrgID != "" {
				return nil, fmt.Errorf("mimir org ID is allowed only on mimir sources")
			}
			if (s.BasicUsername == "") != (s.BasicPasswordEnv == "") || strings.ContainsAny(s.BasicUsername, "\r\n:") || len(s.BasicUsername) > 200 {
				return nil, fmt.Errorf("configure both a basic username and password environment reference")
			}
			if s.BearerTokenEnv != "" && s.BasicUsername != "" {
				return nil, fmt.Errorf("choose bearer or basic authentication, not both")
			}
			r.BearerToken, err = secret(s.BearerTokenEnv, false)
			if err != nil {
				return nil, err
			}
			r.BasicPassword, err = secret(s.BasicPasswordEnv, false)
			if err != nil {
				return nil, err
			}
		}
		if rules := s.Rules; rules != nil {
			if !ValidID(rules.Namespace) {
				return nil, fmt.Errorf("rules require a valid namespace")
			}
			if rules.Mode != "" && rules.Mode != "operator" {
				return nil, fmt.Errorf("unsupported rule deployment mode")
			}
			if rules.Mode == "operator" {
				if !ValidID(rules.Publisher) || rules.Directory != "" || rules.RulerURL != "" || rules.Reload || rules.BearerTokenEnv != "" || rules.BasicUsername != "" || rules.BasicPasswordEnv != "" {
					return nil, fmt.Errorf("operator rules require a publisher and Kubernetes namespace, without file/ruler/reload credentials")
				}
			} else if rules.Publisher != "" {
				return nil, fmt.Errorf("publisher requires operator mode")
			}
			switch {
			case rules.Mode == "operator":
			case s.Kind == "mimir":
				if rules.Reload {
					return nil, fmt.Errorf("reload is supported only for Prometheus rule files")
				}
				u, err := url.Parse(rules.RulerURL)
				if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || rules.Directory != "" {
					return nil, fmt.Errorf("mimir rules require an explicit rulerURL with Prometheus prefix, and no directory")
				}
			case s.Kind == "prometheus":
				if !filepath.IsAbs(rules.Directory) || rules.RulerURL != "" {
					return nil, fmt.Errorf("prometheus rules require an absolute directory, not a rulerURL")
				}
			}
			if (rules.BasicUsername == "") != (rules.BasicPasswordEnv == "") || strings.ContainsAny(rules.BasicUsername, "\r\n:") || rules.BasicUsername != "" && rules.BearerTokenEnv != "" {
				return nil, fmt.Errorf("invalid ruler authentication")
			}
			var err error
			r.RulesBearerToken, err = secret(rules.BearerTokenEnv, false)
			if err != nil {
				return nil, err
			}
			r.RulesBasicPassword, err = secret(rules.BasicPasswordEnv, false)
			if err != nil {
				return nil, err
			}
		}
		if len(t.Connections) > 20 {
			return nil, fmt.Errorf("configure at most 20 named connections per tenant")
		}
		names := map[string]bool{"primary": true}
		for _, connection := range t.Connections {
			if !ValidID(connection.Name) || names[connection.Name] {
				return nil, fmt.Errorf("connections require unique valid names; primary is reserved for the legacy source")
			}
			names[connection.Name] = true
			// Reuse exactly the same strict validation and secret resolution for each
			// named source. The temporary tenant has no child connections.
			cfg := Config{Tenants: []Tenant{{ID: t.ID, Name: t.Name, Access: t.Access, Source: connection.Source}}}
			b, err := yaml.Marshal(cfg)
			if err != nil {
				return nil, err
			}
			nested, err := parse(b, lookup, identityEnabled)
			if err != nil {
				return nil, fmt.Errorf("connection %s: %w", connection.Name, err)
			}
			resolved := nested[0]
			connection.Source = resolved.Config.Source
			r.Connections = append(r.Connections, ResolvedConnection{Config: connection, BearerToken: resolved.BearerToken, BasicPassword: resolved.BasicPassword, RulesBearerToken: resolved.RulesBearerToken, RulesBasicPassword: resolved.RulesBasicPassword})
		}

		var err error
		r.Webhook, err = secret(t.AlertWebhookEnv, false)
		if err != nil {
			return nil, err
		}
		if r.Webhook != "" {
			u, err := url.Parse(r.Webhook)
			if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return nil, fmt.Errorf("tenant %s has invalid alert webhook URL", t.ID)
			}
		}
		out = append(out, r)
	}
	namespaces := map[string]string{}
	for _, resolved := range out {
		sources := []Source{resolved.Config.Source}
		for _, connection := range resolved.Connections {
			sources = append(sources, connection.Config.Source)
		}
		for _, source := range sources {
			if rule := source.Rules; rule != nil && rule.Mode == "operator" {
				if tenant, ok := namespaces[rule.Namespace]; ok && tenant != resolved.Config.ID {
					return nil, fmt.Errorf("operator namespace cannot be shared by different Nines tenants")
				}
				namespaces[rule.Namespace] = resolved.Config.ID
			}
		}
	}
	return out, nil
}
