package googlecloud

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

	"github.com/ishantanu/nines/internal/model"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	c, err := New(Endpoint, "nines-test", "checkout", "access-token", "synthetic-token")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "monitoring.googleapis.com" || r.Header.Get("Authorization") != "Bearer synthetic-token" || !strings.HasPrefix(r.URL.Path, "/v3/projects/nines-test/") {
			t.Fatal("routing or authentication escaped trusted configuration")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Result(), nil
	})
	return c
}
func testSLO() model.SLO {
	return model.SLO{ID: "api-availability", Project: "production", Service: "api", Name: "API availability", QueryLanguage: "googlecloud", Target: .999, WindowSeconds: 28 * 86400, StepSeconds: 60, GoodQuery: `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global" AND metric.labels.result = "good"`, TotalQuery: `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global"`, Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}}
}
func countResponse(r *http.Request) map[string]any {
	q := r.URL.Query()
	from, _ := time.Parse(time.RFC3339, q.Get("interval.startTime"))
	to, _ := time.Parse(time.RFC3339, q.Get("interval.endTime"))
	count := "100"
	if strings.Contains(q.Get("filter"), `metric.labels.result = "good"`) {
		count = "90"
	}
	values := []any{}
	for at := from.Add(time.Minute); !at.After(to); at = at.Add(time.Minute) {
		values = append(values, map[string]any{"interval": map[string]string{"startTime": at.Add(-time.Minute).Format(time.RFC3339), "endTime": at.Format(time.RFC3339)}, "value": map[string]string{"int64Value": count}})
	}
	return map[string]any{"timeSeries": []any{map[string]any{"metric": map[string]any{"type": "custom.googleapis.com/nines/requests", "labels": map[string]string{"instance": "one", "result": "good"}}, "resource": map[string]any{"type": "global", "labels": map[string]string{"project_id": "nines-test"}}, "metricKind": "DELTA", "valueType": "INT64", "points": values}}}
}
func TestCountsAreProjectPinnedExactAndSorted(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v3/projects/nines-test/timeSeries" || r.URL.Query().Get("view") != "FULL" || !strings.HasSuffix(r.URL.Query().Get("filter"), `AND project = "nines-test"`) || r.URL.Query().Get("aggregation.perSeriesAligner") != "" {
			t.Fatal("unsafe query shape")
		}
		if err := json.NewEncoder(w).Encode(countResponse(r)); err != nil {
			t.Fatal(err)
		}
	})
	from := time.Unix(1800000000, 0).UTC()
	points, err := c.Collect(context.Background(), testSLO(), from, from.Add(2*time.Minute))
	if err != nil || len(points) != 2 || points[0].Good != 90 || points[0].Total != 100 || !points[0].Time.Before(points[1].Time) {
		t.Fatal(points, err)
	}
}
func TestInvalidOrIncompleteEvidenceFailsClosed(t *testing.T) {
	for _, mode := range []string{"cumulative", "gauge", "distribution", "duplicate", "misaligned", "negative", "oversized-integer", "partial", "missing-good", "pagination-cycle", "denied", "wrong-metric"} {
		t.Run(mode, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "denied" {
					w.WriteHeader(403)
					_, _ = fmt.Fprint(w, "synthetic-token")
					return
				}
				response := countResponse(r)
				s := response["timeSeries"].([]any)[0].(map[string]any)
				points := s["points"].([]any)
				p := points[0].(map[string]any)
				switch mode {
				case "cumulative":
					s["metricKind"] = "CUMULATIVE"
				case "gauge":
					s["metricKind"] = "GAUGE"
				case "distribution":
					s["valueType"] = "DISTRIBUTION"
				case "duplicate":
					s["points"] = append(points, points[0])
				case "misaligned":
					p["interval"].(map[string]string)["startTime"] = "2020-01-01T00:00:00Z"
				case "negative":
					p["value"] = map[string]string{"int64Value": "-1"}
				case "oversized-integer":
					p["value"] = map[string]string{"int64Value": "9007199254740993"}
				case "partial":
					response["executionErrors"] = []any{map[string]any{"code": 13}}
				case "missing-good":
					if strings.Contains(r.URL.Query().Get("filter"), `result = "good"`) {
						response["timeSeries"] = []any{}
					}
				case "pagination-cycle":
					response["nextPageToken"] = "same-page"
				case "wrong-metric":
					s["metric"].(map[string]any)["type"] = "custom.googleapis.com/other"
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Fatal(err)
				}
			})
			from := time.Unix(1800000000, 0).UTC()
			_, err := c.Collect(context.Background(), testSLO(), from, from.Add(time.Minute))
			if err == nil || strings.Contains(err.Error(), "synthetic-token") {
				t.Fatal("invalid evidence accepted or secret exposed", err)
			}
		})
	}
}
func TestNativePublicationUsesImmutableRevisionAndNoPatchOrDelete(t *testing.T) {
	saved := map[string]NativeDocument{}
	writes := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			doc, ok := saved[strings.TrimPrefix(r.URL.Path, "/v3/")]
			if !ok {
				w.WriteHeader(404)
				return
			}
			if err := json.NewEncoder(w).Encode(doc); err != nil {
				t.Fatal(err)
			}
			return
		}
		if r.Method != "POST" {
			t.Fatal("native mutation must be create-only")
		}
		var doc NativeDocument
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(doc.Name, "/"+r.URL.Query().Get("serviceLevelObjectiveId")) {
			t.Fatal("create resource ID mismatch")
		}
		if _, exists := saved[doc.Name]; exists {
			w.WriteHeader(409)
			return
		}
		saved[doc.Name] = doc
		writes++
	})
	ctx := context.Background()
	s := testSLO()
	p, err := c.Prepare(ctx, s, "team", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p.Document.Indicator.RequestBased.Ratio.Total, `project = "nines-test"`) {
		t.Fatal("native filter escaped project")
	}
	if state, err := c.Publish(ctx, p); err != nil || state != "stored" {
		t.Fatal(state, err)
	}
	if _, err = c.Publish(ctx, p); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale create accepted", err)
	}
	p, err = c.Prepare(ctx, s, "team", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	if state, err := c.Publish(ctx, p); err != nil || state != "unchanged" || writes != 1 {
		t.Fatal(state, writes, err)
	}
	s.Revision = 2
	s.Target = .995
	next, err := c.Prepare(ctx, s, "team", "primary", "nines-prod")
	if err != nil || next.Origin == p.Origin {
		t.Fatal("new Nines revision reused upstream resource", next, err)
	}
	if _, err = c.Publish(ctx, next); err != nil || len(saved) != 2 {
		t.Fatal("old revision was not retained", err)
	}
	doc := saved[next.Origin]
	doc.Goal = .9
	saved[next.Origin] = doc
	if _, err = c.Prepare(ctx, s, "team", "primary", "nines-prod"); err == nil {
		t.Fatal("immutable resource drift overwritten")
	}
}
func TestAuthAndPopulationBoundaries(t *testing.T) {
	for _, endpoint := range []string{"http://monitoring.googleapis.com", Endpoint + "/v3", Endpoint + "?project=other", "https://other.example"} {
		if _, err := New(endpoint, "nines-test", "checkout", "access-token", "key"); err == nil {
			t.Fatal(endpoint)
		}
	}
	if _, err := New(Endpoint, "nines-test", "..", "access-token", "key"); err == nil {
		t.Fatal("path traversal service ID")
	}
	if _, err := New(Endpoint, "nines-test", "checkout", "", "key"); err == nil {
		t.Fatal("implicit auth")
	}
	if _, err := New(Endpoint, "nines-test", "checkout", "adc", "key"); err == nil {
		t.Fatal("mixed ADC/token")
	}
	c := testClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected network request") })
	s := testSLO()
	s.TotalQuery += ` AND metric.labels.result = "bad"`
	if err := c.ValidateSLO(s); err == nil {
		t.Fatal("unrelated population")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Probe(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
	other, err := New(Endpoint, "nines-test", "checkout", "access-token", "rotated-token")
	if err != nil || c.SourceFingerprint() == other.SourceFingerprint() {
		t.Fatal("credential rotation did not change source identity", err)
	}
}

func TestNativeHealthNeedsFreshProviderEvidence(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	var stored NativeDocument
	stale := false
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/timeSeries") {
			if r.URL.Query().Get("filter") != "select_slo_health("+strconv.Quote(stored.Name)+")" {
				t.Fatal("native signal route not pinned")
			}
			at := now.Add(-time.Minute)
			if stale {
				at = now.Add(-8 * time.Minute)
			}
			response := map[string]any{"timeSeries": []any{map[string]any{"metricKind": "GAUGE", "valueType": "DOUBLE", "points": []any{map[string]any{"interval": map[string]string{"endTime": at.Format(time.RFC3339)}, "value": map[string]float64{"doubleValue": .995}}}}}}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Fatal(err)
			}
			return
		}
		if stored.Name == "" {
			w.WriteHeader(404)
			return
		}
		if err := json.NewEncoder(w).Encode(stored); err != nil {
			t.Fatal(err)
		}
	})
	p, err := c.Prepare(context.Background(), testSLO(), "team", "primary", "nines-prod")
	if err != nil {
		t.Fatal(err)
	}
	stored = p.Document
	value, at, err := c.NativeHealth(context.Background(), stored.Name, now)
	if err != nil || value != .995 || !at.Equal(now.Add(-time.Minute)) {
		t.Fatal(value, at, err)
	}
	stale = true
	if _, _, err = c.NativeHealth(context.Background(), stored.Name, now); err == nil {
		t.Fatal("stale native signal accepted")
	}
}

func TestCleanupDetectsOlderRevisionWithChangedLabels(t *testing.T) {
	owner := strings.Repeat("a", 40)
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v3/projects/nines-test/services/checkout/serviceLevelObjectives" {
			t.Fatal("unexpected cleanup operation")
		}
		calls++
		response := map[string]any{}
		if r.URL.Query().Get("pageToken") == "" {
			response["serviceLevelObjectives"] = []NativeDocument{{Name: "projects/nines-test/services/checkout/serviceLevelObjectives/nines-" + owner[:16] + "-old", UserLabels: map[string]string{"nines_owner": "changed"}}}
			response["nextPageToken"] = "next"
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	})
	resources, err := c.OwnedRevisions(context.Background(), owner)
	if err != nil || calls != 2 || len(resources) != 1 {
		t.Fatal("retained revision was hidden by label changes or pagination", resources, calls, err)
	}
}
func TestZeroTrafficAndAbsentTelemetryRemainDistinct(t *testing.T) {
	for _, missing := range []bool{false, true} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			response := countResponse(r)
			if missing {
				response["timeSeries"] = []any{}
			} else {
				s := response["timeSeries"].([]any)[0].(map[string]any)
				for _, p := range s["points"].([]any) {
					p.(map[string]any)["value"] = map[string]string{"int64Value": "0"}
				}
			}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Fatal(err)
			}
		})
		from := time.Unix(1800000000, 0).UTC()
		points, err := c.Collect(context.Background(), testSLO(), from, from.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if missing {
			if len(points) != 0 {
				t.Fatal("absent telemetry filled", points)
			}
		} else if len(points) != 1 || points[0].Good != 0 || points[0].Total != 0 {
			t.Fatal("zero counts changed", points)
		}
	}
}

func TestADCQuotaProjectIsBoundedAndDoesNotSelectDataProject(t *testing.T) {
	project, err := adcQuotaProject([]byte(`{"quota_project_id":"billing-project"}`))
	if err != nil || project != "billing-project" {
		t.Fatal(project, err)
	}
	if _, err = adcQuotaProject([]byte(`{"quota_project_id":"../../other"}`)); err == nil {
		t.Fatal("invalid quota project accepted")
	}
	if project, err = adcQuotaProject(nil); err != nil || project != "" {
		t.Fatal(project, err)
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-User-Project") != "billing-project" || !strings.HasPrefix(r.URL.Path, "/v3/projects/nines-test/") {
			t.Fatal("billing project changed data route")
		}
		_, _ = fmt.Fprint(w, `{}`)
	})
	c.quotaProject = "billing-project"
	if err = c.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeCanonicalProjectNumbersPreserveConfiguredScope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v3/")
		name = strings.Replace(name, "projects/nines-test/", "projects/123456789012/", 1)
		if err := json.NewEncoder(w).Encode(NativeDocument{Name: name}); err != nil {
			t.Fatal(err)
		}
	})
	name := "projects/nines-test/services/checkout/serviceLevelObjectives/nines-owned"
	doc, exists, err := c.ReadNative(context.Background(), name)
	if err != nil || !exists || doc.Name != name {
		t.Fatal("canonical name not recognized", doc, exists, err)
	}
	for _, other := range []string{"projects/unrelated/services/checkout/serviceLevelObjectives/nines-owned", "projects/999999999999/services/checkout/serviceLevelObjectives/nines-owned", "projects/123456789012/services/other/serviceLevelObjectives/nines-owned"} {
		if _, err = c.normalizeNativeName(other); err == nil {
			t.Fatal("canonicalization escaped configured scope", other)
		}
	}
}
