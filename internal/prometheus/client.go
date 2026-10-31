package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/tenancy"
)

type Client struct {
	// DenyPartialResponse instructs compatible backends to fail partial queries.
	DenyPartialResponse bool
	BaseURL             string
	BearerToken         string
	HTTP                *http.Client
	orgID               string
	basicUsername       string
	basicPassword       string
}

func New(base, token string) (*Client, error) {
	if strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("invalid source bearer credential")
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid Prometheus base URL")
	}
	return &Client{BaseURL: strings.TrimRight(base, "/"), BearerToken: token, HTTP: &http.Client{Transport: observability.Transport{PeerService: "prometheus"}, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// NewMimir pins the upstream tenant to trusted server configuration. Incoming
// request headers and PromQL content can never override X-Scope-OrgID.
func NewMimir(base, orgID, token string) (*Client, error) {
	if !tenancy.ValidMimirID(orgID) {
		return nil, fmt.Errorf("mimir requires one valid tenant ID; federation is disabled")
	}
	c, err := New(base, token)
	if err != nil {
		return nil, err
	}
	c.orgID = orgID
	c.HTTP.Transport = observability.Transport{PeerService: "mimir"}
	return c, nil
}
func (c *Client) SetBasicAuth(username, password string) error {
	if c.BearerToken != "" || username == "" || password == "" || strings.ContainsAny(username, "\r\n:") || strings.ContainsAny(password, "\r\n") {
		return fmt.Errorf("invalid or conflicting source authentication")
	}
	c.basicUsername = username
	c.basicPassword = password
	return nil
}

// ValidateSLO prevents queries for a different provider reaching Prometheus.
func (c *Client) ValidateSLO(s model.SLO) error {
	if s.QueryLanguage != "" && s.QueryLanguage != "promql" {
		return model.Invalid("Prometheus/Mimir requires PromQL")
	}
	return nil
}

// Collect evaluates disjoint, user-specified interval-count expressions.
// The caller must use queries such as sum(increase(counter[1m])) with a 60s
// step. Raw cumulative counters and overlapping rate windows are unsupported.
func (c *Client) Collect(ctx context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	if s.ProviderWindow != nil {
		return nil, model.Invalid("use CollectWindows for overlapping provider observations")
	}
	out := []model.Point{}
	step := time.Duration(s.StepSeconds) * time.Second
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(10000 * step)
		if end.After(to) {
			end = to
		}
		good, err := c.query(ctx, s.GoodQuery, cursor.Add(step), end, s.StepSeconds)
		if err != nil {
			return nil, fmt.Errorf("good query: %w", err)
		}
		total, err := c.query(ctx, s.TotalQuery, cursor.Add(step), end, s.StepSeconds)
		if err != nil {
			return nil, fmt.Errorf("total query: %w", err)
		}
		if len(good) != len(total) {
			return nil, fmt.Errorf("good and total queries returned unmatched timestamps")
		}
		for timestamp, g := range good {
			t, ok := total[timestamp]
			if !ok {
				return nil, fmt.Errorf("good and total queries returned unmatched timestamps")
			}
			p := model.Point{Time: time.Unix(timestamp, 0).UTC(), Good: g, Total: t}
			if err = p.Validate(s.StepSeconds); err != nil {
				return nil, fmt.Errorf("invalid source sample: %w", err)
			}
			out = append(out, p)
		}
		cursor = end
	}
	return out, nil
}

func (c *Client) query(ctx context.Context, query string, from, to time.Time, step int64) (map[int64]float64, error) {
	form := url.Values{"query": {query}, "start": {strconv.FormatInt(from.Unix(), 10)}, "end": {strconv.FormatInt(to.Unix(), 10)}, "step": {strconv.FormatInt(step, 10)}, "timeout": {"15s"}}
	if c.DenyPartialResponse {
		form.Set("deny_partial_response", "1")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/query_range", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.orgID != "" {
		req.Header.Set("X-Scope-OrgID", c.orgID)
	}
	if c.basicUsername != "" {
		req.SetBasicAuth(c.basicUsername, c.basicPassword)
	}
	if c.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("prometheus response exceeds 16 MiB")
	}
	var envelope struct {
		Status   string   `json:"status"`
		Partial  bool     `json:"isPartial"`
		Error    string   `json:"error"`
		Warnings []string `json:"warnings"`
		Data     struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		return nil, fmt.Errorf("invalid Prometheus JSON: %w", err)
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed")
	}
	if envelope.Partial || len(envelope.Warnings) > 0 {
		return nil, fmt.Errorf("prometheus returned query warnings; refusing partial data")
	}
	if envelope.Data.ResultType != "matrix" || len(envelope.Data.Result) > 1 {
		return nil, fmt.Errorf("query must return one aggregated series (use sum), or no series")
	}
	values := map[int64]float64{}
	if len(envelope.Data.Result) == 0 {
		return values, nil
	}
	for _, pair := range envelope.Data.Result[0].Values {
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid sample tuple")
		}
		var timestamp float64
		var text string
		if err = json.Unmarshal(pair[0], &timestamp); err != nil {
			return nil, fmt.Errorf("invalid timestamp")
		}
		if err = json.Unmarshal(pair[1], &text); err != nil {
			return nil, fmt.Errorf("invalid sample value")
		}
		if !model.Finite(timestamp) || timestamp < float64(from.Unix()) || timestamp > float64(to.Unix()) || timestamp != float64(int64(timestamp)) || (int64(timestamp)-from.Unix())%step != 0 {
			return nil, fmt.Errorf("out-of-range or unaligned source timestamp")
		}
		v, err := strconv.ParseFloat(text, 64)
		if err != nil || !model.Finite(v) {
			return nil, fmt.Errorf("non-finite source value")
		}
		k := int64(timestamp)
		if _, exists := values[k]; exists {
			return nil, fmt.Errorf("duplicate source timestamp")
		}
		values[k] = v
	}
	return values, nil
}

// SourceFingerprint pins configured endpoint/dataset identity without exposing
// URLs or credentials in intent proposals. Credential rotation does not alter it.
func (c *Client) SourceFingerprint() string {
	return model.DefinitionDigest(struct{ URL, Org string }{c.BaseURL, c.orgID})
}
