// Package dash0 implements dataset-pinned Dash0 telemetry collection.
package dash0

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/promqlvalidate"
)

var datasetPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,26}$`)
var hosts = map[string]bool{"api.eu-west-1.aws.dash0.com": true, "api.eu-central-1.aws.dash0.com": true, "api.us-east-1.aws.dash0.com": true, "api.us-west-2.aws.dash0.com": true, "api.europe-west4.gcp.dash0.com": true}

type Client struct {
	query                    *prometheus.Client
	endpoint, dataset, token string
	http                     *http.Client
}

func New(endpoint, dataset, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || !hosts[u.Host] || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, model.Invalid("Dash0 requires an official regional HTTPS API origin")
	}
	if !datasetPattern.MatchString(dataset) || token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return nil, model.Invalid("Dash0 requires an explicit dataset and bearer credential")
	}
	endpoint = strings.TrimRight(endpoint, "/")
	p, err := prometheus.New(endpoint+"/api/prometheus", token)
	if err != nil {
		return nil, err
	}
	transport := datasetTransport{next: observability.Transport{PeerService: "dash0"}, dataset: dataset, origin: endpoint}
	p.HTTP.Transport = transport
	return &Client{query: p, endpoint: endpoint, dataset: dataset, token: token, http: &http.Client{Transport: transport.next, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Dash0 documents GET query APIs. Adapt the private shared transport rather
// than sending a POST or inheriting caller-selected dataset headers.
type datasetTransport struct {
	next            http.RoundTripper
	dataset, origin string
}

func (t datasetTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != t.origin || r.URL.Path != "/api/prometheus/api/v1/query_range" {
		return nil, fmt.Errorf("Dash0 query route refused")
	}
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("Dash0 query encoding failed")
	}
	q := r.Form
	q.Set("dataset", t.dataset)
	copy := r.Clone(r.Context())
	endpoint := *r.URL
	endpoint.RawQuery = q.Encode()
	copy.URL = &endpoint
	copy.Method = http.MethodGet
	copy.Body = nil
	copy.GetBody = nil
	copy.ContentLength = 0
	copy.Header.Del("Content-Type")
	copy.Header.Del("Dash0-Dataset")
	copy.Header.Del("X-Scope-OrgID")
	return t.next.RoundTrip(copy)
}
func (c *Client) SourceFingerprint() string {
	return model.DefinitionDigest(struct{ Kind, Endpoint, Dataset, Key string }{"dash0", c.endpoint, c.dataset, model.DefinitionDigest(c.token)})
}
func (c *Client) SupportsRuleDeployment() bool { return false }
func (c *Client) ValidateSLO(s model.SLO) error {
	if err := c.query.ValidateSLO(s); err != nil {
		return err
	}
	if s.ProviderWindow == nil && s.Measurement == nil {
		for _, q := range []string{s.GoodQuery, s.TotalQuery} {
			if _, err := promqlvalidate.InstantVector(q); err != nil {
				return model.Invalid("Dash0 count queries require valid PromQL instant vectors")
			}
		}
	}
	return nil
}
func (c *Client) Collect(ctx context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	if err := validBounds(s, from, to); err != nil {
		return nil, err
	}
	points, err := c.query.Collect(ctx, s, from, to)
	if err != nil {
		return nil, model.ErrUnavailable
	}
	return points, nil
}
func (c *Client) CollectWindows(ctx context.Context, s model.SLO, from, to time.Time) ([]model.WindowObservation, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	if err := validBounds(s, from, to); err != nil {
		return nil, err
	}
	points, err := c.query.CollectWindows(ctx, s, from, to)
	if err != nil {
		return nil, model.ErrUnavailable
	}
	return points, nil
}
func validBounds(s model.SLO, from, to time.Time) error {
	if s.StepSeconds < 1 || s.StepSeconds > 3600 || from.IsZero() || from.Nanosecond() != 0 || to.Nanosecond() != 0 || from.Unix()%s.StepSeconds != 0 || to.Unix()%s.StepSeconds != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour || to.Sub(from)/(time.Duration(s.StepSeconds)*time.Second) > 100000 {
		return model.Invalid("Dash0 collection requires aligned bounds, stepSeconds 1–3600 and at most 100000 intervals within 31 days")
	}
	return nil
}
func (c *Client) Probe(ctx context.Context) error {
	at := time.Now().UTC().Truncate(time.Minute)
	p, err := c.query.Collect(ctx, model.SLO{StepSeconds: 60, GoodQuery: "vector(1)", TotalQuery: "vector(1)"}, at.Add(-time.Minute), at)
	if err != nil || len(p) != 1 {
		return fmt.Errorf("Dash0 query probe failed")
	}
	return nil
}
func (c *Client) MeasurementLimitations() []string {
	return []string{"Dash0 query collection and native Dash0 SLO evaluation are separate. Native SLO evaluation has provider-defined counter windows and settling delay; collected Nines SLI and burn policies need not match it."}
}
