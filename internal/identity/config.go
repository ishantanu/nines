package identity

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/tenancy"
)

type Config struct {
	PlatformAdmins []PlatformAdmin  `yaml:"platformAdmins"`
	PublicURLEnv   string           `yaml:"publicURLEnv"`
	PublicURL      string           `yaml:"publicURL"`
	SessionHours   int              `yaml:"sessionHours"`
	Providers      []ProviderConfig `yaml:"providers"`
	Bootstrap      []Bootstrap      `yaml:"bootstrap"`
}
type ProviderConfig struct {
	AllowDemoCredentials bool          `yaml:"allowDemoCredentials"`
	BootstrapUsername    string        `yaml:"bootstrapUsername"`
	BootstrapPasswordEnv string        `yaml:"bootstrapPasswordEnv"`
	ID                   string        `yaml:"id"`
	Name                 string        `yaml:"name"`
	Type                 string        `yaml:"type"`
	IssuerURL            string        `yaml:"issuerURL"`
	ClientID             string        `yaml:"clientID"`
	ClientSecretEnv      string        `yaml:"clientSecretEnv"`
	URL                  string        `yaml:"url"`
	StartTLS             bool          `yaml:"startTLS"`
	CAFile               string        `yaml:"caFile"`
	BindDN               string        `yaml:"bindDN"`
	BindPasswordEnv      string        `yaml:"bindPasswordEnv"`
	UserBaseDN           string        `yaml:"userBaseDN"`
	UserFilter           string        `yaml:"userFilter"`
	SubjectAttribute     string        `yaml:"subjectAttribute"`
	NameAttribute        string        `yaml:"nameAttribute"`
	EmailAttribute       string        `yaml:"emailAttribute"`
	RoleMappings         []RoleMapping `yaml:"roleMappings"`
	ClaimsMaxAgeMinutes  int           `yaml:"claimsMaxAgeMinutes"`
	secret               string
}
type RoleMapping struct {
	ID     string `yaml:"id" json:"id"`
	Tenant string `yaml:"tenant" json:"-"`
	Claim  string `yaml:"claim" json:"claim"`
	Value  string `yaml:"value" json:"value"`
	Role   string `yaml:"role" json:"role"`
}

type Bootstrap struct {
	Tenant   string `yaml:"tenant"`
	Provider string `yaml:"provider"`
	Subject  string `yaml:"subject"`
	Role     string `yaml:"role"`
}

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var attributeName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9;-]{0,63}$`)

func LoadConfig(path string, lookup func(string) string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	return ParseConfig(b, lookup)
}
func ParseConfig(b []byte, lookup func(string) string) (Config, error) {
	var c Config
	if len(b) == 0 || len(b) > 1<<20 {
		return c, fmt.Errorf("authentication configuration exceeds size limit or is empty")
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid authentication configuration: %w", err)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, fmt.Errorf("authentication configuration requires one document")
	}
	if c.PublicURLEnv != "" {
		if c.PublicURL != "" || !envName.MatchString(c.PublicURLEnv) {
			return c, fmt.Errorf("publicURLEnv requires one environment reference instead of publicURL")
		}
		c.PublicURL = lookup(c.PublicURLEnv)
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || !webOrigin(u) {
		return c, fmt.Errorf("publicURL must be an HTTPS origin; HTTP is allowed only for loopback development")
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	if c.SessionHours == 0 {
		c.SessionHours = 12
	}
	if c.SessionHours < 1 || c.SessionHours > 24 {
		return c, fmt.Errorf("sessionHours must be 1–24")
	}
	if len(c.Providers) == 0 || len(c.Providers) > 10 {
		return c, fmt.Errorf("configure 1–10 authentication providers")
	}
	ids := map[string]bool{}
	for i := range c.Providers {
		p := &c.Providers[i]
		if !tenancy.ValidID(p.ID) || ids[p.ID] || strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 {
			return c, fmt.Errorf("authentication providers require unique IDs and names")
		}
		ids[p.ID] = true
		secret := func(ref string) error {
			if !envName.MatchString(ref) {
				return fmt.Errorf("invalid authentication secret environment reference")
			}
			p.secret = lookup(ref)
			if p.secret == "" || strings.ContainsAny(p.secret, "\r\n") {
				return fmt.Errorf("authentication secret %s is empty or invalid", ref)
			}
			return nil
		}
		if p.Type != "local" && (p.BootstrapUsername != "" || p.BootstrapPasswordEnv != "" || p.AllowDemoCredentials) {
			return c, fmt.Errorf("bootstrap credentials require a local provider")
		}
		switch p.Type {
		case "oidc":
			issuer, e := url.Parse(p.IssuerURL)
			if e != nil || !webEndpoint(issuer) || p.ClientID == "" || len(p.ClientID) > 500 {
				return c, fmt.Errorf("OIDC requires a trusted HTTPS issuer and clientID")
			}
			if p.URL != "" || p.BindDN != "" || p.UserBaseDN != "" || p.UserFilter != "" || p.StartTLS || p.CAFile != "" || p.BindPasswordEnv != "" || p.SubjectAttribute != "" || p.NameAttribute != "" || p.EmailAttribute != "" {
				return c, fmt.Errorf("OIDC provider contains LDAP options")
			}
			if p.ClientSecretEnv != "" {
				if e = secret(p.ClientSecretEnv); e != nil {
					return c, e
				}
			}
		case "local":
			loopback := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
			if p.AllowDemoCredentials && (!loopback || p.BootstrapUsername != "admin") {
				return c, fmt.Errorf("demo credentials require a loopback publicURL and the admin bootstrap account")
			}
			if p.IssuerURL != "" || p.ClientID != "" || p.ClientSecretEnv != "" || p.URL != "" || p.BindDN != "" || p.UserBaseDN != "" || p.UserFilter != "" || p.BindPasswordEnv != "" || p.StartTLS || p.CAFile != "" {
				return c, fmt.Errorf("local provider contains external provider options")
			}
			if (p.BootstrapUsername == "") != (p.BootstrapPasswordEnv == "") {
				return c, fmt.Errorf("local bootstrap requires username and password environment reference")
			}
			if p.BootstrapUsername != "" {
				if !localUsername.MatchString(p.BootstrapUsername) {
					return c, fmt.Errorf("invalid local bootstrap username")
				}
				if e := secret(p.BootstrapPasswordEnv); e != nil {
					return c, e
				}
				if !demoPassword(*p, p.BootstrapUsername, p.secret) && (len(p.secret) < 12 || len(p.secret) > 72) {
					return c, fmt.Errorf("local passwords require 12 to 72 bytes")
				}
			}
		case "ldap":
			ldapURL, e := url.Parse(p.URL)
			if e != nil || ldapURL.Host == "" || ldapURL.User != nil || ldapURL.RawQuery != "" || ldapURL.Fragment != "" || ldapURL.Path != "" && ldapURL.Path != "/" || ldapURL.Scheme != "ldaps" && ldapURL.Scheme != "ldap" || ldapURL.Scheme == "ldap" && !p.StartTLS {
				return c, fmt.Errorf("LDAP requires ldaps:// or ldap:// with startTLS: true")
			}
			if ldapURL.Scheme == "ldaps" && p.StartTLS {
				return c, fmt.Errorf("use LDAPS or StartTLS, not both")
			}
			if p.IssuerURL != "" || p.ClientID != "" || p.ClientSecretEnv != "" {
				return c, fmt.Errorf("LDAP provider contains OIDC options")
			}
			if p.BindDN == "" || p.UserBaseDN == "" || strings.Count(p.UserFilter, "{username}") != 1 {
				return c, fmt.Errorf("LDAP requires bindDN, userBaseDN and one {username} placeholder in userFilter")
			}
			if e = secret(p.BindPasswordEnv); e != nil {
				return c, e
			}
			if p.SubjectAttribute == "" {
				p.SubjectAttribute = "entryUUID"
			}
			if p.NameAttribute == "" {
				p.NameAttribute = "displayName"
			}
			if p.EmailAttribute == "" {
				p.EmailAttribute = "mail"
			}
			for _, a := range []string{p.SubjectAttribute, p.NameAttribute, p.EmailAttribute} {
				if !attributeName.MatchString(a) {
					return c, fmt.Errorf("invalid LDAP attribute name")
				}
			}
		default:
			return c, fmt.Errorf("provider type must be local, oidc or ldap")
		}
		if err := validateMappings(p); err != nil {
			return c, err
		}
	}
	if err := validatePlatformAdmins(c); err != nil {
		return c, err
	}
	if len(c.Bootstrap) > 100 {
		return c, fmt.Errorf("too many bootstrap memberships")
	}
	for _, b := range c.Bootstrap {
		if !tenancy.ValidID(b.Tenant) || !ids[b.Provider] || b.Subject == "" || len(b.Subject) > 500 || b.Role != "owner" {
			return c, fmt.Errorf("bootstrap entries require known provider, exact subject, tenant and owner role")
		}
	}
	return c, nil
}
func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}
func webEndpoint(u *url.URL) bool {
	return u != nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || u.Scheme == "http" && loopback(u.Hostname()))
}
func webOrigin(u *url.URL) bool { return webEndpoint(u) && (u.Path == "" || u.Path == "/") }
