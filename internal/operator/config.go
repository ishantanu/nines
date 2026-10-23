package operator

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/tenancy"
)

type Source struct {
	Kind                 string            `yaml:"kind" json:"kind"`
	QueryURL             string            `yaml:"queryURL" json:"queryURL"`
	RulerURL             string            `yaml:"rulerURL,omitempty" json:"rulerURL,omitempty"`
	MimirOrgID           string            `yaml:"mimirOrgID,omitempty" json:"mimirOrgID,omitempty"`
	RulerNamespace       string            `yaml:"rulerNamespace,omitempty" json:"rulerNamespace,omitempty"`
	QueryBearerTokenFile string            `yaml:"queryBearerTokenFile,omitempty" json:"-"`
	RulerBearerTokenFile string            `yaml:"rulerBearerTokenFile,omitempty" json:"-"`
	BearerTokenFile      string            `yaml:"bearerTokenFile,omitempty" json:"-"`
	PrometheusLabels     map[string]string `yaml:"prometheusLabels,omitempty" json:"prometheusLabels,omitempty"`
}
type Config struct {
	Namespace string            `yaml:"namespace" json:"namespace"`
	Tenant    string            `yaml:"tenant" json:"tenant"`
	Projects  []string          `yaml:"projects" json:"projects"`
	Sources   map[string]Source `yaml:"sources" json:"sources"`
}

func LoadConfig(path string) (Config, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Config{}, e
	}
	if len(b) > 64<<10 {
		return Config{}, fmt.Errorf("operator configuration exceeds 64 KiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	var c Config
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	if d.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("provide one configuration document")
	}
	return c, c.Validate()
}
func endpoint(value string) bool {
	u, e := url.Parse(value)
	return e == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func (c Config) Validate() error {
	if !tenancy.ValidID(c.Namespace) || !tenancy.ValidID(c.Tenant) || len(c.Projects) == 0 || len(c.Projects) > 50 || len(c.Sources) == 0 || len(c.Sources) > 50 {
		return fmt.Errorf("configure one namespace, tenant, allowed projects and bounded sources")
	}
	for _, p := range c.Projects {
		if p == "" || len(p) > 200 {
			return fmt.Errorf("invalid allowed project")
		}
	}
	for name, s := range c.Sources {
		if !tenancy.ValidID(name) || !endpoint(s.QueryURL) || (s.Kind != "prometheus-operator" && s.Kind != "mimir") {
			return fmt.Errorf("invalid source %s", name)
		}
		if s.Kind == "mimir" && (!endpoint(s.RulerURL) || !tenancy.ValidMimirID(s.MimirOrgID) || !tenancy.ValidID(s.RulerNamespace)) {
			return fmt.Errorf("mimir source %s requires a fixed ruler URL, org ID and namespace", name)
		}
		if len(s.PrometheusLabels) > 16 {
			return fmt.Errorf("too many Prometheus selector labels")
		}
		for key, v := range s.PrometheusLabels {
			if key == "app.kubernetes.io/managed-by" || len(key) > 253 || len(v) > 63 {
				return fmt.Errorf("invalid or reserved rule selector label")
			}
		}
	}
	return nil
}
func (c Config) Allows(project string) bool {
	for _, p := range c.Projects {
		if p == project {
			return true
		}
	}
	return false
}
func (s Source) Digest() string { return model.DefinitionDigest(s) }
func secret(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	f, e := os.Open(path)
	if e != nil {
		return "", fmt.Errorf("configured credential file is unavailable")
	}
	defer func() {
		_ = f.Close()
	}()
	b, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(b) > 4096 {
		return "", fmt.Errorf("configured credential file exceeds bounds")
	}
	v := strings.TrimSpace(string(b))
	if v == "" || strings.ContainsAny(v, "\r\n") {
		return "", fmt.Errorf("invalid configured credential")
	}
	return v, nil
}
func (s Source) Collector() (*prometheus.Client, error) {
	path := s.QueryBearerTokenFile
	if path == "" {
		path = s.BearerTokenFile
	}
	token, e := secret(path)
	if e != nil {
		return nil, e
	}
	if s.Kind == "mimir" {
		return prometheus.NewMimir(s.QueryURL, s.MimirOrgID, token)
	}
	return prometheus.New(s.QueryURL, token)
}
func (s Source) Target(name string) (rules.Target, error) {
	path := s.RulerBearerTokenFile
	if path == "" {
		path = s.BearerTokenFile
	}
	token, e := secret(path)
	return rules.Target{Connection: name, Kind: "mimir", QueryURL: s.QueryURL, RulerURL: s.RulerURL, MimirOrgID: s.MimirOrgID, Namespace: s.RulerNamespace, BearerToken: token}, e
}
