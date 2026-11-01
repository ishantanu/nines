package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/queryspec"
)

type NativeEventQuery struct {
	From   string        `json:"from"`
	Where  string        `json:"where,omitempty"`
	Select *NativeSelect `json:"select,omitempty"`
	Facets []string      `json:"facets,omitempty"`
}
type NativeSelect struct {
	Function  string   `json:"function"`
	Attribute string   `json:"attribute,omitempty"`
	Threshold *float64 `json:"threshold,omitempty"`
}
type NativeEvents struct {
	AccountID   int64            `json:"accountId"`
	ValidEvents NativeEventQuery `json:"validEvents"`
	GoodEvents  NativeEventQuery `json:"goodEvents"`
}
type NativeRolling struct {
	Count int64  `json:"count"`
	Unit  string `json:"unit"`
}
type NativeTimeWindow struct {
	Rolling NativeRolling `json:"rolling"`
}
type NativeObjective struct {
	Target     float64          `json:"target"`
	TimeWindow NativeTimeWindow `json:"timeWindow"`
}
type NativeDocument struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Events      NativeEvents      `json:"events"`
	Objectives  []NativeObjective `json:"objectives"`
}
type ResultQuery struct {
	NRQL      string `json:"nrql"`
	AccountID int64  `json:"accountId"`
}
type NativeQueries struct {
	GoodEvents  ResultQuery `json:"goodEvents"`
	ValidEvents ResultQuery `json:"validEvents"`
	Indicator   ResultQuery `json:"indicator"`
}
type NativeResource struct {
	ID          string            `json:"id"`
	GUID        string            `json:"guid"`
	EntityGUID  string            `json:"entityGuid"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Objectives  []NativeObjective `json:"objectives"`
	Events      *struct {
		Account *struct {
			ID int64 `json:"id"`
		} `json:"account"`
		GoodEvents  *NativeEventQuery `json:"goodEvents"`
		ValidEvents *NativeEventQuery `json:"validEvents"`
		BadEvents   *NativeEventQuery `json:"badEvents"`
	} `json:"events"`
	TimeBasedCriteria json.RawMessage `json:"timeBasedCriteria"`
	ResultQueries     NativeQueries   `json:"resultQueries"`
}
type Publication struct {
	Document      NativeDocument `json:"document"`
	EntityGUID    string         `json:"entityGUID"`
	Origin        string         `json:"origin"`
	Owner         string         `json:"owner"`
	NativeID      string         `json:"nativeID,omitempty"`
	NativeGUID    string         `json:"nativeGUID,omitempty"`
	URL           string         `json:"url,omitempty"`
	PriorDigest   string         `json:"priorDigest,omitempty"`
	Matches       bool           `json:"matches"`
	ResultQueries *NativeQueries `json:"resultQueries,omitempty"`
	Warnings      []string       `json:"warnings"`
}

var entityGUIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_+/=-]{1,512}$`)

func ValidEntityGUID(guid string) bool { return entityGUIDPattern.MatchString(guid) }
func (c *Client) NativeURL(guid string) string {
	if !ValidEntityGUID(guid) {
		return ""
	}
	host := "one.newrelic.com"
	if strings.Contains(c.endpoint, "api.eu.") {
		host = "one.eu.newrelic.com"
	} else if strings.Contains(c.endpoint, "api.jp.") {
		host = "one.jp.newrelic.com"
	}
	return "https://" + host + "/redirect/entity/" + url.PathEscape(guid)
}

// nativeGraph refuses partial responses and never exposes upstream error bodies.
func (c *Client) nativeGraph(ctx context.Context, document string, variables map[string]any, out any) error {
	b, err := json.Marshal(map[string]any{"query": document, "variables": variables})
	if err != nil {
		return model.ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(b))
	if err != nil {
		return model.ErrUnavailable
	}
	req.Header.Set("API-Key", c.userKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return model.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	b, err = io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(b) > 8<<20 {
		return model.ErrUnavailable
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("new relic native API returned HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Errors []json.RawMessage `json:"errors"`
		Data   json.RawMessage   `json:"data"`
	}
	if json.Unmarshal(b, &envelope) != nil || len(envelope.Errors) > 0 || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return model.Invalid("New Relic native operation returned incomplete or GraphQL error evidence; check entity/account permissions, full-platform access and events-to-metrics permissions")
	}
	if json.Unmarshal(envelope.Data, out) != nil {
		return model.ErrUnavailable
	}
	return nil
}

// Query the supported count-based contract only. Some accounts expose optional
// time-based schema fields that fail when resolved; partial responses still fail closed.
const nativeFields = `id guid entityGuid name description objectives {target timeWindow {rolling {count unit}}}
 events {account {id} validEvents {from where facets select {function attribute threshold}} goodEvents {from where facets select {function attribute threshold}} badEvents {from where facets select {function attribute threshold}}}

 resultQueries {goodEvents {nrql} validEvents {nrql} indicator {nrql}}`
const nativeReadDocument = `query NinesNativeSLOs($entity:EntityGuid!){actor{entity(guid:$entity){guid accountId serviceLevel{indicators{` + nativeFields + `}}}}}`
const nativeCreateDocument = `mutation NinesNativeSLOCreate($entity:EntityGuid!,$indicator:ServiceLevelIndicatorCreateInput!){serviceLevelCreate(entityGuid:$entity,indicator:$indicator){id guid entityGuid}}`

func (c *Client) nativeRows(ctx context.Context, entity string) ([]NativeResource, error) {
	if !ValidEntityGUID(entity) {
		return nil, model.Invalid("New Relic native publication requires a configured entityGUID")
	}
	var response struct {
		Actor *struct {
			Entity *struct {
				GUID         string `json:"guid"`
				AccountID    *int64 `json:"accountId"`
				ServiceLevel *struct {
					Indicators json.RawMessage `json:"indicators"`
				} `json:"serviceLevel"`
			} `json:"entity"`
		} `json:"actor"`
	}
	if err := c.nativeGraph(ctx, nativeReadDocument, map[string]any{"entity": entity}, &response); err != nil {
		return nil, err
	}
	if response.Actor == nil || response.Actor.Entity == nil {
		return nil, model.Invalid("New Relic native entity was not found or is inaccessible; configure an existing service or workload")
	}
	e := response.Actor.Entity
	if e.GUID != entity || e.AccountID == nil || *e.AccountID != c.account || e.ServiceLevel == nil || len(e.ServiceLevel.Indicators) == 0 {
		return nil, model.Invalid("New Relic native entity/account or complete service-level list could not be verified")
	}
	// A new workload returns indicators:null. An omitted field remains invalid;
	// partial GraphQL responses were already rejected by nativeGraph.
	rows := []NativeResource{}
	if string(e.ServiceLevel.Indicators) != "null" && json.Unmarshal(e.ServiceLevel.Indicators, &rows) != nil {
		return nil, model.ErrUnavailable
	}
	if len(rows) > 500 {
		return nil, model.ErrUnavailable
	}
	seen := map[string]bool{}
	guids := map[string]bool{}
	for i, v := range rows {
		if v.ID == "" || len(v.ID) > 512 || !ValidEntityGUID(v.GUID) || v.EntityGUID != entity || seen[v.ID] || guids[v.GUID] {
			return nil, model.ErrUnavailable
		}
		seen[v.ID] = true
		guids[v.GUID] = true
		rows[i].ResultQueries.GoodEvents.AccountID = c.account
		rows[i].ResultQueries.ValidEvents.AccountID = c.account
		rows[i].ResultQueries.Indicator.AccountID = c.account
	}
	return rows, nil
}
func (c *Client) OwnedRevisions(ctx context.Context, entity, owner string) ([]NativeResource, error) {
	if len(owner) != 40 || strings.Trim(owner, "0123456789abcdef") != "" {
		return nil, model.Invalid("invalid New Relic ownership scope")
	}
	rows, err := c.nativeRows(ctx, entity)
	if err != nil {
		return nil, err
	}
	out := []NativeResource{}
	for _, row := range rows {
		if strings.Contains(row.Description, "nines_owner="+owner) || strings.Contains(row.Name, "[nines-"+owner[:16]+"-") {
			out = append(out, row)
		}
	}
	return out, nil
}
func nativeDocument(row NativeResource) (NativeDocument, error) {
	if row.Events == nil || row.Events.Account == nil || row.Events.ValidEvents == nil || row.Events.GoodEvents == nil || row.Events.BadEvents != nil || (len(row.TimeBasedCriteria) > 0 && string(row.TimeBasedCriteria) != "null") {
		return NativeDocument{}, model.Invalid("native New Relic SLI uses unsupported event or time criteria")
	}
	normalize := func(q NativeEventQuery) (NativeEventQuery, error) {
		if len(q.Facets) > 0 {
			return q, model.Invalid("native faceted New Relic SLIs are outside this publication subset")
		}
		if q.Select != nil && (q.Select.Threshold != nil || q.Select.Function != "COUNT" || q.Select.Attribute != "" && q.Select.Attribute != "*") {
			return q, model.Invalid("native New Relic selector differs from count(*)")
		}
		q.Select = nil
		q.Facets = nil
		return q, nil
	}
	good, err := normalize(*row.Events.GoodEvents)
	if err != nil {
		return NativeDocument{}, err
	}
	valid, err := normalize(*row.Events.ValidEvents)
	if err != nil {
		return NativeDocument{}, err
	}
	return NativeDocument{Name: row.Name, Description: row.Description, Events: NativeEvents{AccountID: row.Events.Account.ID, GoodEvents: good, ValidEvents: valid}, Objectives: row.Objectives}, nil
}
func (c *Client) findOrigin(ctx context.Context, p Publication) (*NativeResource, error) {
	rows, err := c.OwnedRevisions(ctx, p.EntityGUID, p.Owner)
	if err != nil {
		return nil, err
	}
	var current *NativeResource
	for _, row := range rows {
		if !strings.Contains(row.Description, "nines_origin="+p.Origin) && !strings.Contains(row.Name, "["+p.Origin+"]") {
			continue
		}
		doc, err := nativeDocument(row)
		if err != nil || current != nil || model.DefinitionDigest(doc) != model.DefinitionDigest(p.Document) {
			return nil, model.Invalid("New Relic native revision has duplicate, edited or conflicting content; inspect provider resources")
		}
		// Result queries are provider evidence only; never execute them implicitly.
		for _, q := range []ResultQuery{row.ResultQueries.GoodEvents, row.ResultQueries.ValidEvents, row.ResultQueries.Indicator} {
			if q.AccountID != c.account || q.NRQL == "" || len(q.NRQL) > 16000 {
				return nil, model.ErrUnavailable
			}
		}
		copy := row
		current = &copy
	}
	return current, nil
}
func (c *Client) Prepare(ctx context.Context, s model.SLO, tenant, source, namespace, entity string) (Publication, error) {
	if err := s.Validate(); err != nil {
		return Publication{}, err
	}
	if err := c.ValidateSLO(s); err != nil {
		return Publication{}, err
	}
	days := s.WindowSeconds / 86400
	if tenant == "" || source == "" || namespace == "" || !ValidEntityGUID(entity) || s.WindowSeconds%86400 != 0 || (days != 1 && days != 7 && days != 14 && days != 28 && days != 30) || len(s.Name) > 180 || math.Abs(s.Target*1e7-math.Round(s.Target*1e7)) > 1e-7 {
		return Publication{}, model.Invalid("New Relic native SLOs require configured ownership/entityGUID, exact 1/7/14/28/30 day windows, a display name up to 180 bytes and target percentage with at most five decimals")
	}
	good, _ := queryspec.NewRelicCount(s.GoodQuery, s.StepSeconds)
	valid, _ := queryspec.NewRelicCount(s.TotalQuery, s.StepSeconds)
	owner := model.DefinitionDigest([]string{c.endpoint, strconv.FormatInt(c.account, 10), entity, tenant, source, namespace, s.Project, s.ID})[:40]
	revision := model.ConfigRevision(s)
	definition := model.DefinitionDigest(struct {
		Definition model.SLODefinition
		Revision   int64
	}{model.DefinitionFromSLO(s), revision})[:40]
	origin := "nines-" + owner[:16] + "-" + definition[:24]
	p := Publication{Owner: owner, Origin: origin, EntityGUID: entity, Document: NativeDocument{Name: s.Name + " [" + origin + "]", Description: fmt.Sprintf("Managed by Nines. nines_owner=%s nines_origin=%s nines_revision=%d. Native evaluation is independent from Nines coverage and burn policies.", owner, origin, revision), Events: NativeEvents{AccountID: c.account, GoodEvents: NativeEventQuery{From: good.EventType, Where: good.Where}, ValidEvents: NativeEventQuery{From: valid.EventType, Where: valid.Where}}, Objectives: []NativeObjective{{Target: s.Target * 100, TimeWindow: NativeTimeWindow{Rolling: NativeRolling{Count: days, Unit: "DAY"}}}}}, Warnings: []string{
		"This publishes a native New Relic event-count SLI/SLO on the configured entity. Full-platform access and events-to-metrics management permissions are required.",
		"Native derived metrics start after configuration; previous custom events do not establish native metric history. Inspect the returned New Relic result queries and verify fresh telemetry separately.",
		"Each definition revision creates an immutable native indicator. Previous revisions are retained and count toward New Relic's 500-SLI account limit. Nines never updates or deletes an existing indicator without conditional-write guarantees.",
		"Native alert conditions and notifications are not created. Nines interval counts, coverage and burn alerts remain independent.",
		"Use one publisher per ownership scope. Failed creates are not automatically retried; inspect the provider before refreshing an ambiguous outcome. Independent publishers do not provide exactly-once creation.",
	}}
	row, err := c.findOrigin(ctx, p)
	if err != nil {
		return p, err
	}
	if row != nil {
		p.Matches = true
		p.NativeID = row.ID
		p.NativeGUID = row.GUID
		p.URL = c.NativeURL(row.GUID)
		p.PriorDigest = model.DefinitionDigest(p.Document)
		p.ResultQueries = &row.ResultQueries
	}
	return p, nil
}
func (c *Client) Publish(ctx context.Context, p Publication) (NativeResource, string, error) {
	current, err := c.findOrigin(ctx, p)
	if err != nil {
		return NativeResource{}, "unverified", err
	}
	if current != nil {
		if current.ID != p.NativeID || current.GUID != p.NativeGUID || p.PriorDigest != model.DefinitionDigest(p.Document) {
			return NativeResource{}, "unverified", model.ErrRevisionConflict
		}
		return *current, "unchanged", nil
	}
	if p.NativeID != "" || p.NativeGUID != "" || p.PriorDigest != "" {
		return NativeResource{}, "unverified", model.ErrRevisionConflict
	}
	var response struct {
		Created *struct {
			ID         string `json:"id"`
			GUID       string `json:"guid"`
			EntityGUID string `json:"entityGuid"`
		} `json:"serviceLevelCreate"`
	}
	err = c.nativeGraph(ctx, nativeCreateDocument, map[string]any{"entity": p.EntityGUID, "indicator": p.Document}, &response)
	if err != nil {
		// A mutation can commit before a GraphQL resolver or transport fails.
		// Read the exact reviewed ownership/content once; never repeat the mutation.
		if recovered, readErr := c.findOrigin(ctx, p); readErr == nil && recovered != nil {
			return *recovered, "stored", nil
		}
		return NativeResource{}, "unverified", fmt.Errorf("new relic create outcome unverified; inspect native service levels before retrying: %w", model.ErrUnavailable)
	}
	if response.Created == nil || response.Created.ID == "" || !ValidEntityGUID(response.Created.GUID) || response.Created.EntityGUID != p.EntityGUID {
		return NativeResource{}, "unverified", model.ErrUnavailable
	}
	saved, err := c.findOrigin(ctx, p)
	if err != nil || saved == nil || saved.ID != response.Created.ID || saved.GUID != response.Created.GUID {
		return NativeResource{}, "unverified", model.ErrUnavailable
	}
	return *saved, "stored", nil
}
