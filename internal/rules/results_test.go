package rules

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/model"
)

func TestNativeResultsRequireCurrentOwnedContentAndDoNotPublish(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	var rows []datadog.NativeResource
	histories := 0
	from := time.Unix(1800000000, 0).UTC()
	to := from.Add(time.Minute)
	http.DefaultTransport = dash0Transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("read wrote provider", r.Method)
		}
		w := httptest.NewRecorder()
		var data any
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/metrics/"):
			data = map[string]any{"type": "count"}
		case r.URL.Path == "/api/v1/slo":
			data = map[string]any{"data": rows}
		case strings.HasSuffix(r.URL.Path, "/history"):
			histories++
			data = map[string]any{"data": map[string]any{"from_ts": from.Unix(), "to_ts": to.Unix(), "type": "metric", "overall": map[string]any{"sli_value": 95}, "series": map[string]any{"interval": 60, "times": []int64{from.UnixMilli()}, "numerator": map[string]any{"values": []float64{95}}, "denominator": map[string]any{"values": []float64{100}}}}}
		default:
			t.Fatal("unexpected provider read", r.URL.Path)
		}
		if e := json.NewEncoder(w).Encode(data); e != nil {
			t.Fatal(e)
		}
		return w.Result(), nil
	})
	s := fixture()
	s.ProviderWindow = nil
	s.Source = "dd"
	s.QueryLanguage = "datadog"
	s.StepSeconds = 60
	s.WindowSeconds = 7 * 86400
	s.GoodQuery = "sum:api.good{env:prod}.as_count().rollup(sum,60)"
	s.TotalQuery = "sum:api.total{env:prod}.as_count().rollup(sum,60)"
	m := &Manager{Tenant: "alpha", Targets: map[string]Target{"dd": {Connection: "dd", Kind: "datadog", Namespace: "prod", QueryURL: "https://api.uk1.datadoghq.com", DatadogAPIKey: "synthetic", DatadogApplicationKey: "synthetic"}}}
	ctx := context.Background()
	p, e := m.PlanContext(ctx, s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.NativeResults(ctx, s, from, to); e == nil || histories != 0 {
		t.Fatal("missing revision queried", e)
	}
	rows = []datadog.NativeResource{{ID: "0123456789abcdef0123456789abcdef", NativeDocument: p.NativeDatadog.Document}}
	out, e := m.NativeResults(ctx, s, from, to)
	if e != nil || out.Tenant != "alpha" || out.SLOID != s.ID || out.Revision != model.ConfigRevision(s) || out.Attainment == nil {
		t.Fatal(out, e)
	}
	catalog, e := m.NativeCatalog(ctx, "dd")
	if e != nil || len(catalog.Items) != 1 || !catalog.Items[0].Importable {
		t.Fatal(catalog, e)
	}
	draft, e := m.NativeDraft(ctx, "dd", rows[0].ID, "imported", "prod", "api")
	if e != nil || !strings.Contains(draft.Manifest, "lifecycle: experimental") {
		t.Fatal(draft, e)
	}
	rows[0].Name = "edited by provider"
	before := histories
	if _, e = m.NativeResults(ctx, s, from, to); e == nil || histories != before {
		t.Fatal("drifted revision queried", e)
	}
	if _, e = m.NativeDraft(ctx, "dd", rows[0].ID, "../unsafe", "prod", "api"); e == nil {
		t.Fatal("invalid import metadata accepted")
	}
}
func TestNativeReadRouteIdentityIncludesCredentials(t *testing.T) {
	a := Target{Kind: "datadog", DatadogAPIKey: "a"}
	b := a
	b.DatadogAPIKey = "b"
	if nativeReadRouteDigest(a) == nativeReadRouteDigest(b) {
		t.Fatal("cross-account credential rotation was ignored")
	}
}
