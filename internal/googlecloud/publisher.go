package googlecloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type NativeDocument struct {
	Name          string            `json:"name"`
	DisplayName   string            `json:"displayName"`
	Goal          float64           `json:"goal"`
	RollingPeriod string            `json:"rollingPeriod"`
	UserLabels    map[string]string `json:"userLabels"`
	Indicator     NativeIndicator   `json:"serviceLevelIndicator"`
}
type NativeIndicator struct {
	RequestBased RequestBased `json:"requestBased"`
}
type RequestBased struct {
	Ratio TimeSeriesRatio `json:"goodTotalRatio"`
}
type TimeSeriesRatio struct {
	Good  string `json:"goodServiceFilter"`
	Total string `json:"totalServiceFilter"`
}
type Publication struct {
	Document    NativeDocument `json:"document"`
	Origin      string         `json:"origin"`
	PriorDigest string         `json:"priorDigest,omitempty"`
	Matches     bool           `json:"matches"`
	Warnings    []string       `json:"warnings"`
}

func (c *Client) Prepare(ctx context.Context, s model.SLO, tenant, source, namespace string) (Publication, error) {
	if err := s.Validate(); err != nil {
		return Publication{}, err
	}
	if err := c.ValidateSLO(s); err != nil {
		return Publication{}, err
	}
	if c.service == "" || tenant == "" || source == "" || namespace == "" || s.WindowSeconds%86400 != 0 || s.WindowSeconds > 30*86400 || s.Target > .9999 {
		return Publication{}, model.Invalid("Google Cloud native SLOs require configured serviceID/ownership, a whole-day rolling window up to 30 days and target at most 0.9999")
	}
	owner := model.DefinitionDigest([]string{namespace, tenant, source, c.project, c.service, s.ID})[:40]
	revision := strconv.FormatInt(model.ConfigRevision(s), 10)
	content := model.DefinitionDigest(struct {
		Definition model.SLODefinition
		Revision   string
	}{model.DefinitionFromSLO(s), revision})[:40]
	id := "nines-" + owner[:16] + "-" + content[:24]
	name := "projects/" + c.project + "/services/" + c.service + "/serviceLevelObjectives/" + id
	doc := NativeDocument{Name: name, DisplayName: s.Name, Goal: s.Target, RollingPeriod: fmt.Sprintf("%ds", s.WindowSeconds), UserLabels: map[string]string{"nines_owner": owner, "nines_definition": content, "nines_revision": revision}, Indicator: NativeIndicator{RequestBased: RequestBased{Ratio: TimeSeriesRatio{Good: c.filter(s.GoodQuery), Total: c.filter(s.TotalQuery)}}}}
	p := Publication{Document: doc, Origin: name, Warnings: []string{
		"Google Cloud native SLOs use provider evaluation semantics. Nines DELTA interval counts, coverage and burn decisions remain independent.",
		"This is an immutable native resource per Nines revision. The SLO API has no documented version precondition; Nines does not PATCH or DELETE an existing resource. Previous native revisions remain retained.",
		"Native alert policies are not published by this action. Configure and verify Cloud Monitoring notifications separately; Nines burn policies remain active in Nines.",
	}}
	current, exists, err := c.ReadNative(ctx, name)
	if err != nil {
		return p, err
	}
	if !exists {
		return p, nil
	}
	if model.DefinitionDigest(current) != model.DefinitionDigest(doc) {
		return p, model.Invalid("Google Cloud native SLO content or ownership conflicts with this immutable revision")
	}
	p.PriorDigest = model.DefinitionDigest(current)
	p.Matches = true
	return p, nil
}
func (c *Client) ReadNative(ctx context.Context, name string) (NativeDocument, bool, error) {
	var doc NativeDocument
	prefix := "projects/" + c.project + "/services/" + c.service + "/serviceLevelObjectives/"
	if c.service == "" || len(name) <= len(prefix) || name[:len(prefix)] != prefix || !servicePattern.MatchString(name[len(prefix):]) {
		return doc, false, model.Invalid("Google Cloud native resource is outside the configured project/service")
	}
	code, data, err := c.request(ctx, http.MethodGet, "/v3/"+name, nil, nil)
	if err != nil {
		return doc, false, err
	}
	if code == 404 {
		return doc, false, nil
	}
	if code != 200 || json.Unmarshal(data, &doc) != nil {
		return doc, false, model.ErrUnavailable
	}
	normalized, err := c.normalizeNativeName(doc.Name)
	if err != nil || normalized != name {
		return doc, false, model.ErrUnavailable
	}
	doc.Name = normalized
	return doc, true, nil
}
func (c *Client) Publish(ctx context.Context, p Publication) (string, error) {
	current, exists, err := c.ReadNative(ctx, p.Origin)
	if err != nil {
		return "", err
	}
	if exists {
		if p.PriorDigest == "" || model.DefinitionDigest(current) != p.PriorDigest || model.DefinitionDigest(current) != model.DefinitionDigest(p.Document) {
			return "", model.ErrRevisionConflict
		}
		return "unchanged", nil
	}
	if p.PriorDigest != "" {
		return "", model.ErrRevisionConflict
	}
	prefix := "projects/" + c.project + "/services/" + c.service + "/serviceLevelObjectives/"
	id := p.Origin[len(prefix):]
	code, _, err := c.request(ctx, http.MethodPost, "/v3/projects/"+c.project+"/services/"+c.service+"/serviceLevelObjectives", url.Values{"serviceLevelObjectiveId": {id}}, p.Document)
	if err != nil || code < 200 || code >= 300 {
		return "", fmt.Errorf("google cloud native publication outcome is unverified; inspect and refresh the plan: %w", model.ErrUnavailable)
	}
	saved, found, err := c.ReadNative(ctx, p.Origin)
	if err != nil || !found || model.DefinitionDigest(saved) != model.DefinitionDigest(p.Document) {
		return "", model.ErrUnavailable
	}
	return "stored", nil
}

// NativeHealth reads provider evidence for this exact native resource. It never
// turns a percentage into event counts or imports it into Nines' SLI engine.
func (c *Client) NativeHealth(ctx context.Context, name string, now time.Time) (float64, time.Time, error) {
	if _, exists, err := c.ReadNative(ctx, name); err != nil || !exists {
		return 0, time.Time{}, model.ErrUnavailable
	}
	params := url.Values{"filter": {"select_slo_health(" + strconv.Quote(name) + ")"}, "interval.startTime": {now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)}, "interval.endTime": {now.UTC().Format(time.RFC3339)}, "view": {"FULL"}, "pageSize": {"100"}}
	code, data, err := c.request(ctx, http.MethodGet, "/v3/projects/"+c.project+"/timeSeries", params, nil)
	if err != nil || code != 200 {
		return 0, time.Time{}, model.ErrUnavailable
	}
	var response struct {
		TimeSeries  []series          `json:"timeSeries"`
		Next        string            `json:"nextPageToken"`
		Errors      []json.RawMessage `json:"executionErrors"`
		Error       json.RawMessage   `json:"error"`
		Warnings    []json.RawMessage `json:"warnings"`
		Unreachable []string          `json:"unreachable"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.TimeSeries) != 1 || response.Next != "" || len(response.Errors) > 0 || len(response.Error) > 0 || len(response.Unreachable) > 0 || len(response.Warnings) > 0 {
		return 0, time.Time{}, model.ErrUnavailable
	}
	s := response.TimeSeries[0]
	if s.Kind != "GAUGE" || s.ValueType != "DOUBLE" || len(s.Points) == 0 || len(s.Points) > 100 {
		return 0, time.Time{}, model.ErrUnavailable
	}
	var latest time.Time
	value := 0.0
	seen := map[int64]bool{}
	for _, p := range s.Points {
		at, err := time.Parse(time.RFC3339Nano, p.Interval.End)
		if err != nil || at.After(now) || at.Before(now.Add(-10*time.Minute)) || p.Value.Double == nil || p.Value.Int64 != nil || !model.Finite(*p.Value.Double) || *p.Value.Double < 0 || *p.Value.Double > 1 || seen[at.UnixNano()] {
			return 0, time.Time{}, model.ErrUnavailable
		}
		seen[at.UnixNano()] = true
		if at.After(latest) {
			latest = at
			value = *p.Value.Double
		}
	}
	if now.Sub(latest) > 5*time.Minute {
		return 0, time.Time{}, model.ErrUnavailable
	}
	return value, latest, nil
}

// OwnedRevisions lists the entire configured service before confirming cleanup.
// Name-prefix matches remain blockers even if somebody changed ownership labels.
func (c *Client) OwnedRevisions(ctx context.Context, owner string) ([]string, error) {
	if c.service == "" || len(owner) != 40 {
		return nil, model.Invalid("native cleanup requires a configured ownership scope")
	}
	prefix := "projects/" + c.project + "/services/" + c.service + "/serviceLevelObjectives/"
	ownedPrefix := prefix + "nines-" + owner[:16] + "-"
	out := []string{}
	page := ""
	pages := map[string]bool{}
	names := map[string]bool{}
	for n := 0; n < 32; n++ {
		params := url.Values{"pageSize": {"100"}}
		if page != "" {
			params.Set("pageToken", page)
		}
		code, data, err := c.request(ctx, http.MethodGet, "/v3/projects/"+c.project+"/services/"+c.service+"/serviceLevelObjectives", params, nil)
		if err != nil || code != 200 {
			return nil, model.ErrUnavailable
		}
		var response struct {
			SLOs        []NativeDocument  `json:"serviceLevelObjectives"`
			Next        string            `json:"nextPageToken"`
			Unreachable []string          `json:"unreachable"`
			Errors      []json.RawMessage `json:"executionErrors"`
			Error       json.RawMessage   `json:"error"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Error) > 0 || len(response.Unreachable) > 0 || len(response.Errors) > 0 {
			return nil, model.ErrUnavailable
		}
		for _, doc := range response.SLOs {
			normalized, err := c.normalizeNativeName(doc.Name)
			if err != nil {
				return nil, err
			}
			doc.Name = normalized
			if !strings.HasPrefix(doc.Name, prefix) || names[doc.Name] {
				return nil, model.ErrUnavailable
			}
			names[doc.Name] = true
			if doc.UserLabels["nines_owner"] == owner || strings.HasPrefix(doc.Name, ownedPrefix) {
				out = append(out, doc.Name)
			}
		}
		if response.Next == "" {
			return out, nil
		}
		if len(response.Next) > 8192 || pages[response.Next] {
			return nil, model.ErrUnavailable
		}
		page = response.Next
		pages[page] = true
	}
	return nil, model.ErrUnavailable
}

// Google can return a canonical project number for a request using its project
// ID. Trust that alias only on the fixed project-scoped API response, preserve
// the exact service/resource IDs, and reject mixed canonical projects.
func (c *Client) normalizeNativeName(name string) (string, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "services" || parts[3] != c.service || parts[4] != "serviceLevelObjectives" || !servicePattern.MatchString(parts[5]) {
		return "", model.ErrUnavailable
	}
	project := parts[1]
	if project != c.project {
		if strings.Trim(c.project, "0123456789") == "" || strings.Trim(project, "0123456789") != "" || !projectPattern.MatchString(project) {
			return "", model.ErrUnavailable
		}
		c.canonicalMu.Lock()
		defer c.canonicalMu.Unlock()
		if c.canonicalProject != "" && c.canonicalProject != project {
			return "", model.ErrUnavailable
		}
		c.canonicalProject = project
	}
	return "projects/" + c.project + "/services/" + c.service + "/serviceLevelObjectives/" + parts[5], nil
}
