// Package googlecloud reads exact DELTA event counts from Cloud Monitoring.
package googlecloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/queryspec"
)

const Endpoint = "https://monitoring.googleapis.com"

var projectPattern = regexp.MustCompile(`^([a-z][a-z0-9-]{4,28}[a-z0-9]|[1-9][0-9]{0,19})$`)
var servicePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_:.~-]{0,99}$`)

type Client struct {
	canonicalMu                sync.Mutex
	canonicalProject           string
	project, service, identity string
	quotaProject               string
	tokens                     oauth2.TokenSource
	http                       *http.Client
}

// New enables ADC only when explicitly selected. No credential JSON is accepted
// in tenant configuration, SLOs or model output.
func New(endpoint, project, service, auth, token string) (*Client, error) {
	if endpoint != Endpoint || !projectPattern.MatchString(project) || service != "" && !servicePattern.MatchString(service) {
		return nil, model.Invalid("Google Cloud requires the official Monitoring origin, a project ID/number and a bounded service ID")
	}
	h := &http.Client{Transport: observability.Transport{PeerService: "google-cloud-monitoring"}, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var source oauth2.TokenSource
	var identity string
	var quotaProject string
	switch auth {
	case "access-token":
		if token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
			return nil, model.Invalid("Google Cloud access-token auth requires a bounded bearer credential")
		}
		source = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		identity = model.DefinitionDigest(token)
	case "adc":
		if token != "" {
			return nil, model.Invalid("Google Cloud ADC and access-token authentication cannot be combined")
		}
		authHTTP := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, authHTTP)
		credentials, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/monitoring")
		if err != nil {
			return nil, fmt.Errorf("google cloud application default credentials are unavailable")
		}
		quotaProject, err = adcQuotaProject(credentials.JSON)
		if err != nil {
			return nil, err
		}
		source = credentials.TokenSource
		identity = model.DefinitionDigest(struct {
			Mode, Project string
			Config        []byte
		}{"adc", credentials.ProjectID, credentials.JSON})
	default:
		return nil, model.Invalid("Google Cloud requires explicit options.auth: adc or access-token")
	}
	return &Client{project: project, service: service, identity: identity, quotaProject: quotaProject, tokens: source, http: h}, nil
}
func (c *Client) SourceFingerprint() string {
	return model.DefinitionDigest([]string{"googlecloud", c.project, c.identity})
}
func (c *Client) SupportsRuleDeployment() bool { return false }
func (c *Client) ValidateSLO(s model.SLO) error {
	if s.QueryLanguage != "googlecloud" || s.ProviderWindow != nil || s.Measurement != nil || s.MeasurementBinding != nil {
		return model.Invalid("Google Cloud collection requires queryLanguage: googlecloud and explicit good/total count filters")
	}
	good, err := queryspec.GoogleCloudFilter(s.GoodQuery, s.StepSeconds)
	if err != nil {
		return model.Invalid("%v", err)
	}
	total, err := queryspec.GoogleCloudFilter(s.TotalQuery, s.StepSeconds)
	if err != nil {
		return model.Invalid("%v", err)
	}
	for key, value := range total {
		if good[key] != value {
			return model.Invalid("Google Cloud good filter must refine the same metric/resource population as total")
		}
	}
	return nil
}
func (c *Client) filter(query string) string {
	return "(" + query + ") AND project = " + strconv.Quote(c.project)
}
func (c *Client) request(ctx context.Context, method, path string, params url.Values, body any) (int, []byte, error) {
	if ctx.Err() != nil {
		return 0, nil, model.ErrUnavailable
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, nil, model.ErrUnavailable
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, Endpoint+path+"?"+params.Encode(), bytes.NewReader(data))
	if err != nil {
		return 0, nil, model.ErrUnavailable
	}
	token, err := c.tokens.Token()
	if err != nil || token == nil || token.AccessToken == "" || !token.Valid() || strings.ContainsAny(token.AccessToken, "\r\n") {
		return 0, nil, model.ErrUnavailable
	}
	if ctx.Err() != nil {
		return 0, nil, model.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	if c.quotaProject != "" {
		req.Header.Set("X-Goog-User-Project", c.quotaProject)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, model.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	data, err = io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return 0, nil, model.ErrUnavailable
	}
	return resp.StatusCode, data, nil
}

type interval struct {
	Start string `json:"startTime"`
	End   string `json:"endTime"`
}
type point struct {
	Interval interval `json:"interval"`
	Value    struct {
		Int64  *string  `json:"int64Value"`
		Double *float64 `json:"doubleValue"`
	} `json:"value"`
}
type series struct {
	Metric struct {
		Type   string            `json:"type"`
		Labels map[string]string `json:"labels"`
	} `json:"metric"`
	Resource struct {
		Type   string            `json:"type"`
		Labels map[string]string `json:"labels"`
	} `json:"resource"`
	Kind      string  `json:"metricKind"`
	ValueType string  `json:"valueType"`
	Points    []point `json:"points"`
}

func (c *Client) counts(ctx context.Context, filter string, step int64, from, to time.Time) (map[int64]float64, error) {
	expected, err := queryspec.GoogleCloudFilter(filter, step)
	if err != nil {
		return nil, model.ErrUnavailable
	}
	sums := map[int64]float64{}
	seen := map[string]bool{}
	seriesSlots := map[string]map[int64]bool{}
	pages := map[string]bool{}
	page := ""
	count := 0
	for n := 0; n < 32; n++ {
		params := url.Values{"filter": {c.filter(filter)}, "interval.startTime": {from.Format(time.RFC3339)}, "interval.endTime": {to.Format(time.RFC3339)}, "view": {"FULL"}, "pageSize": {"10000"}}
		if page != "" {
			params.Set("pageToken", page)
		}
		code, data, err := c.request(ctx, http.MethodGet, "/v3/projects/"+c.project+"/timeSeries", params, nil)
		if err != nil || code != 200 {
			return nil, model.ErrUnavailable
		}
		var response struct {
			TimeSeries  []series          `json:"timeSeries"`
			Next        string            `json:"nextPageToken"`
			Errors      []json.RawMessage `json:"executionErrors"`
			Error       json.RawMessage   `json:"error"`
			Warnings    []json.RawMessage `json:"warnings"`
			Unreachable []string          `json:"unreachable"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Errors) > 0 || len(response.Unreachable) > 0 || len(response.Error) > 0 || len(response.Warnings) > 0 {
			return nil, model.ErrUnavailable
		}
		for _, s := range response.TimeSeries {
			if s.Kind != "DELTA" || (s.ValueType != "INT64" && s.ValueType != "DOUBLE") || s.Metric.Type != expected["metric.type"] || s.Resource.Type != expected["resource.type"] {
				return nil, model.ErrUnavailable
			}
			for key, value := range expected {
				if strings.HasPrefix(key, "metric.labels.") && s.Metric.Labels[strings.TrimPrefix(key, "metric.labels.")] != value {
					return nil, model.ErrUnavailable
				}
				if strings.HasPrefix(key, "resource.labels.") && s.Resource.Labels[strings.TrimPrefix(key, "resource.labels.")] != value {
					return nil, model.ErrUnavailable
				}
			}
			if project := s.Resource.Labels["project_id"]; project != "" && strings.Trim(c.project, "0123456789") != "" && project != c.project {
				return nil, model.ErrUnavailable
			}
			identity := model.DefinitionDigest(struct{ Metric, Resource map[string]string }{s.Metric.Labels, s.Resource.Labels})
			slots := seriesSlots[identity]
			if slots == nil {
				slots = map[int64]bool{}
				seriesSlots[identity] = slots
			}
			if len(seriesSlots) > 512 {
				return nil, model.ErrUnavailable
			}
			for _, p := range s.Points {
				start, e1 := time.Parse(time.RFC3339Nano, p.Interval.Start)
				end, e2 := time.Parse(time.RFC3339Nano, p.Interval.End)
				if e1 != nil || e2 != nil || start.Nanosecond() != 0 || end.Nanosecond() != 0 || !start.Equal(end.Add(-time.Duration(step)*time.Second)) || end.Unix()%step != 0 || start.Before(from) || end.After(to) {
					return nil, model.ErrUnavailable
				}
				var value float64
				switch {
				case s.ValueType == "INT64" && p.Value.Int64 != nil && p.Value.Double == nil:
					value, err = strconv.ParseFloat(*p.Value.Int64, 64)
					integer, e := strconv.ParseUint(*p.Value.Int64, 10, 64)
					if err != nil || e != nil || integer > 1<<53 {
						return nil, model.ErrUnavailable
					}
				case s.ValueType == "DOUBLE" && p.Value.Double != nil && p.Value.Int64 == nil:
					value = *p.Value.Double
				default:
					return nil, model.ErrUnavailable
				}
				key := identity + "/" + strconv.FormatInt(end.Unix(), 10)
				if seen[key] || !model.Finite(value) || value < 0 || value > 1<<53 {
					return nil, model.ErrUnavailable
				}
				seen[key] = true
				slots[end.Unix()] = true
				if value > (1<<53)-sums[end.Unix()] {
					return nil, model.ErrUnavailable
				}
				sums[end.Unix()] += value
				count++
				if count > 100000 {
					return nil, model.ErrUnavailable
				}
			}
		}
		if response.Next == "" {
			// Every selected series must cover every returned slot. A disappearing replica
			// is missing evidence, not an implicit zero contribution.
			for _, slots := range seriesSlots {
				if len(slots) != len(sums) {
					return nil, model.ErrUnavailable
				}
			}
			return sums, nil
		}
		if len(response.Next) > 8192 || pages[response.Next] {
			return nil, model.ErrUnavailable
		}
		page = response.Next
		pages[page] = true
	}
	return nil, model.ErrUnavailable
}
func (c *Client) Collect(ctx context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if err := c.ValidateSLO(s); err != nil {
		return nil, err
	}
	if from.IsZero() || from.Nanosecond() != 0 || to.Nanosecond() != 0 || from.Unix()%s.StepSeconds != 0 || to.Unix()%s.StepSeconds != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour || to.Sub(from)/(time.Duration(s.StepSeconds)*time.Second) > 100000 {
		return nil, model.Invalid("Google Cloud collection requires aligned bounds and at most 100000 intervals within 31 days")
	}
	good, err := c.counts(ctx, s.GoodQuery, s.StepSeconds, from, to)
	if err != nil {
		return nil, err
	}
	total, err := c.counts(ctx, s.TotalQuery, s.StepSeconds, from, to)
	if err != nil {
		return nil, err
	}
	if len(good) != len(total) {
		return nil, model.ErrUnavailable
	}
	out := []model.Point{}
	for at, g := range good {
		t, ok := total[at]
		p := model.Point{Time: time.Unix(at, 0).UTC(), Good: g, Total: t}
		if !ok || p.Validate(s.StepSeconds) != nil {
			return nil, model.ErrUnavailable
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}
func (c *Client) Probe(ctx context.Context) error {
	code, body, err := c.request(ctx, http.MethodGet, "/v3/projects/"+c.project+"/metricDescriptors", url.Values{"pageSize": {"1"}}, nil)
	var result map[string]json.RawMessage
	if err != nil || code != 200 || json.Unmarshal(body, &result) != nil || result == nil || len(result["error"]) > 0 {
		return model.ErrUnavailable
	}
	return nil
}
func (c *Client) MeasurementLimitations() []string {
	return []string{"Google Cloud collection accepts raw DELTA INT64/DOUBLE event counts spanning exactly one configured step; it does not interpolate missing samples or convert cumulative counters, distributions, rates or percentages.", "Metrics scopes are constrained with a server-pinned project filter. Observed count intervals do not prove instrumentation or population completeness."}
}

// ADC's quota project controls billing/quota, never the metric or SLO route.
func adcQuotaProject(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	var config struct {
		Project string `json:"quota_project_id"`
	}
	if json.Unmarshal(data, &config) != nil || config.Project != "" && !projectPattern.MatchString(config.Project) {
		return "", fmt.Errorf("google cloud ADC quota project is invalid")
	}
	return config.Project, nil
}
