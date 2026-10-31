package newrelic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

const testEntity = "MTIzNDU2fFdPUktMT0FEfHRlc3Q="
const testIndicatorGUID = "MTIzNDU2fEVYVHxTRVJWSUNFX0xFVkVMfDE="

func nativeSLO() model.SLO { s := slo(); s.WindowSeconds = 86400; return s }
func resource(doc NativeDocument) NativeResource {
	b, _ := json.Marshal(map[string]any{"id": "test-indicator-id", "guid": testIndicatorGUID, "entityGuid": testEntity, "name": doc.Name, "description": doc.Description, "objectives": doc.Objectives, "events": map[string]any{"account": map[string]any{"id": 123456}, "goodEvents": doc.Events.GoodEvents, "validEvents": doc.Events.ValidEvents}, "resultQueries": map[string]any{"goodEvents": map[string]any{"nrql": "FROM Metric SELECT sum(newrelic.sli.good)"}, "validEvents": map[string]any{"nrql": "FROM Metric SELECT sum(newrelic.sli.valid)"}, "indicator": map[string]any{"nrql": "FROM Metric SELECT sum(newrelic.sli.good)/sum(newrelic.sli.valid)"}}})
	var row NativeResource
	_ = json.Unmarshal(b, &row)
	return row
}
func nativeFixture(t *testing.T) (*Client, *[]NativeResource, *int) {
	t.Helper()
	c, err := New("https://api.eu.newrelic.com/graphql", "123456", "synthetic-user")
	if err != nil {
		t.Fatal(err)
	}
	rows := []NativeResource{}
	writes := 0
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != c.endpoint || r.Header.Get("API-Key") != "synthetic-user" {
			t.Fatal("route/credential changed")
		}
		var req struct {
			Query     string                     `json:"query"`
			Variables map[string]json.RawMessage `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		var entity string
		if err := json.Unmarshal(req.Variables["entity"], &entity); err != nil || entity != testEntity {
			t.Fatal("unreviewed entity")
		}
		var data any
		switch req.Query {
		case nativeReadDocument:
			data = map[string]any{"actor": map[string]any{"entity": map[string]any{"guid": entity, "accountId": 123456, "serviceLevel": map[string]any{"indicators": rows}}}}
		case nativeCreateDocument:
			var doc NativeDocument
			if err := json.Unmarshal(req.Variables["indicator"], &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Events.AccountID != 123456 {
				t.Fatal("unreviewed event account")
			}
			row := resource(doc)
			rows = append(rows, row)
			writes++
			data = map[string]any{"serviceLevelCreate": row}
		default:
			t.Fatal("unexpected native operation", req.Query)
		}
		b, err := json.Marshal(map[string]any{"data": data})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(b))), Request: r}, nil
	})
	return c, &rows, &writes
}
func TestNativePublicationMappingReadbackAndReuse(t *testing.T) {
	c, _, writes := nativeFixture(t)
	ctx := context.Background()
	s := nativeSLO()
	p, err := c.Prepare(ctx, s, "tenant", "nr", "production", testEntity)
	if err != nil {
		t.Fatal(err)
	}
	if p.Document.Events.AccountID != 123456 || p.Document.Events.GoodEvents.From != "NinesRequest" || p.Document.Events.GoodEvents.Where != "service = 'api' AND success = true" || p.Document.Events.ValidEvents.Where != "service = 'api'" || p.Document.Objectives[0].Target != 99 || p.Document.Objectives[0].TimeWindow.Rolling.Count != 1 {
		t.Fatal(p)
	}
	saved, state, err := c.Publish(ctx, p)
	if err != nil || state != "stored" || saved.GUID != testIndicatorGUID || *writes != 1 {
		t.Fatal(saved, state, err)
	}
	if _, _, err = c.Publish(ctx, p); !errors.Is(err, model.ErrRevisionConflict) || *writes != 1 {
		t.Fatal("stale plan repeated create", err)
	}
	current, err := c.Prepare(ctx, s, "tenant", "nr", "production", testEntity)
	if err != nil || !current.Matches || current.NativeGUID != testIndicatorGUID || current.ResultQueries == nil || current.ResultQueries.GoodEvents.AccountID != 123456 || !strings.HasPrefix(current.URL, "https://one.eu.newrelic.com/") {
		t.Fatal(current, err)
	}
	_, state, err = c.Publish(ctx, current)
	if err != nil || state != "unchanged" || *writes != 1 {
		t.Fatal(state, err)
	}
}
func TestNativeUnsupportedWindowsAndPrecision(t *testing.T) {
	c, _, _ := nativeFixture(t)
	for _, days := range []int64{1, 7, 14, 28, 30} {
		s := nativeSLO()
		s.WindowSeconds = days * 86400
		if _, err := c.Prepare(context.Background(), s, "tenant", "nr", "prod", testEntity); err != nil {
			t.Fatal(days, err)
		}
	}
	for _, seconds := range []int64{120, 2 * 86400, 31 * 86400} {
		s := nativeSLO()
		s.WindowSeconds = seconds
		if _, err := c.Prepare(context.Background(), s, "tenant", "nr", "prod", testEntity); err == nil {
			t.Fatal("invalid window", seconds)
		}
	}
	s := nativeSLO()
	s.Target = .999999999
	if _, err := c.Prepare(context.Background(), s, "tenant", "nr", "prod", testEntity); err == nil {
		t.Fatal("target precision silently changed")
	}
	s = nativeSLO()
	s.GoodQuery = "SELECT percentile(duration,95) FROM Transaction"
	if _, err := c.Prepare(context.Background(), s, "tenant", "nr", "prod", testEntity); err == nil {
		t.Fatal("non count native query")
	}
}
func TestNativeOwnershipDriftAndDuplicateRefusal(t *testing.T) {
	for _, kind := range []string{"query", "target", "duplicate", "facets", "badEvents", "select", "threshold", "eventAccount"} {
		t.Run(kind, func(t *testing.T) {
			c, rows, writes := nativeFixture(t)
			p, err := c.Prepare(context.Background(), nativeSLO(), "tenant", "nr", "prod", testEntity)
			if err != nil {
				t.Fatal(err)
			}
			row := resource(p.Document)
			switch kind {
			case "query":
				row.Events.GoodEvents.Where = "success = false"
			case "target":
				row.Objectives[0].Target = 90
			case "duplicate":
				copy := resource(p.Document)
				copy.ID = "second"
				copy.GUID = "c2Vjb25k"
				*rows = append(*rows, copy)
			case "facets":
				row.Events.GoodEvents.Facets = []string{"service"}
			case "badEvents":
				row.Events.BadEvents = &NativeEventQuery{From: "NinesRequest"}
			case "select":
				row.Events.GoodEvents.Select = &NativeSelect{Function: "SUM", Attribute: "value"}
			case "threshold":
				v := 1.
				row.Events.GoodEvents.Select = &NativeSelect{Function: "COUNT", Threshold: &v}
			case "eventAccount":
				row.Events.Account.ID = 654321
			}
			*rows = append(*rows, row)
			if _, err = c.Prepare(context.Background(), nativeSLO(), "tenant", "nr", "prod", testEntity); err == nil || *writes != 0 {
				t.Fatal("conflicting native resource adopted", err)
			}
		})
	}
}
func TestNativeRejectsWrongParentAccountPartialAndMissingEntity(t *testing.T) {
	for _, body := range []string{
		`{"data":{"actor":{"entity":{"guid":"` + testEntity + `","accountId":654321,"serviceLevel":{"indicators":[]}}}}}`,
		`{"data":{"actor":{"entity":null}}}`,
		`{"errors":[{"message":"synthetic-user private query"}],"data":{"actor":{"entity":{"guid":"` + testEntity + `","accountId":123456,"serviceLevel":{"indicators":[]}}}}}`,
	} {
		c, _, _ := nativeFixture(t)
		c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
		})
		if _, err := c.Prepare(context.Background(), nativeSLO(), "tenant", "nr", "prod", testEntity); err == nil || strings.Contains(err.Error(), "synthetic-user") || strings.Contains(err.Error(), "private query") {
			t.Fatal("unsafe or leaking response", err)
		}
	}
}
func TestNativeLostCreateResponseRecoversWithoutAnotherMutation(t *testing.T) {
	c, _, writes := nativeFixture(t)
	ctx := context.Background()
	s := nativeSLO()
	p, err := c.Prepare(ctx, s, "tenant", "nr", "prod", testEntity)
	if err != nil {
		t.Fatal(err)
	}
	base := c.http.Transport
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		r.Body = io.NopCloser(strings.NewReader(string(b)))
		res, err := base.RoundTrip(r)
		if strings.Contains(string(b), "mutation NinesNativeSLOCreate") {
			if res != nil {
				_ = res.Body.Close()
			}
			return nil, fmt.Errorf("synthetic-user lost response")
		}
		return res, err
	})
	_, state, err := c.Publish(ctx, p)
	if err != nil || state != "stored" || *writes != 1 {
		t.Fatal(state, err)
	}
	c.http.Transport = base
	current, err := c.Prepare(ctx, s, "tenant", "nr", "prod", testEntity)
	if err != nil || !current.Matches {
		t.Fatal(current, err)
	}
	_, state, err = c.Publish(ctx, current)
	if err != nil || state != "unchanged" || *writes != 1 {
		t.Fatal(state, err)
	}
}

func TestNativeNewWorkloadNullListIsEmptyButOmittedListIsUnverified(t *testing.T) {
	for _, tc := range []struct {
		field    string
		accepted bool
	}{{`"indicators":null`, true}, {`"indicators":[]`, true}, {`"unexpected":null`, false}} {
		c, _, _ := nativeFixture(t)
		body := `{"data":{"actor":{"entity":{"guid":"` + testEntity + `","accountId":123456,"serviceLevel":{` + tc.field + `}}}}}`
		c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
		})
		_, err := c.Prepare(context.Background(), nativeSLO(), "tenant", "nr", "prod", testEntity)
		if (err == nil) != tc.accepted {
			t.Fatal(tc, err)
		}
	}
}

func TestNativeUnreadableAfterAmbiguousCreateStaysUnverified(t *testing.T) {
	c, _, writes := nativeFixture(t)
	ctx := context.Background()
	p, err := c.Prepare(ctx, nativeSLO(), "tenant", "nr", "prod", testEntity)
	if err != nil {
		t.Fatal(err)
	}
	base := c.http.Transport
	committed := false
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if committed {
			return nil, fmt.Errorf("synthetic-user backend unavailable")
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		r.Body = io.NopCloser(strings.NewReader(string(b)))
		res, err := base.RoundTrip(r)
		if strings.Contains(string(b), "mutation NinesNativeSLOCreate") {
			committed = true
			if res != nil {
				_ = res.Body.Close()
			}
			return nil, fmt.Errorf("synthetic-user response lost")
		}
		return res, err
	})
	_, state, err := c.Publish(ctx, p)
	if err == nil || state != "unverified" || *writes != 1 || strings.Contains(err.Error(), "synthetic-user") {
		t.Fatal(state, err)
	}
}

func TestNativeLinksKeepConfiguredRegion(t *testing.T) {
	for _, tc := range []struct{ api, ui string }{{"api.newrelic.com", "one.newrelic.com"}, {"api.eu.newrelic.com", "one.eu.newrelic.com"}, {"api.jp.newrelic.com", "one.jp.newrelic.com"}} {
		c, err := New("https://"+tc.api+"/graphql", "123456", "synthetic-user")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(c.NativeURL(testIndicatorGUID), "https://"+tc.ui+"/redirect/entity/") || c.NativeURL("bad guid") != "" {
			t.Fatal("native link escaped region")
		}
	}
}
