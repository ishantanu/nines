package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestTelemetryPinnedMimirAuthAndQueries(t *testing.T) {
	at := time.Unix(1800000000, 0)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Scope-OrgID") != "trusted-org" || r.Header.Get("Authorization") != "Bearer server-token" {
			t.Error("lost configured credentials")
		}
		q := r.URL.Query()
		data := any(nil)
		switch r.URL.Path {
		case "/prometheus/api/v1/label/__name__/values":
			if q.Get("limit") != "101" || !strings.Contains(q.Get("match[]"), "http_") {
				t.Error(q)
			}
			data = []string{"http_requests_total"}
		case "/prometheus/api/v1/metadata":
			if q.Get("metric") != "http_requests_total" {
				t.Error(q)
			}
			data = map[string]any{"http_requests_total": []any{map[string]string{"type": "counter", "help": "Untrusted description", "unit": ""}}}
		case "/prometheus/api/v1/series":
			if q.Get("match[]") != `http_requests_total{service="api\"} or vector(1)"}` || q.Get("start") != "1799999100" || q.Get("limit") != "101" {
				t.Error(q)
			}
			data = []map[string]string{{"__name__": "http_requests_total", "service": `api"} or vector(1)`, "status": "200"}}
		case "/prometheus/api/v1/query":
			if !strings.HasPrefix(q.Get("query"), "topk(101, sum by (client,server,connection_type) (increase(traces_service_graph_request_total{") || q.Get("timeout") != "5s" {
				t.Error(q)
			}
			data = map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"client": "api", "server": "db", "connection_type": "database"}, "value": []any{at.Unix(), "42.5"}}}}
		default:
			t.Error(r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
	}))
	defer server.Close()
	c, err := NewMimir(server.URL+"/prometheus", "trusted-org", "server-token")
	if err != nil {
		t.Fatal(err)
	}
	names, truncated, err := c.MetricNames(context.Background(), "http_", at)
	if err != nil || truncated || len(names) != 1 {
		t.Fatal(names, truncated, err)
	}
	d, err := c.Discover(context.Background(), "http_requests_total", map[string]string{"service": `api"} or vector(1)`}, at)
	if err != nil || len(d.Series) != 1 {
		t.Fatal(d, err)
	}
	edges, truncated, err := c.ObserveDependencies(context.Background(), v1.GraphObservation{WindowSeconds: 900, Labels: map[string]string{"env": "prod"}}, at)
	if err != nil || truncated || len(edges) != 1 || edges[0].Requests != 42.5 {
		t.Fatal(edges, truncated, err)
	}
	if calls != 4 {
		t.Fatal(calls)
	}
	if _, _, err = c.MetricNames(context.Background(), ".*", at); err == nil {
		t.Fatal("query prefix accepted")
	}
	if calls != 4 {
		t.Fatal("invalid input called provider")
	}
}
func TestTelemetryRejectsPartialOversizedAndStaleEvidence(t *testing.T) {
	for _, scenario := range []string{"warnings", "too-many", "wrong-series", "invalid-count", "stale-vector", "bad-json"} {
		t.Run(scenario, func(t *testing.T) {
			at := time.Unix(1800000000, 0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "bad-json" {
					_, _ = w.Write([]byte(`{`))
					return
				}
				data := any(map[string]any{"http_requests_total": []any{map[string]string{"type": "counter"}}})
				warnings := []string{}
				if scenario == "warnings" {
					warnings = []string{"partial storage result"}
				}
				if strings.HasSuffix(r.URL.Path, "/series") {
					series := []map[string]string{{"__name__": "http_requests_total"}}
					if scenario == "too-many" {
						series = make([]map[string]string, 102)
					}
					if scenario == "wrong-series" {
						series[0]["__name__"] = "other"
					}
					data = series
				}
				if strings.HasSuffix(r.URL.Path, "/query") {
					value := "NaN"
					timestamp := at.Unix()
					if scenario == "stale-vector" {
						value = "4"
						timestamp--
					}
					data = map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"client": "api", "server": "db"}, "value": []any{timestamp, value}}}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "warnings": warnings, "data": data})
			}))
			defer server.Close()
			c, _ := New(server.URL, "")
			var err error
			if scenario == "invalid-count" || scenario == "stale-vector" {
				_, _, err = c.ObserveDependencies(context.Background(), v1.GraphObservation{WindowSeconds: 900}, at)
			} else {
				_, err = c.Discover(context.Background(), "http_requests_total", nil, at)
			}
			if err == nil {
				t.Fatal("unsafe evidence accepted")
			}
		})
	}
}

func TestHistogramSeriesDiscoveryResolvesMetadataFamily(t *testing.T) {
	at := time.Unix(1800000000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch r.URL.Path {
		case "/api/v1/metadata":
			data = map[string]any{}
			if r.URL.Query().Get("metric") == "duration_seconds" {
				data = map[string]any{"duration_seconds": []map[string]string{{"type": "histogram", "unit": "seconds"}}}
			}
		case "/api/v1/series":
			if len(r.URL.Query()["match[]"]) != 2 || !strings.Contains(r.URL.Query().Get("match[]"), "duration_seconds_bucket") {
				t.Error(r.URL.Query())
			}
			data = []map[string]string{{"__name__": "duration_seconds_bucket", "le": "0.3"}, {"__name__": "duration_seconds_count"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
	}))
	defer server.Close()
	c, _ := New(server.URL, "")
	d, err := c.Discover(context.Background(), "duration_seconds_bucket", nil, at)
	if err != nil || d.Metric != "duration_seconds" || len(d.Series) != 2 || len(d.Metadata) != 1 {
		t.Fatal(d, err)
	}
}

func TestMetricNamesEmptyPrefixIsBoundedAndTruncated(t *testing.T) {
	for _, amount := range []int{0, 101, 102} {
		t.Run(strconv.Itoa(amount), func(t *testing.T) {
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("match[]") != `{__name__=~".+"}` || q.Get("limit") != "101" || q.Get("end") == "" || q.Get("start") == "" {
					t.Error(q)
				}
				names := []string{}
				for i := 0; i < amount; i++ {
					names = append(names, fmt.Sprintf("requests_%03d_total", i))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": names})
			}))
			defer source.Close()
			c, err := New(source.URL, "")
			if err != nil {
				t.Fatal(err)
			}
			names, truncated, err := c.MetricNames(context.Background(), "", time.Now())
			if amount > 101 {
				if err == nil {
					t.Fatal("unbounded response accepted")
				}
				return
			}
			if err != nil || len(names) != min(100, amount) || truncated != (amount > 100) {
				t.Fatal(names, truncated, err)
			}
		})
	}
}
