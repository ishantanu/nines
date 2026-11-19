// Package gitops proposes reviewed SLO definitions to a Git repository as pull
// requests. Nines only writes a branch, a file and a pull request: merging and
// deploying stay with the repository's own review and pipeline.
package gitops

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Config is the administrator-owned, per-tenant delivery configuration. The
// caller of a proposal can never choose the repository, branch, path or
// credential.
type Config struct {
	Provider   string `json:"provider" yaml:"provider"`
	Repository string `json:"repository" yaml:"repository"`
	BaseBranch string `json:"baseBranch,omitempty" yaml:"baseBranch,omitempty"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
	TokenEnv   string `json:"tokenEnv" yaml:"tokenEnv"`
	// APIURL selects a GitHub Enterprise Server or self-managed GitLab API
	// origin (for GitLab it includes /api/v4). It must be HTTPS.
	APIURL string `json:"apiURL,omitempty" yaml:"apiURL,omitempty"`
}

// Settings is a validated Config plus its resolved credential. The token is
// never serialized.
type Settings struct {
	Config
	Token string `json:"-" yaml:"-"`
}

// Info is what the portal may learn about the configuration. It has no secret.
type Info struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
	Repository string `json:"repository,omitempty"`
	BaseBranch string `json:"baseBranch,omitempty"`
	Path       string `json:"path,omitempty"`
}

var (
	repositoryPattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)
	branchPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,100}$`)
	pathPattern          = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+){0,5}$`)
	safeName             = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	gitlabProjectPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}(/[A-Za-z0-9_.-]{1,100}){1,8}$`)
)

// Normalize applies defaults and validates the configuration.
func (c Config) Normalize() (Config, error) {
	switch c.Provider {
	case "github":
		if !repositoryPattern.MatchString(c.Repository) || strings.Contains(c.Repository, "..") {
			return c, errors.New("gitops.repository must be owner/name")
		}
	case "gitlab":
		// GitLab projects may live in nested groups: group/subgroup/project.
		if !gitlabProjectPattern.MatchString(c.Repository) || strings.Contains(c.Repository, "..") {
			return c, errors.New("gitops.repository must be group/project, optionally with subgroups")
		}
	default:
		return c, errors.New("gitops.provider must be github or gitlab")
	}
	if c.BaseBranch == "" {
		c.BaseBranch = "main"
	}
	if !branchPattern.MatchString(c.BaseBranch) || strings.Contains(c.BaseBranch, "..") || strings.HasSuffix(c.BaseBranch, "/") || strings.HasSuffix(c.BaseBranch, ".lock") {
		return c, errors.New("gitops.baseBranch is not a valid branch name")
	}
	if c.Path == "" {
		c.Path = "slos"
	}
	if !pathPattern.MatchString(c.Path) || strings.Contains(c.Path, "..") || strings.HasPrefix(c.Path, ".") {
		return c, errors.New("gitops.path must be a relative directory without dot segments")
	}
	if c.APIURL == "" {
		c.APIURL = "https://api.github.com"
		if c.Provider == "gitlab" {
			c.APIURL = "https://gitlab.com/api/v4"
		}
	}
	u, err := url.Parse(c.APIURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return c, errors.New("gitops.apiURL must be an https origin without credentials")
	}
	c.APIURL = strings.TrimRight(u.String(), "/")
	return c, nil
}

// Resolve validates the configuration and binds the credential read from the
// named environment variable.
func Resolve(c Config, lookup func(string) string) (*Settings, error) {
	c, err := c.Normalize()
	if err != nil {
		return nil, err
	}
	if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`).MatchString(c.TokenEnv) {
		return nil, errors.New("gitops.tokenEnv must name an environment variable")
	}
	token := lookup(c.TokenEnv)
	if token == "" {
		return nil, fmt.Errorf("required environment variable %s is empty", c.TokenEnv)
	}
	if len(token) > 8192 || strings.ContainsFunc(token, func(r rune) bool { return r < 0x20 || r == 0x7f }) || strings.TrimSpace(token) != token {
		return nil, fmt.Errorf("credential %s must be a single line", c.TokenEnv)
	}
	return &Settings{Config: c, Token: token}, nil
}

func (s *Settings) Info() Info {
	if s == nil {
		return Info{}
	}
	return Info{Configured: true, Provider: s.Provider, Repository: s.Repository, BaseBranch: s.BaseBranch, Path: s.Path}
}
