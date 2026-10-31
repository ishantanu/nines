package newrelic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestNativeResultsUseOnlyPinnedMetricQuery(t *testing.T) {
	for _, change := range []string{"ok", "empty", "foreign-account", "partial", "missing", "bad-count", "injected-query"} {
		t.Run(change, func(t *testing.T) {
			from, to := bounds()
			row := NativeResource{ID: "123", GUID: testIndicatorGUID}
			row.ResultQueries.GoodEvents.NRQL = fmt.Sprintf("SELECT sum(newrelic.sli.good) AS 'Good' FROM Metric WHERE entity.guid = '%s' UNTIL 2 minutes AGO", row.GUID)
			row.ResultQueries.ValidEvents.NRQL = fmt.Sprintf("SELECT sum(newrelic.sli.valid) AS 'Valid' FROM Metric WHERE entity.guid = '%s' UNTIL 2 minutes AGO", row.GUID)
			if change == "injected-query" {
				row.ResultQueries.GoodEvents.NRQL += " FACET secret"
			}
			c, _ := New("https://api.eu.newrelic.com/graphql", "123456", "synthetic")
			calls := 0
			c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
				calls++
				var req struct {
					Query     string         `json:"query"`
					Variables map[string]any `json:"variables"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.Query != countDocument || req.Variables["account"] != float64(123456) || !strings.Contains(req.Variables["good"].(string), "SELECT sum(newrelic.sli.good) AS nines_count FROM Metric WHERE entity.guid = '") || strings.Contains(req.Query, "mutation") {
					t.Fatal(req)
				}
				account := map[string]any{"id": 123456}
				for _, alias := range []string{"good", "total"} {
					count := 100.
					if alias == "good" {
						count = 95
					}
					if change == "empty" {
						count = 0
					}
					if change == "bad-count" && alias == "good" {
						count = 101
					}
					b := []any{map[string]any{"beginTimeSeconds": from.Unix(), "endTimeSeconds": from.Unix() + 60, "nines_count": count}, map[string]any{"beginTimeSeconds": from.Unix() + 60, "endTimeSeconds": to.Unix(), "nines_count": count}}
					if change == "missing" && alias == "good" {
						b = b[:1]
					}
					account[alias] = map[string]any{"results": b, "metadata": map[string]any{"messages": []any{}, "timeWindow": map[string]any{"begin": from.UnixMilli(), "end": to.UnixMilli()}}}
				}
				if change == "foreign-account" {
					account["id"] = 654321
				}
				res := map[string]any{"data": map[string]any{"actor": map[string]any{"account": account}}}
				if change == "partial" {
					res["errors"] = []any{map[string]any{"message": "private"}}
				}
				b, _ := json.Marshal(res)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}, nil
			})
			out, e := c.NativeResults(context.Background(), row, .99, from, to)
			switch change {
			case "ok":
				if e != nil || out.Attainment == nil || *out.Attainment != .95 || out.BudgetRemaining == nil || math.Abs(*out.BudgetRemaining+4) > 1e-10 || len(out.Points) != 2 {
					t.Fatal(out, e)
				}
			case "empty":
				if e != nil || out.State != "no_data" || out.Attainment != nil || out.BudgetRemaining != nil {
					t.Fatal(out, e)
				}
			case "injected-query":
				if e == nil || calls != 0 {
					t.Fatal("provider query executed", e, calls)
				}
			default:
				if e == nil {
					t.Fatal("invalid evidence accepted", out)
				}
			}
		})
	}
}
func TestNativeDraftKeepsCountScopeAndSuppressesAlerts(t *testing.T) {
	c, rows, writes := nativeFixture(t)
	s := nativeSLO()
	p, e := c.Prepare(context.Background(), s, "tenant", "nr", "prod", testEntity)
	if e != nil {
		t.Fatal(e)
	}
	*rows = append(*rows, resource(p.Document))
	catalog, e := c.NativeCatalog(context.Background(), testEntity)
	if e != nil {
		t.Fatal(e)
	}
	d, e := c.NativeDraft(catalog[0], "nr", "imported", "prod", "api")
	if e != nil || d.Spec.Lifecycle != "experimental" || d.Spec.GoodQuery != s.GoodQuery || d.Spec.TotalQuery != s.TotalQuery || *writes != 0 {
		t.Fatal(d, e)
	}
	catalog[0].Events.BadEvents = &NativeEventQuery{From: "Other"}
	if _, e = c.NativeDraft(catalog[0], "nr", "imported", "prod", "api"); e == nil {
		t.Fatal("unsupported bad-event profile imported")
	}
}
