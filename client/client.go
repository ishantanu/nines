// Package client provides a tenant-bound HTTP client for Nines resources.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type Config struct {
	URL    string
	Tenant string
	Token  string
	// HTTPClient optionally supplies a transport. Redirects are always blocked.
	HTTPClient *http.Client
}
type Client struct {
	origin, tenant, token string
	http                  *http.Client
}
type APIError struct{ StatusCode int }

func (e *APIError) Error() string  { return fmt.Sprintf("Nines API returned HTTP %d", e.StatusCode) }
func (e *APIError) Conflict() bool { return e.StatusCode == http.StatusConflict }

type Validation struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   v1.Metadata `json:"metadata"`
	Digest     string      `json:"digest"`
	Status     string      `json:"status"`
}
type Revision struct {
	Manifest json.RawMessage `json:"manifest"`
	Revision int64           `json:"revision"`
	Digest   string          `json:"digest"`
}
type Plan struct {
	APIVersion       string          `json:"apiVersion"`
	Kind             string          `json:"kind"`
	Action           string          `json:"action"`
	ExpectedRevision int64           `json:"expectedRevision"`
	Digest           string          `json:"digest"`
	Manifest         json.RawMessage `json:"manifest"`
	Previous         *Revision       `json:"previous,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func New(c Config) (*Client, error) {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, errors.New("uRL must be an origin without credentials, path, query or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return nil, errors.New("use HTTPS, or HTTP on loopback for local development")
	}
	if !identifier.MatchString(c.Tenant) || strings.ContainsAny(c.Token, "\r\n") {
		return nil, errors.New("a valid tenant and single-line token are required")
	}
	h := http.Client{Timeout: 30 * time.Second}
	if c.HTTPClient != nil {
		h = http.Client{Transport: c.HTTPClient.Transport, Jar: c.HTTPClient.Jar, Timeout: c.HTTPClient.Timeout}
		if h.Timeout == 0 {
			h.Timeout = 30 * time.Second
		}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{strings.TrimRight(c.URL, "/"), c.Tenant, c.Token, &h}, nil
}
func (c *Client) Validate(ctx context.Context, r v1.Resource) (Validation, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return Validation{}, err
	}
	return c.ValidateYAML(ctx, b)
}
func (c *Client) Plan(ctx context.Context, r v1.Resource) (Plan, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return Plan{}, err
	}
	return c.PlanYAML(ctx, b)
}
func (c *Client) Apply(ctx context.Context, r v1.Resource, expected int64) (Revision, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return Revision{}, err
	}
	return c.ApplyYAML(ctx, b, expected)
}

// CreateSLO creates a new objective. Existing names return HTTP 409.
func (c *Client) CreateSLO(ctx context.Context, slo v1.ServiceLevelObjective) (Revision, error) {
	return c.Apply(ctx, slo, 0)
}
func (c *Client) ValidateYAML(ctx context.Context, b []byte) (Validation, error) {
	var out Validation
	err := c.request(ctx, "validate", b, nil, &out)
	return out, err
}
func (c *Client) PlanYAML(ctx context.Context, b []byte) (Plan, error) {
	var out Plan
	err := c.request(ctx, "plan", b, nil, &out)
	return out, err
}
func (c *Client) ApplyYAML(ctx context.Context, b []byte, expected int64) (Revision, error) {
	var out Revision
	if expected < 0 {
		return out, errors.New("expected revision must be nonnegative")
	}
	err := c.request(ctx, "apply", b, &expected, &out)
	return out, err
}
func (c *Client) request(ctx context.Context, action string, body []byte, expected *int64, out any) error {
	if len(body) == 0 || len(body) > 1<<20 {
		return errors.New("provide one YAML resource of at most 1 MiB")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/api/resources/"+action, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Nines-Tenant", c.tenant)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if expected != nil {
		req.Header.Set("If-Match", fmt.Sprintf("\"%d\"", *expected))
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("nines request failed")
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != 200 {
		return &APIError{res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil {
		return errors.New("could not read Nines response")
	}
	if len(data) > 4<<20 {
		return errors.New("nines response exceeds 4 MiB")
	}
	if err = json.Unmarshal(data, out); err != nil {
		return errors.New("invalid Nines response")
	}
	return nil
}
