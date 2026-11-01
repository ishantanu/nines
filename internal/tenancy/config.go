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
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/auditarchive"
	"github.com/ishantanu/nines/internal/intentmodel"
	"github.com/ishantanu/nines/internal/kubediscovery"
	"github.com/ishantanu/nines/internal/systemone"
	"github.com/ishantanu/nines/internal/workflowhook"
)

const Header = "X-Nines-Tenant"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var envPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var entityGUIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_+/=-]{1,512}$`)
var mimirPattern = regexp.MustCompile(`^[a-zA-Z0-9_!.*'()-]{1,150}$`)

func ValidID(id string) bool      { return idPattern.MatchString(id) }
func ValidMimirID(id string) bool { return mimirPattern.MatchString(id) && id != "." && id != ".." }

type Config struct {
	Tenants []Tenant `json:"tenants" yaml:"tenants"`
}
type IntentModel = intentmodel.Config

type Tenant struct {
	ManagedProviderRoutes map[string][]string   `json:"managedProviderRoutes,omitempty" yaml:"managedProviderRoutes,omitempty"`
	WorkflowWebhook       *workflowhook.Config  `json:"workflowWebhook,omitempty" yaml:"workflowWebhook,omitempty"`
	ReplayPrivacy         v1.ReplayPrivacy      `json:"replayPrivacy,omitempty" yaml:"replayPrivacy,omitempty"`
	KubernetesDiscovery   *kubediscovery.Config `json:"kubernetesDiscovery,omitempty" yaml:"kubernetesDiscovery,omitempty"`
	SystemOne             *systemone.Config     `json:"systemOne,omitempty" yaml:"systemOne,omitempty"`
	SLOExtensionLock      string                `json:"sloExtensionLock,omitempty" yaml:"sloExtensionLock,omitempty"`
	AuditArchive          *auditarchive.Config  `json:"auditArchive,omitempty" yaml:"auditArchive,omitempty"`
	ReviewModel           *IntentModel          `json:"reviewModel,omitempty" yaml:"reviewModel,omitempty"`
	IntentModel           *IntentModel          `json:"intentModel,omitempty" yaml:"intentModel,omitempty"`
	Connections           []Connection          `json:"connections,omitempty" yaml:"connections,omitempty"`
	ID                    string                `json:"id" yaml:"id"`
	Name                  string                `json:"name" yaml:"name"`
	Access                []Access              `json:"access" yaml:"access"`
	Source                Source                `json:"source" yaml:"source"`
	AlertWebhookEnv       string                `json:"alertWebhookEnv,omitempty" yaml:"alertWebhookEnv,omitempty"`
}
type Access struct {
	TokenEnv string `json:"tokenEnv" yaml:"tokenEnv"`
	Role     string `json:"role" yaml:"role"`
}
type RuleDestination struct {
	EntityGUID       string `json:"entityGUID,omitempty" yaml:"entityGUID,omitempty"`
	AutoDeploy       bool   `json:"autoDeploy,omitempty" yaml:"autoDeploy,omitempty"`
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
	Credentials        map[string]string `json:"-" yaml:"-"`
	Config             Connection
	BearerToken        string `json:"-" yaml:"-"`
	BasicPassword      string `json:"-" yaml:"-"`
	RulesBearerToken   string `json:"-" yaml:"-"`
	RulesBasicPassword string `json:"-" yaml:"-"`
}
type Source struct {
	CollectionMode string            `json:"collectionMode,omitempty" yaml:"collectionMode,omitempty"`
	Options        map[string]string `json:"options,omitempty" yaml:"options,omitempty"`
	CredentialsEnv map[string]string `json:"credentialsEnv,omitempty" yaml:"credentialsEnv,omitempty"`
	Rules          *RuleDestination  `json:"rules,omitempty" yaml:"rules,omitempty"`

	Kind             string `json:"kind" yaml:"kind"`
	URL              string `json:"url,omitempty" yaml:"url,omitempty"`
	MimirOrgID       string `json:"mimirOrgID,omitempty" yaml:"mimirOrgID,omitempty"`
	BearerTokenEnv   string `json:"bearerTokenEnv,omitempty" yaml:"bearerTokenEnv,omitempty"`
	BasicUsername    string `json:"basicUsername,omitempty" yaml:"basicUsername,omitempty"`
	BasicPasswordEnv string `json:"basicPasswordEnv,omitempty" yaml:"basicPasswordEnv,omitempty"`
}

// Resolved values are intentionally excluded from JSON/YAML serialization.
type Resolved struct {
	Credentials        map[string]string      `json:"-" yaml:"-"`
	WorkflowWebhook    *workflowhook.Settings `json:"-" yaml:"-"`
	IntentModelKey     string                 `json:"-" yaml:"-"`
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
		if t.KubernetesDiscovery != nil {
			if err := t.KubernetesDiscovery.Validate(); err != nil {
				return nil, fmt.Errorf("tenant %s: %w", t.ID, err)
			}
		}
		if len(t.ManagedProviderRoutes) > 4 {
			return nil, fmt.Errorf("managed provider routes support New Relic accounts/entities, Dash0 datasets and VictoriaMetrics cluster accounts/projects")
		}
		for kind, routes := range t.ManagedProviderRoutes {
			if (kind != "newrelic" && kind != "newrelic-entity" && kind != "dash0" && kind != "victoriametrics") || len(routes) > 32 {
				return nil, fmt.Errorf("invalid managed provider route policy")
			}
			for _, route := range routes {
				if kind == "victoriametrics" && !ValidVictoriaMetricsRoute(route) {
					return nil, fmt.Errorf("invalid VictoriaMetrics cluster route; use a canonical uint32 account or account:project")
				}
				if route == "" || len(route) > 512 || kind != "newrelic-entity" && len(route) > 64 || strings.TrimSpace(route) != route || kind == "newrelic-entity" && !entityGUIDPattern.MatchString(route) {
					return nil, fmt.Errorf("invalid managed provider route")
				}
			}
		}
		if err := validateReplayPrivacy(t.ReplayPrivacy); err != nil {
			return nil, err
		}
		if t.AuditArchive != nil {
			if err := t.AuditArchive.Validate(); err != nil {
				return nil, fmt.Errorf("tenant %s: %w", t.ID, err)
			}
		}
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
		if s.CollectionMode != "" && s.CollectionMode != "automatic" && s.CollectionMode != "manual" {
			return nil, fmt.Errorf("collectionMode must be automatic or manual")
		}
		if !ValidID(s.Kind) {
			return nil, fmt.Errorf("source kind must be a provider identifier")
		}
		if len(s.Options) > 16 {
			return nil, fmt.Errorf("configure at most 16 provider options")
		}
		for key, value := range s.Options {
			if key == "" || len(key) > 64 || len(value) > 512 {
				return nil, fmt.Errorf("provider option exceeds bounds")
			}
		}
		if len(s.CredentialsEnv) > 8 {
			return nil, fmt.Errorf("configure at most eight provider credential references")
		}
		if len(s.CredentialsEnv) > 0 && (s.BearerTokenEnv != "" || s.BasicUsername != "" || s.BasicPasswordEnv != "") {
			return nil, fmt.Errorf("choose provider credential references or legacy bearer/basic authentication")
		}
		r.Credentials = map[string]string{}
		for name, env := range s.CredentialsEnv {
			if name == "" || len(name) > 64 {
				return nil, fmt.Errorf("invalid provider credential field")
			}
			value, err := secret(env, true)
			if err != nil {
				return nil, err
			}
			r.Credentials[name] = value
		}

		if s.Kind == "manual" {
			if len(s.Options) > 0 || len(s.CredentialsEnv) > 0 || s.Rules != nil || s.URL != "" || s.MimirOrgID != "" || s.BearerTokenEnv != "" || s.BasicUsername != "" || s.BasicPasswordEnv != "" {
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
			if s.Kind != "prometheus" && s.Kind != "mimir" && s.Kind != "dash0" && s.Kind != "googlecloud" && s.Kind != "datadog" && s.Kind != "newrelic" {
				return nil, fmt.Errorf("rule deployment is supported only for Prometheus, Mimir, Datadog, New Relic, Dash0 and Google Cloud providers")
			}
			if s.Kind != "newrelic" && rules.EntityGUID != "" {
				return nil, fmt.Errorf("entityGUID is only supported for New Relic native publication")
			}
			if !ValidID(rules.Namespace) {
				return nil, fmt.Errorf("rules require a valid namespace")
			}
			if rules.Mode != "" && rules.Mode != "operator" && rules.Mode != "native" {
				return nil, fmt.Errorf("unsupported rule deployment mode")
			}
			if s.Kind == "newrelic" {
				if rules.Mode != "native" || rules.EntityGUID == "" || !entityGUIDPattern.MatchString(rules.EntityGUID) || rules.AutoDeploy || rules.Directory != "" || rules.RulerURL != "" || rules.Reload || rules.Publisher != "" || rules.BasicUsername != "" || rules.BasicPasswordEnv != "" || rules.BearerTokenEnv != "" {
					return nil, fmt.Errorf("new relic native publication requires mode: native, configured entityGUID and the connection user key; automatic/file/ruler/operator options are unsupported")
				}
			} else if s.Kind == "datadog" {
				if rules.Mode != "native" || rules.AutoDeploy || rules.Directory != "" || rules.RulerURL != "" || rules.Reload || rules.Publisher != "" || rules.BasicUsername != "" || rules.BasicPasswordEnv != "" || rules.BearerTokenEnv != "" {
					return nil, fmt.Errorf("datadog native publication requires mode: native, explicit namespace and the configured API/application key pair; automatic/file/ruler/operator options are unsupported")
				}
			} else if s.Kind == "dash0" {
				if rules.Mode != "native" || rules.AutoDeploy || rules.Directory != "" || rules.RulerURL != "" || rules.Reload || rules.Publisher != "" || rules.BasicUsername != "" || rules.BasicPasswordEnv != "" || rules.BearerTokenEnv == "" {
					return nil, fmt.Errorf("Dash0 native publication requires mode: native and a separate bearerTokenEnv; automatic deployment and file/ruler/operator options are unsupported")
				}
			} else if s.Kind == "googlecloud" {
				if rules.Mode != "native" || rules.AutoDeploy || rules.Directory != "" || rules.RulerURL != "" || rules.Reload || rules.Publisher != "" || rules.BasicUsername != "" || rules.BasicPasswordEnv != "" || s.Options["serviceID"] == "" || rules.BearerTokenEnv == "" && s.Options["auth"] != "adc" {
					return nil, fmt.Errorf("google cloud native publication requires mode: native, serviceID and explicit ADC or a publish-token reference; automatic/file/ruler/operator options are unsupported")
				}
			} else if rules.Mode == "native" {
				return nil, fmt.Errorf("native publication mode requires Datadog, New Relic, Dash0 or Google Cloud")
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
				if rules.AutoDeploy && !rules.Reload {
					return nil, fmt.Errorf("automatic Prometheus rule deployment requires reload: true, a shared rule_files directory and lifecycle enablement")
				}
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
			r.Connections = append(r.Connections, ResolvedConnection{Credentials: resolved.Credentials, Config: connection, BearerToken: resolved.BearerToken, BasicPassword: resolved.BasicPassword, RulesBearerToken: resolved.RulesBearerToken, RulesBasicPassword: resolved.RulesBasicPassword})
		}

		var err error
		if t.WorkflowWebhook != nil {
			endpoint, e := secret(t.WorkflowWebhook.EndpointEnv, true)
			if e != nil {
				return nil, e
			}
			key, e := secret(t.WorkflowWebhook.SigningKeyEnv, true)
			if e != nil {
				return nil, e
			}
			if e = workflowhook.Validate(endpoint, key, t.WorkflowWebhook.Since); e != nil {
				return nil, fmt.Errorf("tenant %s: %w", t.ID, e)
			}
			r.WorkflowWebhook = &workflowhook.Settings{Endpoint: endpoint, Key: key, Since: t.WorkflowWebhook.Since}
		}
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

// ValidVictoriaMetricsRoute accepts one fixed numeric cluster account/project.
// Wildcard and multitenant endpoints never become managed routes.
func ValidVictoriaMetricsRoute(route string) bool {
	parts := strings.Split(route, ":")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(n, 10) != part {
			return false
		}
	}
	return true
}
