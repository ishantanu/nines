package datadog

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

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/queryspec"
)

type NativeQuery struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}
type NativeThreshold struct {
	Warning   *float64 `json:"warning,omitempty"`
	Target    float64  `json:"target"`
	Timeframe string   `json:"timeframe"`
}
type NativeDocument struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Query       NativeQuery       `json:"query"`
	Thresholds  []NativeThreshold `json:"thresholds"`
	Tags        []string          `json:"tags"`
}
type NativeResource struct {
	NativeDocument
	ID string `json:"id"`
}
type Publication struct {
	Document    NativeDocument `json:"document"`
	Origin      string         `json:"origin"`
	Owner       string         `json:"owner"`
	NativeID    string         `json:"nativeID,omitempty"`
	URL         string         `json:"url,omitempty"`
	PriorDigest string         `json:"priorDigest,omitempty"`
	Matches     bool           `json:"matches"`
	Warnings    []string       `json:"warnings"`
}

var nativeIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

func (c *Client) NativeURL(id string) string {
	if !nativeIDPattern.MatchString(id) {
		return ""
	}
	return strings.Replace(c.endpoint, "https://api.", "https://app.", 1) + "/slo/manage?slo_id=" + id
}
func (c *Client) Prepare(ctx context.Context, s model.SLO, tenant, source, namespace string) (Publication, error) {
	if err := s.Validate(); err != nil {
		return Publication{}, err
	}
	if err := c.ValidateSLO(s); err != nil {
		return Publication{}, err
	}
	if tenant == "" || source == "" || namespace == "" || (s.WindowSeconds != 7*86400 && s.WindowSeconds != 30*86400) {
		return Publication{}, model.Invalid("Datadog native SLOs require ownership and an exact 7 or 30 day window; Nines does not round or replace the configured window")
	}
	if len(s.Name) > 180 {
		return Publication{}, model.Invalid("Datadog native publication requires a display name of at most 180 bytes")
	}
	owner := model.DefinitionDigest([]string{c.endpoint, tenant, source, namespace, s.Project, s.ID})[:40]
	revision := strconv.FormatInt(model.ConfigRevision(s), 10)
	definition := model.DefinitionDigest(struct {
		Definition model.SLODefinition
		Revision   string
	}{model.DefinitionFromSLO(s), revision})[:40]
	origin := "nines-" + owner[:16] + "-" + definition[:24]
	strip := func(q string) string { return strings.TrimSuffix(q, fmt.Sprintf(".rollup(sum,%d)", s.StepSeconds)) }
	p := Publication{Owner: owner, Origin: origin, Document: NativeDocument{
		Name: s.Name + " [" + origin + "]", Type: "metric", Description: "Managed by Nines. Immutable native revision " + revision + ". Nines coverage and burn policies are evaluated separately.",
		Query:      NativeQuery{Numerator: strip(s.GoodQuery), Denominator: strip(s.TotalQuery)},
		Thresholds: []NativeThreshold{{Target: s.Target * 100, Timeframe: fmt.Sprintf("%dd", s.WindowSeconds/86400)}},
		Tags:       []string{"nines_owner:" + owner, "nines_origin:" + origin, "nines_revision:" + revision},
	}, Warnings: []string{
		"Datadog evaluates this native count ratio independently. Nines coverage, missing-data handling and burn decisions remain separate.",
		"Each definition revision creates an immutable native SLO. Existing revisions are retained; native burn-rate monitors and notifications are not created.",
		"Datadog does not provide a create idempotency key or conditional updates for this endpoint. Use one publisher per ownership scope. If a create outcome is ambiguous, inspect Datadog before refreshing or retrying; Nines never automatically retries a create.",
	}}
	metrics := map[string]bool{}
	for _, q := range []string{s.GoodQuery, s.TotalQuery} {
		metric, err := queryspec.DatadogCount(q, s.StepSeconds)
		if err != nil {
			return p, err
		}
		if !metrics[metric] {
			if err = c.metricType(ctx, metric, s.StepSeconds); err != nil {
				return p, err
			}
			metrics[metric] = true
		}
	}
	current, err := c.findOrigin(ctx, p)
	if err != nil {
		return p, err
	}
	if current != nil {
		p.NativeID = current.ID
		p.URL = c.NativeURL(current.ID)
		p.PriorDigest = nativeDigest(current.NativeDocument)
		p.Matches = true
	}
	return p, nil
}
func nativeDigest(d NativeDocument) string {
	d.Tags = append([]string(nil), d.Tags...)
	sort.Strings(d.Tags)
	return model.DefinitionDigest(d)
}
func (c *Client) nativeRequest(ctx context.Context, method, path string, params url.Values, body any, out any) error {
	var b []byte
	var err error
	if body != nil {
		b, err = json.Marshal(body)
		if err != nil {
			return model.ErrUnavailable
		}
	}
	u := c.endpoint + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(b))
	if err != nil {
		return model.ErrUnavailable
	}
	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", c.applicationKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return model.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return model.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("datadog native API returned HTTP %d", resp.StatusCode)
	}
	if out != nil && json.Unmarshal(data, out) != nil {
		return model.ErrUnavailable
	}
	return nil
}

// OwnedRevisions scans a bounded, complete tag-filtered result. Do not accept a
// truncated list as proof of absence, and never adopt a name-only match.
func (c *Client) OwnedRevisions(ctx context.Context, owner string) ([]NativeResource, error) {
	if len(owner) != 40 || strings.Trim(owner, "0123456789abcdef") != "" {
		return nil, model.Invalid("invalid Datadog ownership scope")
	}
	rows := []NativeResource{}
	seen := map[string]bool{}
	for offset := 0; offset < 1000; offset += 100 {
		var response struct {
			Data   []NativeResource `json:"data"`
			Errors []string         `json:"errors"`
		}
		err := c.nativeRequest(ctx, http.MethodGet, "/api/v1/slo", url.Values{"tags_query": {"nines_owner:" + owner}, "limit": {"100"}, "offset": {strconv.Itoa(offset)}}, nil, &response)
		if err != nil {
			return nil, err
		}
		if len(response.Errors) > 0 || len(response.Data) > 100 {
			return nil, model.ErrUnavailable
		}
		for _, v := range response.Data {
			if !nativeIDPattern.MatchString(v.ID) || seen[v.ID] || !hasTag(v.Tags, "nines_owner:"+owner) {
				return nil, model.ErrUnavailable
			}
			seen[v.ID] = true
			rows = append(rows, v)
		}
		if len(response.Data) < 100 {
			return rows, nil
		}
	}
	return nil, model.Invalid("Datadog native ownership lookup exceeds 1000 resources; absence cannot be verified")
}
func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
func (c *Client) findOrigin(ctx context.Context, p Publication) (*NativeResource, error) {
	rows, err := c.OwnedRevisions(ctx, p.Owner)
	if err != nil {
		return nil, err
	}
	var found *NativeResource
	for _, v := range rows {
		if !hasTag(v.Tags, "nines_origin:"+p.Origin) && !strings.Contains(v.Name, "["+p.Origin+"]") {
			continue
		}
		if found != nil || nativeDigest(v.NativeDocument) != nativeDigest(p.Document) {
			return nil, model.Invalid("Datadog native revision has duplicate, edited or conflicting content; inspect provider resources")
		}
		copy := v
		found = &copy
	}
	return found, nil
}
func (c *Client) readNative(ctx context.Context, id string) (NativeResource, error) {
	var response struct {
		Data   NativeResource `json:"data"`
		Errors []string       `json:"errors"`
	}
	if !nativeIDPattern.MatchString(id) {
		return response.Data, model.Invalid("invalid Datadog SLO ID")
	}
	if err := c.nativeRequest(ctx, http.MethodGet, "/api/v1/slo/"+id, nil, nil, &response); err != nil {
		return response.Data, err
	}
	if response.Data.ID != id || len(response.Errors) > 0 {
		return response.Data, model.ErrUnavailable
	}
	return response.Data, nil
}

// Publish never updates or deletes a resource. A failed POST is not retried.
func (c *Client) Publish(ctx context.Context, p Publication) (NativeResource, string, error) {
	current, err := c.findOrigin(ctx, p)
	if err != nil {
		return NativeResource{}, "unverified", err
	}
	if current != nil {
		if p.NativeID != current.ID || p.PriorDigest != nativeDigest(current.NativeDocument) {
			return NativeResource{}, "unverified", model.ErrRevisionConflict
		}
		return *current, "unchanged", nil
	}
	if p.NativeID != "" || p.PriorDigest != "" {
		return NativeResource{}, "unverified", model.ErrRevisionConflict
	}
	var response struct {
		Data   []NativeResource `json:"data"`
		Errors []string         `json:"errors"`
	}
	if err = c.nativeRequest(ctx, http.MethodPost, "/api/v1/slo", nil, p.Document, &response); err != nil {
		return NativeResource{}, "unverified", fmt.Errorf("native create outcome unverified; inspect Datadog before retrying: %w", err)
	}
	if len(response.Data) != 1 || len(response.Errors) > 0 {
		return NativeResource{}, "unverified", model.ErrUnavailable
	}
	saved, err := c.readNative(ctx, response.Data[0].ID)
	if err != nil || nativeDigest(saved.NativeDocument) != nativeDigest(p.Document) {
		return NativeResource{}, "unverified", model.ErrUnavailable
	}
	return saved, "stored", nil
}
