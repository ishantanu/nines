// Package datadog collects disjoint COUNT/RATE metric buckets. It does not
// import vendor SLO percentages or create monitors and recording rules.
package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/queryspec"
)

type Client struct {
	endpoint, apiKey, applicationKey string
	http                             *http.Client
}

var hosts = map[string]bool{"api.datadoghq.com": true, "api.datadoghq.eu": true, "api.us3.datadoghq.com": true, "api.us5.datadoghq.com": true, "api.ap1.datadoghq.com": true, "api.ap2.datadoghq.com": true, "api.uk1.datadoghq.com": true, "api.ddog-gov.com": true, "api.us2.ddog-gov.com": true}

func New(endpoint, apiKey, applicationKey string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || !hosts[u.Host] || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, model.Invalid("Datadog requires an official HTTPS API site origin")
	}
	for _, key := range []string{apiKey, applicationKey} {
		if key == "" || len(key) > 8192 || strings.TrimSpace(key) != key || strings.ContainsAny(key, "\r\n\x00") {
			return nil, model.Invalid("Datadog requires API and application credentials")
		}
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), apiKey: apiKey, applicationKey: applicationKey, http: &http.Client{Transport: observability.Transport{PeerService: "datadog"}, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) SourceFingerprint() string {
	return model.DefinitionDigest(struct{ Kind, Endpoint, CredentialIdentity string }{"datadog", c.endpoint, model.DefinitionDigest([]string{c.apiKey, c.applicationKey})})
}
func (c *Client) ValidateSLO(s model.SLO) error {
	if s.QueryLanguage != "datadog" || s.ProviderWindow != nil || s.Measurement != nil || s.MeasurementBinding != nil {
		return model.Invalid("Datadog requires queryLanguage: datadog and interval-count queries")
	}
	for _, q := range []string{s.GoodQuery, s.TotalQuery} {
		if _, err := queryspec.DatadogCount(q, s.StepSeconds); err != nil {
			return model.Invalid("%v", err)
		}
	}
	return nil
}
func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("cannot prepare Datadog request")
	}
	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", c.applicationKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("datadog request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("datadog returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		return fmt.Errorf("datadog response exceeded bounds or could not be read")
	}
	if json.Unmarshal(body, out) != nil {
		return fmt.Errorf("invalid Datadog response")
	}
	return nil
}
func (c *Client) metricType(ctx context.Context, metric string, step int64) error {
	var metadata struct {
		Type     string `json:"type"`
		Interval int64  `json:"statsd_interval"`
	}
	if err := c.get(ctx, "/api/v1/metrics/"+url.PathEscape(metric), nil, &metadata); err != nil {
		return err
	}
	if metadata.Type != "count" && metadata.Type != "rate" {
		return model.Invalid("Datadog measurement metrics must have COUNT or RATE metadata; gauges and distributions are unsupported")
	}
	if (metadata.Type == "rate" && metadata.Interval <= 0) || metadata.Interval < 0 || (metadata.Interval > 0 && (metadata.Interval > step || step%metadata.Interval != 0)) {
		return model.Invalid("Datadog sampling interval must divide stepSeconds; RATE metrics require explicit interval metadata")
	}
	return nil
}
func (c *Client) query(ctx context.Context, q string, from, to time.Time, step int64) (map[int64]float64, error) {
	var data struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Type   string `json:"res_type"`
		Series []struct {
			Interval    int64        `json:"interval"`
			Aggregation string       `json:"aggr"`
			Points      [][]*float64 `json:"pointlist"`
		} `json:"series"`
	}
	params := url.Values{"from": {strconv.FormatInt(from.Unix(), 10)}, "to": {strconv.FormatInt(to.Unix()-1, 10)}, "query": {q}}
	if err := c.get(ctx, "/api/v1/query", params, &data); err != nil {
		return nil, err
	}
	if data.Status != "ok" || data.Error != "" || data.Type != "time_series" || len(data.Series) > 1 {
		return nil, fmt.Errorf("datadog did not return a single successful count series")
	}
	out := map[int64]float64{}
	if len(data.Series) == 0 {
		return out, nil
	}
	series := data.Series[0]
	// The documented v1 schema uses milliseconds, but live UK responses also
	// return seconds. Accept only an exact requested interval in either unit;
	// point timestamps remain milliseconds and are independently validated.
	if (series.Interval != step && series.Interval != step*1000) || series.Aggregation != "sum" {
		return nil, fmt.Errorf("datadog changed the requested rollup interval")
	}
	if len(series.Points) > 501 {
		return nil, fmt.Errorf("datadog response contains too many points")
	}
	seen := map[int64]bool{}
	for _, p := range series.Points {
		if len(p) != 2 || p[0] == nil || !model.Finite(*p[0]) {
			return nil, fmt.Errorf("invalid Datadog bucket")
		}
		ms := *p[0]
		sec := int64(ms / 1000)
		if ms != float64(sec*1000) || sec%step != 0 || sec < from.Unix() || sec >= to.Unix() || seen[sec] {
			return nil, fmt.Errorf("datadog returned duplicate, unaligned or out-of-range buckets")
		}
		seen[sec] = true
		if p[1] == nil {
			continue
		}
		if !model.Finite(*p[1]) || *p[1] < 0 || *p[1] > 1e18 {
			return nil, fmt.Errorf("invalid Datadog event count")
		}
		// Datadog labels buckets at their start; Nines labels interval ends.
		out[sec+step] = *p[1]
	}
	return out, nil
}
func (c *Client) Collect(ctx context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	step := time.Duration(s.StepSeconds) * time.Second
	if from.IsZero() || from.Nanosecond() != 0 || to.Nanosecond() != 0 || from.Unix()%s.StepSeconds != 0 || to.Unix()%s.StepSeconds != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour || to.Sub(from)/step > 100000 {
		return nil, model.Invalid("Datadog collection requires aligned bounds and at most 100000 intervals within 31 days")
	}
	checked := map[string]bool{}
	for _, q := range []string{s.GoodQuery, s.TotalQuery} {
		metric, _ := queryspec.DatadogCount(q, s.StepSeconds)
		if !checked[metric] {
			if err := c.metricType(ctx, metric, s.StepSeconds); err != nil {
				return nil, err
			}
			checked[metric] = true
		}
	}
	out := []model.Point{}
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(500 * step)
		if end.After(to) {
			end = to
		}
		good, err := c.query(ctx, s.GoodQuery, cursor, end, s.StepSeconds)
		if err != nil {
			return nil, err
		}
		total, err := c.query(ctx, s.TotalQuery, cursor, end, s.StepSeconds)
		if err != nil {
			return nil, err
		}
		if len(good) != len(total) {
			return nil, fmt.Errorf("datadog good and total buckets do not match; missing data is not zero")
		}
		for timestamp, g := range good {
			t, ok := total[timestamp]
			if !ok {
				return nil, fmt.Errorf("datadog good and total buckets do not match")
			}
			p := model.Point{Time: time.Unix(timestamp, 0).UTC(), Good: g, Total: t}
			if err := p.Validate(s.StepSeconds); err != nil {
				return nil, fmt.Errorf("invalid Datadog good/total count relationship")
			}
			out = append(out, p)
		}
		cursor = end
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// Probe checks the configured key pair. It does not prove metric permissions,
// eligible traffic, count semantics or a particular SLO's data coverage.
func (c *Client) Probe(ctx context.Context) error {
	var out struct {
		Status string `json:"status"`
	}
	if err := c.get(ctx, "/api/v2/validate_keys", nil, &out); err != nil {
		return err
	}
	if out.Status != "ok" {
		return fmt.Errorf("datadog key validation failed")
	}
	return nil
}
func (c *Client) SupportsRuleDeployment() bool { return false }
