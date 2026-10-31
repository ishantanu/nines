// Package newrelic reads bounded NRQL event counts and publishes explicitly
// reviewed native service levels. Nines evaluates its own burn policies.
package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/queryspec"
)

const maxBuckets = 300
const countDocument = `query NinesCounts($account: Int!, $good: Nrql!, $total: Nrql!) {
 actor { account(id: $account) { id
 good: nrql(query: $good, timeout: 15) { results metadata { messages timeWindow { begin end } } }
 total: nrql(query: $total, timeout: 15) { results metadata { messages timeWindow { begin end } } }
 } }
}`
const probeDocument = `query NinesConnection($account: Int!) { actor { account(id: $account) { id } } }`

type Client struct {
	endpoint, userKey string
	account           int64
	http              *http.Client
}

func New(endpoint, accountID, userKey string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || (u.Host != "api.newrelic.com" && u.Host != "api.eu.newrelic.com" && u.Host != "api.jp.newrelic.com") || u.Path != "/graphql" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, model.Invalid("New Relic requires an official HTTPS NerdGraph endpoint")
	}
	account, err := strconv.ParseInt(accountID, 10, 32)
	if err != nil || account < 1 || strconv.FormatInt(account, 10) != accountID {
		return nil, model.Invalid("New Relic requires one positive accountID in server-owned options")
	}
	if userKey == "" || len(userKey) > 8192 {
		return nil, model.Invalid("New Relic requires a user API key")
	}
	for _, ch := range userKey {
		if ch <= 32 || ch >= 127 {
			return nil, model.Invalid("invalid New Relic user key")
		}
	}
	return &Client{endpoint: endpoint, account: account, userKey: userKey, http: &http.Client{Timeout: 20 * time.Second, Transport: observability.Transport{PeerService: "newrelic"}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) SourceFingerprint() string {
	return model.DefinitionDigest(struct {
		Kind, Endpoint, KeyIdentity string
		Account                     int64
	}{"newrelic", c.endpoint, model.DefinitionDigest(c.userKey), c.account})
}
func (c *Client) ValidateSLO(s model.SLO) error {
	if s.QueryLanguage != "nrql" || s.ProviderWindow != nil || s.Measurement != nil || s.MeasurementBinding != nil {
		return model.Invalid("New Relic requires queryLanguage: nrql and interval event-count queries")
	}
	good, err := queryspec.NewRelicCount(s.GoodQuery, s.StepSeconds)
	if err != nil {
		return model.Invalid("goodQuery: %v", err)
	}
	total, err := queryspec.NewRelicCount(s.TotalQuery, s.StepSeconds)
	if err != nil {
		return model.Invalid("totalQuery: %v", err)
	}
	if good.EventType != total.EventType {
		return model.Invalid("New Relic good and total queries must count the same event type")
	}
	return nil
}

type bucket struct {
	Begin *int64   `json:"beginTimeSeconds"`
	End   *int64   `json:"endTimeSeconds"`
	Count *float64 `json:"nines_count"`
}
type countResult struct {
	Results  []bucket `json:"results"`
	Metadata *struct {
		Messages []json.RawMessage `json:"messages"`
		Window   *struct {
			Begin *int64 `json:"begin"`
			End   *int64 `json:"end"`
		} `json:"timeWindow"`
	} `json:"metadata"`
}
type graphResponse struct {
	Errors []json.RawMessage `json:"errors"`
	Data   *struct {
		Actor *struct {
			Account *struct {
				ID    *int64       `json:"id"`
				Good  *countResult `json:"good"`
				Total *countResult `json:"total"`
			} `json:"account"`
		} `json:"actor"`
	} `json:"data"`
}

func (c *Client) request(ctx context.Context, document string, variables map[string]any) (graphResponse, error) {
	var out graphResponse
	body, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{document, variables})
	if err != nil {
		return out, fmt.Errorf("cannot prepare New Relic request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return out, fmt.Errorf("cannot prepare New Relic request")
	}
	req.Header.Set("API-Key", c.userKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return out, fmt.Errorf("new relic request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("new relic returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return out, fmt.Errorf("new relic response exceeded bounds or could not be read")
	}
	if json.Unmarshal(data, &out) != nil {
		return out, fmt.Errorf("invalid New Relic response")
	}
	if len(out.Errors) > 0 {
		return graphResponse{}, fmt.Errorf("new relic returned GraphQL errors; partial results were refused")
	}
	if out.Data == nil || out.Data.Actor == nil || out.Data.Actor.Account == nil || out.Data.Actor.Account.ID == nil || *out.Data.Actor.Account.ID != c.account {
		return graphResponse{}, fmt.Errorf("new relic did not confirm the configured account")
	}
	return out, nil
}
func (c *Client) Probe(ctx context.Context) error {
	_, err := c.request(ctx, probeDocument, map[string]any{"account": c.account})
	return err
}
func (c *Client) SupportsRuleDeployment() bool { return false }
func readBuckets(result *countResult, from, to time.Time, step int64) (map[int64]float64, error) {
	if result == nil || result.Results == nil || result.Metadata == nil || result.Metadata.Window == nil || result.Metadata.Window.Begin == nil || result.Metadata.Window.End == nil || *result.Metadata.Window.Begin != from.UnixMilli() || *result.Metadata.Window.End != to.UnixMilli() {
		return nil, fmt.Errorf("new relic returned incomplete, warned or shifted query evidence")
	}
	if len(result.Results) > maxBuckets {
		return nil, fmt.Errorf("new relic returned too many buckets")
	}
	// New Relic returns this informational message with zero-filled TIMESERIES
	// buckets when the event population is absent. Do not turn those buckets
	// into measured coverage, and do not accept other provider warnings.
	empty := false
	for _, raw := range result.Metadata.Messages {
		var message string
		if json.Unmarshal(raw, &message) != nil || message != "No events found -- do you have the correct event type and time range?" {
			return nil, fmt.Errorf("new relic returned warned query evidence")
		}
		empty = true
	}
	out := map[int64]float64{}
	seen := map[int64]bool{}
	for _, b := range result.Results {
		if b.Begin == nil || b.End == nil || *b.Begin < from.Unix() || *b.End > to.Unix() || *b.Begin%step != 0 || *b.End-*b.Begin != step || seen[*b.End] {
			return nil, fmt.Errorf("new relic returned duplicate, unaligned or out-of-range buckets")
		}
		seen[*b.End] = true
		if b.Count == nil {
			continue
		}
		if !model.Finite(*b.Count) || *b.Count < 0 || *b.Count > 1e18 {
			return nil, fmt.Errorf("invalid New Relic event count")
		}
		if empty {
			if *b.Count != 0 {
				return nil, fmt.Errorf("new relic returned inconsistent empty query evidence")
			}
			continue
		}
		out[*b.End] = *b.Count
	}
	return out, nil
}
func (c *Client) Collect(ctx context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	step := time.Duration(s.StepSeconds) * time.Second
	if from.IsZero() || from.Unix() < 0 || from.Nanosecond() != 0 || to.Nanosecond() != 0 || from.Unix()%s.StepSeconds != 0 || to.Unix()%s.StepSeconds != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour || to.Sub(from)/step > 100000 {
		return nil, model.Invalid("New Relic collection requires aligned bounds and at most 100000 intervals within 31 days")
	}
	good, _ := queryspec.NewRelicCount(s.GoodQuery, s.StepSeconds)
	total, _ := queryspec.NewRelicCount(s.TotalQuery, s.StepSeconds)
	out := []model.Point{}
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(maxBuckets * step)
		if end.After(to) {
			end = to
		}
		suffix := fmt.Sprintf(" SINCE %d UNTIL %d TIMESERIES %d SECONDS", cursor.UnixMilli(), end.UnixMilli(), s.StepSeconds)
		response, err := c.request(ctx, countDocument, map[string]any{"account": c.account, "good": good.NRQL + suffix, "total": total.NRQL + suffix})
		if err != nil {
			return nil, err
		}
		account := response.Data.Actor.Account
		g, err := readBuckets(account.Good, cursor, end, s.StepSeconds)
		if err != nil {
			return nil, err
		}
		t, err := readBuckets(account.Total, cursor, end, s.StepSeconds)
		if err != nil {
			return nil, err
		}
		if len(g) != len(t) {
			return nil, fmt.Errorf("new relic good and total buckets do not match; missing data is not zero")
		}
		for timestamp, gv := range g {
			tv, ok := t[timestamp]
			if !ok {
				return nil, fmt.Errorf("new relic good and total buckets do not match")
			}
			p := model.Point{Time: time.Unix(timestamp, 0).UTC(), Good: gv, Total: tv}
			if err := p.Validate(s.StepSeconds); err != nil {
				return nil, fmt.Errorf("invalid New Relic good/total count relationship")
			}
			out = append(out, p)
		}
		cursor = end
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// MeasurementLimitations makes provider semantics visible to portal, CLI and
// agent quality readers instead of leaving them only in deployment docs.
func (c *Client) MeasurementLimitations() []string {
	return []string{
		"New Relic count(*) measures stored events; it does not establish unsampled request coverage or compensate for lost telemetry.",
		"NRQL may return zero-count buckets without stored events. Bucket coverage does not establish instrumentation health.",
	}
}
