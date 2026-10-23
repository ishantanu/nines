// Package mcpserver exposes a tenant-bound MCP interface to the Nines API.
package mcpserver

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
)

const maxResponseBytes = 4 << 20

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Config is operator-controlled. Tools cannot select a server, token or tenant.
type Config struct {
	Server              string
	Tenant              string
	Token               string
	AllowWrites         bool
	AllowRuleDeployment bool
	AllowRuleWithdrawal bool
}

type yamlResource struct {
	Manifest string
	Expected *int64
}

type apiHTTPError struct {
	Status  int
	Message string
}

func (e *apiHTTPError) Error() string { return fmt.Sprintf("API HTTP %d: %s", e.Status, e.Message) }

type apiClient struct {
	base   string
	tenant string
	token  string
	http   *http.Client
}

func newAPI(c Config) (*apiClient, error) {
	if c.AllowRuleWithdrawal && (!c.AllowWrites || !c.AllowRuleDeployment) {
		return nil, errors.New("rule withdrawal requires allow-writes, allow-rule-deployment and allow-rule-withdrawal")
	}
	if c.AllowRuleDeployment && !c.AllowWrites {
		return nil, errors.New("rule deployment requires allow-writes as well as allow-rule-deployment")
	}
	u, err := url.Parse(c.Server)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return nil, errors.New("server must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return nil, errors.New("use HTTPS for a remote server; HTTP is allowed only on loopback")
		}
	}
	if !identifier.MatchString(c.Tenant) {
		return nil, errors.New("tenant must contain 1–63 lowercase letters, digits or hyphens")
	}
	if strings.ContainsAny(c.Token, "\r\n") {
		return nil, errors.New("API token contains invalid characters")
	}
	return &apiClient{
		base: strings.TrimRight(c.Server, "/"), tenant: c.Tenant, token: c.Token,
		http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func validID(id string) error {
	if !identifier.MatchString(id) {
		return errors.New("id must contain 1–63 lowercase letters, digits or hyphens")
	}
	return nil
}

// request never includes upstream bodies, URLs, headers or transport error text
// in errors returned to a model. Upstream responses may contain credentials.
func (a *apiClient) request(ctx context.Context, method, path string, body any) (any, error) {
	b, err := a.requestBytes(ctx, method, path, body, "application/json")
	if err != nil {
		return nil, err
	}
	var result any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&result) != nil || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("API returned invalid JSON")
	}
	return result, nil
}

func (a *apiClient) requestBytes(ctx context.Context, method, path string, body any, accept string) ([]byte, error) {
	var data []byte
	contentType := "application/json"
	var expected *int64
	if resource, ok := body.(yamlResource); ok {
		data = []byte(resource.Manifest)
		contentType = "application/yaml"
		expected = resource.Expected
		if len(data) > 1<<20 {
			return nil, errors.New("manifest exceeds size limit")
		}
	} else if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil || len(data) > 2<<20 {
			return nil, errors.New("request is invalid or exceeds the API size limit")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("could not construct API request")
	}
	req.Header.Set("X-Nines-Tenant", a.tenant)
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
		if expected != nil {
			req.Header.Set("If-Match", fmt.Sprintf("\"%d\"", *expected))
		}
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("API request cancelled")
		}
		return nil, errors.New("API request failed or timed out")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := map[int]string{400: "invalid request", 401: "authentication required", 403: "tenant access denied", 404: "resource not found", 409: "revision or resource conflict", 413: "request too large", 429: "rate limited"}[resp.StatusCode]
		if message == "" {
			message = "upstream request failed"
		}
		return nil, &apiHTTPError{Status: resp.StatusCode, Message: message}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(b) > maxResponseBytes {
		return nil, errors.New("API response unreadable or exceeds 4 MiB")
	}
	return b, nil
}
