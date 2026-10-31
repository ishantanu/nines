package newrelic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func slo() model.SLO {
	return model.SLO{ID: "nr-availability", Name: "API availability", Service: "api", Project: "production", Target: .99, WindowSeconds: 120, StepSeconds: 60, QueryLanguage: "nrql", GoodQuery: "SELECT count(*) FROM NinesRequest WHERE service = 'api' AND success = true", TotalQuery: "SELECT count(*) FROM NinesRequest WHERE service = 'api'", Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, MinCoverage: 1}}
}
func bounds() (time.Time, time.Time) {
	from := time.Unix(1800000000, 0).UTC()
	return from, from.Add(120 * time.Second)
}

var window = regexp.MustCompile(` SINCE ([0-9]+) UNTIL ([0-9]+) TIMESERIES ([0-9]+) SECONDS$`)

func fixture(t *testing.T, change func(map[string]any, map[string]any) int) *Client {
	t.Helper()
	c, err := New("https://api.eu.newrelic.com/graphql", "123456", "synthetic-user-key")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != c.endpoint || r.Header.Get("API-Key") != "synthetic-user-key" {
			t.Fatal("wrong account route or credential")
		}
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("invalid request JSON")
		}
		if request.Variables["account"] != float64(123456) {
			t.Fatal("request did not pin account")
		}
		if strings.Contains(strings.ToLower(request.Query), "mutation") {
			t.Fatal("mutation attempted")
		}
		account := map[string]any{"id": 123456}
		if request.Query == countDocument {
			for _, alias := range []string{"good", "total"} {
				q, ok := request.Variables[alias].(string)
				if !ok {
					t.Fatal("query missing")
				}
				m := window.FindStringSubmatch(q)
				if m == nil || !strings.Contains(q, " AS nines_count FROM ") {
					t.Fatal(q)
				}
				from, _ := strconv.ParseInt(m[1], 10, 64)
				to, _ := strconv.ParseInt(m[2], 10, 64)
				step, _ := strconv.ParseInt(m[3], 10, 64)
				if (to-from)/1000/step > maxBuckets {
					t.Fatal("bucket bound exceeded")
				}
				rows := []any{}
				count := 100.
				if alias == "good" {
					count = 90.
				}
				for at := from / 1000; at < to/1000; at += step {
					rows = append(rows, map[string]any{"beginTimeSeconds": at, "endTimeSeconds": at + step, "nines_count": count})
				}
				account[alias] = map[string]any{"results": rows, "metadata": map[string]any{"messages": []any{}, "timeWindow": map[string]any{"begin": from, "end": to}}}
			}
		} else if request.Query != probeDocument {
			t.Fatal("unexpected GraphQL document")
		}
		response := map[string]any{"data": map[string]any{"actor": map[string]any{"account": account}}}
		code := 200
		if change != nil {
			code = change(account, response)
		}
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
	})
	return c
}
func TestCollectNRQLCountsAndChunking(t *testing.T) {
	c := fixture(t, nil)
	from, to := bounds()
	points, err := c.Collect(context.Background(), slo(), from, to)
	if err != nil || len(points) != 2 || !points[0].Time.Equal(from.Add(time.Minute)) || points[0].Good != 90 || points[0].Total != 100 {
		t.Fatal(points, err)
	}
	to = from.Add(301 * time.Minute)
	points, err = c.Collect(context.Background(), slo(), from, to)
	if err != nil || len(points) != 301 || !points[300].Time.Equal(to) {
		t.Fatal(len(points), err)
	}
}

func goodResult(a map[string]any) map[string]any { return a["good"].(map[string]any) }
func goodRows(a map[string]any) []any            { return goodResult(a)["results"].([]any) }
func TestNRQLRefusesIncompleteOrUnsafeEvidence(t *testing.T) {
	tests := map[string]func(map[string]any, map[string]any) int{
		"partial GraphQL": func(_ map[string]any, r map[string]any) int {
			r["errors"] = []any{map[string]any{"message": "synthetic-user-key private query"}}
			return 200
		},
		"wrong account":  func(a, r map[string]any) int { a["id"] = 999; return 200 },
		"missing alias":  func(a, r map[string]any) int { a["good"] = nil; return 200 },
		"missing bucket": func(a, r map[string]any) int { goodResult(a)["results"] = goodRows(a)[1:]; return 200 },
		"null numerator": func(a, r map[string]any) int { goodRows(a)[0].(map[string]any)["nines_count"] = nil; return 200 },
		"duplicate": func(a, r map[string]any) int {
			goodResult(a)["results"] = append(goodRows(a), goodRows(a)[0])
			return 200
		},
		"coarser": func(a, r map[string]any) int {
			row := goodRows(a)[0].(map[string]any)
			row["endTimeSeconds"] = row["beginTimeSeconds"].(int64) + 120
			return 200
		},
		"misalignment": func(a, r map[string]any) int {
			row := goodRows(a)[0].(map[string]any)
			row["beginTimeSeconds"] = row["beginTimeSeconds"].(int64) + 1
			return 200
		},
		"warned": func(a, r map[string]any) int {
			goodResult(a)["metadata"].(map[string]any)["messages"] = []any{"sampling private info"}
			return 200
		},
		"shifted window": func(a, r map[string]any) int {
			goodResult(a)["metadata"].(map[string]any)["timeWindow"].(map[string]any)["begin"] = 1
			return 200
		},
		"too many good": func(a, r map[string]any) int { goodRows(a)[0].(map[string]any)["nines_count"] = 101; return 200 },
		"rate limit":    func(a, r map[string]any) int { r["errors"] = []any{"private query synthetic-user-key"}; return 429 },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			c := fixture(t, change)
			from, to := bounds()
			_, err := c.Collect(context.Background(), slo(), from, to)
			if err == nil {
				t.Fatal("accepted incomplete evidence")
			}
			if strings.Contains(err.Error(), "synthetic-user-key") || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "sampling private") {
				t.Fatal("leaked provider evidence", err)
			}
		})
	}
}
func TestNRQLGapsAndZeroTraffic(t *testing.T) {
	for _, null := range []bool{true, false} {
		c := fixture(t, func(a, r map[string]any) int {
			for _, name := range []string{"good", "total"} {
				rows := a[name].(map[string]any)["results"].([]any)
				row := rows[0].(map[string]any)
				if null {
					row["nines_count"] = nil
				} else {
					row["nines_count"] = 0
				}
			}
			return 200
		})
		from, to := bounds()
		points, err := c.Collect(context.Background(), slo(), from, to)
		want := 2
		if null {
			want = 1
		}
		if err != nil || len(points) != want {
			t.Fatal(points, err)
		}
		if !null && (points[0].Good != 0 || points[0].Total != 0) {
			t.Fatal("zero traffic changed")
		}
	}
}
func TestNRQLProbeAndRouteValidation(t *testing.T) {
	c := fixture(t, nil)
	if err := c.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://api.newrelic.com/graphql", "https://evil.test/graphql", "https://api.newrelic.com/graphql?account=3", "https://api.newrelic.com:443/graphql", "https://user@api.newrelic.com/graphql", "https://api.newrelic.com/other"} {
		if _, err := New(endpoint, "123", "key"); err == nil {
			t.Fatal(endpoint)
		}
	}
	for _, account := range []string{"0", "-1", "1,2", "001", "2147483648", " 123"} {
		if _, err := New(c.endpoint, account, "key"); err == nil {
			t.Fatal(account)
		}
	}
	if _, err := New(c.endpoint, "123", "key\r\nHeader:x"); err == nil {
		t.Fatal("invalid key accepted")
	}
	old := c.SourceFingerprint()
	c.account++
	if old == c.SourceFingerprint() {
		t.Fatal("account identity lost")
	}
	c.account--
	c.userKey = "rotated"
	if old == c.SourceFingerprint() {
		t.Fatal("credential identity lost")
	}
}
func TestNRQLInvalidQueriesDoNotCallProvider(t *testing.T) {
	c := fixture(t, nil)
	calls := 0
	c.http.Transport = transport(func(*http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("must not run") })
	from, to := bounds()
	for _, change := range []func(*model.SLO){func(s *model.SLO) { s.QueryLanguage = "promql" }, func(s *model.SLO) { s.GoodQuery = "SELECT average(duration) FROM NinesRequest" }, func(s *model.SLO) { s.TotalQuery = "SELECT count(*) FROM Other" }} {
		s := slo()
		change(&s)
		if _, err := c.Collect(context.Background(), s, from, to); err == nil || calls != 0 {
			t.Fatal("invalid query called provider")
		}
	}
}

func TestNRQLTransportBoundsCancellationAndRedaction(t *testing.T) {
	c := fixture(t, nil)
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", (8<<20)+1))), Header: http.Header{}, Request: r}, nil
	})
	if err := c.Probe(context.Background()); err == nil {
		t.Fatal("oversized response accepted")
	}
	calls := 0
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusFound, Body: io.NopCloser(strings.NewReader("private")), Header: http.Header{"Location": []string{"https://evil.test/"}}, Request: r}, nil
	})
	if err := c.Probe(context.Background()); err == nil || calls != 1 {
		t.Fatal("redirect followed", calls, err)
	}
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Context().Err() == nil {
			t.Fatal("cancellation context lost")
		}
		return nil, fmt.Errorf("synthetic-user-key private query: %w", r.Context().Err())
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Probe(ctx); err == nil || strings.Contains(err.Error(), "synthetic") || strings.Contains(err.Error(), "private") {
		t.Fatal("transport error leaked", err)
	}
}

func TestNRQLNoEventsNoticeDoesNotCreateCoverage(t *testing.T) {
	for _, mode := range []string{"empty", "contradictory", "additional warning", "shifted", "one alias"} {
		t.Run(mode, func(t *testing.T) {
			c := fixture(t, func(a, _ map[string]any) int {
				for _, alias := range []string{"good", "total"} {
					if mode == "one alias" && alias == "total" {
						continue
					}
					result := a[alias].(map[string]any)
					metadata := result["metadata"].(map[string]any)
					metadata["messages"] = []any{"No events found -- do you have the correct event type and time range?"}
					if mode == "additional warning" {
						metadata["messages"] = append(metadata["messages"].([]any), "private sampling warning")
					}
					if mode == "shifted" {
						metadata["timeWindow"].(map[string]any)["begin"] = 1
					}
					for _, row := range result["results"].([]any) {
						if mode != "contradictory" {
							row.(map[string]any)["nines_count"] = 0
						}
					}
				}
				return 200
			})
			from, to := bounds()
			points, err := c.Collect(context.Background(), slo(), from, to)
			if mode == "empty" {
				if err != nil || len(points) != 0 {
					t.Fatalf("empty events fabricated coverage: %v %v", points, err)
				}
			} else if err == nil || len(points) != 0 {
				t.Fatalf("accepted inconsistent or warned response: %v %v", points, err)
			}
		})
	}
}
