package dash0

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/openslo"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	c, err := New("https://api.eu-west-1.aws.dash0.com", "production", "synthetic-dash0-secret")
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.eu-west-1.aws.dash0.com" || r.URL.Query().Get("dataset") != "production" || r.Header.Get("Authorization") != "Bearer synthetic-dash0-secret" || r.Header.Get("X-Scope-OrgID") != "" {
			t.Fatal("route, dataset or authorization changed")
		}
		w := httptest.NewRecorder()
		handler(w, r)
		return w.Result(), nil
	})
	c.query.HTTP.Transport = datasetTransport{next: transport, dataset: c.dataset, origin: c.endpoint}
	c.http.Transport = transport
	return c
}
func nativeSLO() model.SLO {
	return model.SLO{ID: "api-availability", Project: "production", Service: "api", Name: "API availability", Target: .999, WindowSeconds: 28 * 86400, StepSeconds: 60, ProviderWindow: &v1.ProviderWindow{GoodSelector: `requests_total{code="200"}`, TotalSelector: `requests_total`, ScrapeIntervalSeconds: 15}, Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}}
}
func TestCollectionPinsGETDatasetAndPreservesCounts(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/prometheus/api/v1/query_range" || r.Body != nil {
			t.Fatal("wrong query method or route")
		}
		at, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		value := "100"
		if strings.Contains(r.URL.Query().Get("query"), `code="200"`) {
			value = "90"
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": map[string]string{}, "values": []any{[]any{at, value}}}}}}); err != nil {
			t.Fatal(err)
		}
	})
	s := nativeSLO()
	s.ProviderWindow = nil
	s.GoodQuery = `sum(increase(requests_total{code="200"}[1m]))`
	s.TotalQuery = `sum(increase(requests_total[1m]))`
	from := time.Unix(1800000000, 0)
	points, err := c.Collect(context.Background(), s, from, from.Add(time.Minute))
	if err != nil || len(points) != 1 || points[0].Good != 90 || points[0].Total != 100 {
		t.Fatal(points, err)
	}
	s.GoodQuery = "sum("
	if _, err = c.Collect(context.Background(), s, from, from.Add(time.Minute)); err == nil {
		t.Fatal("malformed query accepted")
	}
}
func TestPublisherCreationVersionedUpdateAndDisable(t *testing.T) {
	var saved *openslo.Document
	writes := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if saved == nil {
				w.WriteHeader(404)
				return
			}
			if err := json.NewEncoder(w).Encode(saved); err != nil {
				t.Fatal(err)
			}
			return
		}
		var doc openslo.Document
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		if saved == nil {
			if r.Method != http.MethodPost || r.URL.Path != "/api/slos" || doc.Metadata.Labels["dash0.com/version"] != "" {
				t.Fatal("creation must use origin-unique POST")
			}
		} else {
			if r.Method != http.MethodPut || doc.Metadata.Labels["dash0.com/version"] != saved.Metadata.Labels["dash0.com/version"] {
				w.WriteHeader(400)
				return
			}
		}
		writes++
		doc.Metadata.Labels["dash0.com/version"] = strconv.Itoa(writes)
		doc.APIVersion = "openslo.com/v1"
		saved = &doc
		w.WriteHeader(200)
	})
	ctx := context.Background()
	s := nativeSLO()
	plan, err := c.Prepare(ctx, s, "team-a", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	if plan.PriorVersion != "" || len(plan.Warnings) < 3 {
		t.Fatal(plan)
	}
	if state, err := c.Publish(ctx, plan); err != nil || state != "stored" {
		t.Fatal(state, err)
	}
	if _, err = c.Publish(ctx, plan); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale create accepted", err)
	}
	plan, err = c.Prepare(ctx, s, "team-a", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	if state, err := c.Publish(ctx, plan); err != nil || state != "unchanged" || writes != 1 {
		t.Fatal(state, writes, err)
	}
	s.Target = .995
	s.Revision = 2
	plan, err = c.Prepare(ctx, s, "team-a", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	if state, err := c.Publish(ctx, plan); err != nil || state != "stored" {
		t.Fatal(state, err)
	}
	plan, err = c.Prepare(ctx, s, "team-a", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Disable(ctx, plan); err != nil || saved.Metadata.Annotations["dash0.com/enabled"] != "false" {
		t.Fatal(err)
	}
	if writes != 3 {
		t.Fatal("unexpected mutations", writes)
	}
}
func TestPublisherRejectsForeignOwnershipAndUnsupportedDefinitions(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatal("unexpected write")
		}
		if err := json.NewEncoder(w).Encode(openslo.Document{APIVersion: "openslo.com/v1", Kind: "SLO", Metadata: openslo.Metadata{Name: "api-availability", Labels: map[string]string{"dash0.com/version": "9", "nines.dev/tenant": "other"}}}); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := c.Prepare(context.Background(), nativeSLO(), "team-a", "primary", "prod"); err == nil {
		t.Fatal("foreign resource accepted")
	}
	s := nativeSLO()
	s.WindowSeconds = 86400
	if _, err := c.Prepare(context.Background(), s, "team-a", "primary", "prod"); err == nil {
		t.Fatal("unsupported window accepted")
	}
	for _, endpoint := range []string{"http://api.eu-west-1.aws.dash0.com", "https://attacker.example", "https://api.eu-west-1.aws.dash0.com/api", "https://api.eu-west-1.aws.dash0.com?dataset=other"} {
		if _, err := New(endpoint, "production", "key"); err == nil {
			t.Fatal("unsafe endpoint", endpoint)
		}
	}
	if _, err := New("https://api.eu-west-1.aws.dash0.com", "", "key"); err == nil {
		t.Fatal("implicit dataset")
	}
}
func TestPublisherRedactsFailuresAndRejectsFailedReadback(t *testing.T) {
	for _, mode := range []string{"denied", "bad-readback", "oversized", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			reads := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					w.Header().Set("Location", "https://elsewhere.example")
					w.WriteHeader(302)
					return
				}
				if mode == "denied" {
					w.WriteHeader(403)
					_, _ = fmt.Fprint(w, "synthetic-dash0-secret")
					return
				}
				if mode == "oversized" {
					_, _ = fmt.Fprint(w, strings.Repeat("x", (1<<20)+1))
					return
				}
				if r.Method == http.MethodGet {
					reads++
					if reads <= 2 {
						w.WriteHeader(404)
						return
					}
					_, _ = fmt.Fprint(w, `{"apiVersion":"openslo.com/v1","kind":"SLO","metadata":{"name":"wrong"}}`)
					return
				}
				w.WriteHeader(200)
			})
			p, err := c.Prepare(context.Background(), nativeSLO(), "tenant", "primary", "prod")
			if err == nil {
				_, err = c.Publish(context.Background(), p)
			}
			if err == nil || strings.Contains(err.Error(), "synthetic-dash0-secret") {
				t.Fatal("failure not safe", err)
			}
		})
	}
}
