package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func slo() model.SLO {
	return model.SLO{ID: "availability", Name: "API availability", Project: "production", Service: "api", Target: .99, WindowSeconds: 120, StepSeconds: 60, QueryLanguage: "datadog", GoodQuery: "sum:api.good{env:prod}.as_count().rollup(sum,60)", TotalQuery: "sum:api.total{env:prod}.as_count().rollup(sum,60)"}
}
func fixture(t *testing.T, change func(*http.Request, map[string]any) int) *Client {
	t.Helper()
	c, err := New("https://api.datadoghq.eu", "synthetic-api-key", "synthetic-application-key")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.datadoghq.eu" || r.Header.Get("DD-API-KEY") != "synthetic-api-key" || r.Header.Get("DD-APPLICATION-KEY") != "synthetic-application-key" {
			t.Fatal("routing or credentials changed")
		}
		body := map[string]any{"type": "count"}
		if r.URL.Path == "/api/v1/query" {
			var start, end int64
			if _, err := fmt.Sscan(r.URL.Query().Get("from"), &start); err != nil {
				t.Fatal(err)
			}
			if _, err := fmt.Sscan(r.URL.Query().Get("to"), &end); err != nil {
				t.Fatal(err)
			}
			value := 100.
			if strings.Contains(r.URL.Query().Get("query"), "api.good") {
				value = 99.
			}
			points := []any{}
			for at := start; at <= end; at += 60 {
				points = append(points, []any{at * 1000, value})
			}
			body = map[string]any{"status": "ok", "res_type": "time_series", "series": []any{map[string]any{"interval": 60000, "aggr": "sum", "pointlist": points}}}
		}
		code := 200
		if change != nil {
			code = change(r, body)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}, Request: r}, nil
	})
	return c
}
func bounds() (time.Time, time.Time) {
	from := time.Unix(1800000000, 0).UTC()
	return from, from.Add(120 * time.Second)
}
func TestCollectCountBuckets(t *testing.T) {
	from, to := bounds()
	c := fixture(t, nil)
	points, err := c.Collect(context.Background(), slo(), from, to)
	if err != nil || len(points) != 2 || !points[0].Time.Equal(from.Add(time.Minute)) || !points[1].Time.Equal(to) || points[0].Good != 99 || points[0].Total != 100 {
		t.Fatal(points, err)
	}
}

// UK trial responses return interval=60 for 60-second buckets even though
// the documented v1 schema specifies milliseconds. Both exact representations
// must work without admitting a finer or coarser upstream rollup.
func TestCollectRollupIntervalUnits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval int64
		accepted bool
	}{
		{"seconds", 60, true},
		{"milliseconds", 60000, true},
		{"coarser_seconds", 300, false},
		{"coarser_milliseconds", 300000, false},
		{"finer_seconds", 30, false},
		{"finer_milliseconds", 30000, false},
		{"zero", 0, false},
		{"negative", -60, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, func(r *http.Request, b map[string]any) int {
				if r.URL.Path == "/api/v1/query" {
					b["series"].([]any)[0].(map[string]any)["interval"] = tc.interval
				}
				return 200
			})
			from, to := bounds()
			points, err := c.Collect(context.Background(), slo(), from, to)
			if tc.accepted {
				if err != nil || len(points) != 2 || points[0].Good != 99 || points[0].Total != 100 {
					t.Fatal(points, err)
				}
			} else if err == nil {
				t.Fatal("unexpected rollup accepted")
			}
		})
	}
}

func TestDatadogRefusesAmbiguousEvidence(t *testing.T) {
	tests := map[string]func(*http.Request, map[string]any) int{
		"gauge": func(r *http.Request, b map[string]any) int {
			if r.URL.Path != "/api/v1/query" {
				b["type"] = "gauge"
			}
			return 200
		},
		"rate limit": func(_ *http.Request, b map[string]any) int {
			b["error"] = "synthetic-api-key private query"
			return 429
		},
		"coarser rollup": func(r *http.Request, b map[string]any) int {
			if r.URL.Path == "/api/v1/query" {
				b["series"].([]any)[0].(map[string]any)["interval"] = 300000
			}
			return 200
		},
		"multiple series": func(r *http.Request, b map[string]any) int {
			if r.URL.Path == "/api/v1/query" {
				b["series"] = append(b["series"].([]any), b["series"].([]any)[0])
			}
			return 200
		},
		"missing numerator": func(r *http.Request, b map[string]any) int {
			if strings.Contains(r.URL.Query().Get("query"), "api.good") {
				b["series"] = []any{}
			}
			return 200
		},
		"null numerator": func(r *http.Request, b map[string]any) int {
			if strings.Contains(r.URL.Query().Get("query"), "api.good") {
				b["series"].([]any)[0].(map[string]any)["pointlist"].([]any)[0].([]any)[1] = nil
			}
			return 200
		},
		"duplicate timestamp": func(r *http.Request, b map[string]any) int {
			if r.URL.Path == "/api/v1/query" {
				series := b["series"].([]any)[0].(map[string]any)
				points := series["pointlist"].([]any)
				series["pointlist"] = append(points, points[0])
			}
			return 200
		},
		"good exceeds total": func(r *http.Request, b map[string]any) int {
			if strings.Contains(r.URL.Query().Get("query"), "api.good") {
				b["series"].([]any)[0].(map[string]any)["pointlist"].([]any)[0].([]any)[1] = 101.
			}
			return 200
		},
		"misalignment": func(r *http.Request, b map[string]any) int {
			if r.URL.Path == "/api/v1/query" {
				b["series"].([]any)[0].(map[string]any)["pointlist"].([]any)[0].([]any)[0] = 1800000000001.
			}
			return 200
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			c := fixture(t, change)
			from, to := bounds()
			_, err := c.Collect(context.Background(), slo(), from, to)
			if err == nil {
				t.Fatal("accepted ambiguous evidence")
			}
			if strings.Contains(err.Error(), "synthetic-api-key") || strings.Contains(err.Error(), "private query") {
				t.Fatal("leaked provider evidence")
			}
		})
	}
}
func TestDatadogMissingPairsRemainMissing(t *testing.T) {
	c := fixture(t, func(r *http.Request, b map[string]any) int {
		if r.URL.Path == "/api/v1/query" {
			b["series"].([]any)[0].(map[string]any)["pointlist"].([]any)[0].([]any)[1] = nil
		}
		return 200
	})
	from, to := bounds()
	points, err := c.Collect(context.Background(), slo(), from, to)
	if err != nil || len(points) != 1 || !points[0].Time.Equal(to) {
		t.Fatal(points, err)
	}
}
func TestDatadogBoundsAndRouting(t *testing.T) {
	for _, endpoint := range []string{"http://api.datadoghq.com", "https://evil.test", "https://api.datadoghq.com.evil.test", "https://api.datadoghq.com:443", "https://api.datadoghq.com/path", "https://user@api.datadoghq.com", "https://api.datadoghq.com?x=y"} {
		if _, err := New(endpoint, "a", "b"); err == nil {
			t.Fatal(endpoint)
		}
	}
	for _, key := range []string{"", "a\r\nheader:x", " secret"} {
		if _, err := New("https://api.datadoghq.com", key, "app"); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
	c := fixture(t, nil)
	calls := 0
	c.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("should not run") })
	from, to := bounds()
	s := slo()
	s.QueryLanguage = "promql"
	if _, err := c.Collect(context.Background(), s, from, to); err == nil || calls != 0 {
		t.Fatal("invalid query reached provider")
	}
}
func TestDatadogChunkBoundaries(t *testing.T) {
	c := fixture(t, nil)
	from, _ := bounds()
	to := from.Add(501 * time.Minute)
	points, err := c.Collect(context.Background(), slo(), from, to)
	if err != nil || len(points) != 501 || !points[500].Time.Equal(to) {
		t.Fatal(len(points), err)
	}
}

func TestRateMetricsNeedIntervalMetadata(t *testing.T) {
	from, to := bounds()
	for _, interval := range []int64{0, 7, 120} {
		c := fixture(t, func(r *http.Request, b map[string]any) int {
			if r.URL.Path != "/api/v1/query" {
				b["type"] = "rate"
				b["statsd_interval"] = interval
			}
			return 200
		})
		if _, err := c.Collect(context.Background(), slo(), from, to); err == nil {
			t.Fatal("unverified rate normalization", interval)
		}
	}
	c := fixture(t, func(r *http.Request, b map[string]any) int {
		if r.URL.Path != "/api/v1/query" {
			b["type"] = "rate"
			b["statsd_interval"] = 10
		}
		return 200
	})
	if _, err := c.Collect(context.Background(), slo(), from, to); err != nil {
		t.Fatal(err)
	}
}
func TestProbeAndCredentialIdentity(t *testing.T) {
	c := fixture(t, func(r *http.Request, b map[string]any) int {
		if r.URL.Path != "/api/v2/validate_keys" {
			t.Fatal("probe executed a metric query")
		}
		b["status"] = "ok"
		return 200
	})
	if err := c.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	old := c.SourceFingerprint()
	c.apiKey = "rotated-synthetic-key"
	if old == c.SourceFingerprint() || strings.Contains(c.SourceFingerprint(), "rotated") {
		t.Fatal("account credential identity was not pinned")
	}
}
func TestRedirectAndTransportErrorsDoNotLeak(t *testing.T) {
	c := fixture(t, nil)
	calls := 0
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.test/"}}, Body: io.NopCloser(strings.NewReader("private response")), Request: r}, nil
	})
	if err := c.Probe(context.Background()); err == nil || calls != 1 || strings.Contains(err.Error(), "private") {
		t.Fatal("redirect not denied", err, calls)
	}
	c.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("synthetic-api-key raw query") })
	if err := c.Probe(context.Background()); err == nil || strings.Contains(err.Error(), "synthetic") || strings.Contains(err.Error(), "raw query") {
		t.Fatal("transport error leaked", err)
	}
}
func TestResponseSizeBound(t *testing.T) {
	c := fixture(t, nil)
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", (8<<20)+1))), Request: r}, nil
	})
	if err := c.Probe(context.Background()); err == nil {
		t.Fatal("oversized response accepted")
	}
}
