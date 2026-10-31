package datadog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNativeResultsReadOnlyAndMissingEvidence(t *testing.T) {
	for _, change := range []string{"ok", "empty", "missing-pair", "shifted", "warning", "bad-count", "errors", "preview", "budget-missing", "negative-budget"} {
		t.Run(change, func(t *testing.T) {
			c, _ := New("https://api.uk1.datadoghq.com", "test-api", "test-app")
			from := time.Unix(1800000000, 0).UTC()
			to := from.Add(time.Minute)
			c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/slo/"+testNativeID+"/history" || r.URL.Query().Get("apply_correction") != "false" || r.URL.Query().Get("target") != "99" {
					t.Fatal("untrusted native request", r.URL)
				}
				series := map[string]any{"interval": 60, "times": []int64{from.UnixMilli()}, "numerator": map[string]any{"values": []any{95}}, "denominator": map[string]any{"values": []any{100}}}
				overall := map[string]any{"sli_value": 95, "error_budget_remaining": map[string]any{"custom": -400}}
				data := map[string]any{"from_ts": from.Unix(), "to_ts": to.Unix(), "type": "metric", "overall": overall, "series": series}
				res := map[string]any{"data": data}
				switch change {
				case "empty":
					series["numerator"] = map[string]any{"values": []any{0}}
					series["denominator"] = map[string]any{"values": []any{0}}
					overall["sli_value"] = 100
				case "missing-pair":
					series["numerator"] = nil
				case "shifted":
					data["to_ts"] = to.Unix() + 1
				case "warning":
					series["message"] = "partial"
				case "bad-count":
					series["numerator"] = map[string]any{"values": []any{101}}
				case "errors":
					res["errors"] = []any{map[string]any{"error_message": "private"}}
				case "preview":
					overall["preview"] = true
				case "budget-missing":
					delete(overall, "error_budget_remaining")
				}
				b, _ := json.Marshal(res)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}, nil
			})
			out, e := c.NativeResults(context.Background(), testNativeID, .99, from, to)
			switch change {
			case "ok", "negative-budget":
				if e != nil || out.Attainment == nil || *out.Attainment != .95 || out.BudgetRemaining == nil || *out.BudgetRemaining != -4 {
					t.Fatal(out, e)
				}
			case "empty":
				if e != nil || out.Attainment != nil || out.BudgetRemaining != nil || out.State != "no_data" {
					t.Fatal(out, e)
				}
			case "budget-missing":
				if e != nil || out.BudgetRemaining != nil || out.Attainment == nil {
					t.Fatal(out, e)
				}
			default:
				if e == nil {
					t.Fatal("accepted incomplete provider evidence", out)
				}
			}
		})
	}
}
func TestNativeCatalogDraftIsExperimentalAndReadOnly(t *testing.T) {
	c, rows, writes := nativeFixture(t)
	s := nativeSLO()
	p, e := c.Prepare(context.Background(), s, "tenant", "dd", "prod")
	if e != nil {
		t.Fatal(e)
	}
	*rows = append(*rows, NativeResource{ID: testNativeID, NativeDocument: p.Document})
	catalog, e := c.NativeCatalog(context.Background())
	if e != nil || len(catalog) != 1 {
		t.Fatal(catalog, e)
	}
	draft, e := c.NativeDraft(catalog[0], "dd", "imported", "prod", "api")
	if e != nil || draft.Spec.Lifecycle != "experimental" || draft.Spec.Policy != nil || len(draft.Spec.Policies) > 0 || *writes != 0 {
		t.Fatal(draft, e)
	}
	catalog[0].Type = "monitor"
	if _, e = c.NativeDraft(catalog[0], "dd", "imported", "prod", "api"); e == nil {
		t.Fatal("monitor import accepted")
	}
}
