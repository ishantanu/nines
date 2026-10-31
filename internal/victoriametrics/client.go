// Package victoriametrics adapts the Prometheus-compatible query API while
// pinning cluster tenants and validating explicit MetricsQL interval counts.
package victoriametrics

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/queryspec"
)

type Client struct {
	client      *prometheus.Client
	fingerprint string
}

func numericID(value string) bool {
	n, err := strconv.ParseUint(value, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == value
}
func New(endpoint, accountID, projectID, bearer, username, password string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, model.Invalid("VictoriaMetrics requires an HTTP(S) origin; cluster routing belongs in accountID/projectID options")
	}
	if accountID == "" && projectID != "" {
		return nil, model.Invalid("VictoriaMetrics projectID requires accountID")
	}
	base := strings.TrimRight(endpoint, "/")
	if accountID != "" {
		if !numericID(accountID) || (projectID != "" && !numericID(projectID)) {
			return nil, model.Invalid("VictoriaMetrics requires one canonical numeric accountID and optional projectID; multitenant reads are disabled")
		}
		tenant := accountID
		if projectID != "" {
			tenant += ":" + projectID
		}
		base += "/select/" + tenant + "/prometheus"
	}
	c, err := prometheus.New(base, bearer)
	if err != nil {
		return nil, err
	}
	if username != "" || password != "" {
		if err = c.SetBasicAuth(username, password); err != nil {
			return nil, err
		}
	}
	c.DenyPartialResponse = true
	c.HTTP.Transport = observability.Transport{PeerService: "victoriametrics"}
	fingerprint := model.DefinitionDigest(struct{ Kind, URL, CredentialIdentity string }{"victoriametrics", base, model.DefinitionDigest([]string{bearer, username, password})})
	return &Client{client: c, fingerprint: fingerprint}, nil
}
func (c *Client) SourceFingerprint() string    { return c.fingerprint }
func (c *Client) SupportsRuleDeployment() bool { return false }
func (c *Client) ValidateSLO(s model.SLO) error {
	if s.StepSeconds < 1 || s.StepSeconds > 3600 || s.ProviderWindow != nil || s.Measurement != nil || s.MeasurementBinding != nil || s.GoodQuery == "" || s.TotalQuery == "" {
		return model.Invalid("VictoriaMetrics currently requires explicit good/total interval queries and stepSeconds from 1 to 3600")
	}
	if s.QueryLanguage != "" && s.QueryLanguage != "promql" && s.QueryLanguage != "metricsql" {
		return model.Invalid("VictoriaMetrics requires queryLanguage: promql or metricsql")
	}
	for _, q := range []string{s.GoodQuery, s.TotalQuery} {
		if err := queryspec.Validate(s.QueryLanguage, q, s.StepSeconds); err != nil {
			return model.Invalid("%v", err)
		}
	}
	return nil
}
func (c *Client) Collect(ctx context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	step := time.Duration(s.StepSeconds) * time.Second
	if from.IsZero() || from.Nanosecond() != 0 || to.Nanosecond() != 0 || from.Unix()%s.StepSeconds != 0 || to.Unix()%s.StepSeconds != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour || to.Sub(from)/step > 100000 {
		return nil, model.Invalid("VictoriaMetrics collection requires aligned bounds and at most 100000 intervals within 31 days")
	}
	// The shared transport checks matrix shape, cardinality, bounds and good/total
	// pairs. Language validation has already happened against this adapter.
	transportSLO := s
	transportSLO.QueryLanguage = "promql"
	points, err := c.client.Collect(ctx, transportSLO, from, to)
	if err != nil {
		return nil, fmt.Errorf("victoriametrics count query failed: %w", model.ErrUnavailable)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Time.Before(points[j].Time) })
	return points, nil
}
func (c *Client) Probe(ctx context.Context) error {
	at := time.Now().UTC().Truncate(time.Minute)
	points, err := c.client.Collect(ctx, model.SLO{StepSeconds: 60, GoodQuery: "vector(1)", TotalQuery: "vector(1)"}, at.Add(-time.Minute), at)
	if err != nil || len(points) != 1 {
		return fmt.Errorf("victoriametrics configured query probe failed")
	}
	return nil
}
func (c *Client) MeasurementLimitations() []string {
	return []string{
		"VictoriaMetrics evaluates MetricsQL, including PromQL-compatible expressions; counter increase behavior can differ from Prometheus.",
		"Counter type, eligible population, scrape coverage and interval-count semantics require telemetry review. Query validation alone does not prove them.",
	}
}
