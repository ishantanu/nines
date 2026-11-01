package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/googlecloud"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type googleCountTransport func(*http.Request) (*http.Response, error)

func (f googleCountTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGoogleCloudCountsFeedSLIAndBurnLifecycle(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	healthy := false
	http.DefaultTransport = googleCountTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "monitoring.googleapis.com" || r.URL.Path != "/v3/projects/nines-test/timeSeries" || !strings.HasSuffix(r.URL.Query().Get("filter"), `AND project = "nines-test"`) {
			t.Fatal("query escaped project")
		}
		goodFilter := strings.Contains(r.URL.Query().Get("filter"), `result = "good"`)
		from, err := time.Parse(time.RFC3339, r.URL.Query().Get("interval.startTime"))
		if err != nil {
			t.Fatal(err)
		}
		to, err := time.Parse(time.RFC3339, r.URL.Query().Get("interval.endTime"))
		if err != nil {
			t.Fatal(err)
		}
		good, bad := 90, 10
		if healthy {
			good, bad = 100, 0
		}
		rows := []any{}
		for _, kind := range []string{"good", "bad"} {
			if goodFilter && kind == "bad" {
				continue
			}
			value := good
			if kind == "bad" {
				value = bad
			}
			points := []any{}
			for at := from.Add(time.Minute); !at.After(to); at = at.Add(time.Minute) {
				points = append(points, map[string]any{"interval": map[string]string{"startTime": at.Add(-time.Minute).Format(time.RFC3339), "endTime": at.Format(time.RFC3339)}, "value": map[string]string{"int64Value": strconv.Itoa(value)}})
			}
			rows = append(rows, map[string]any{"metricKind": "DELTA", "valueType": "INT64", "metric": map[string]any{"type": "custom.googleapis.com/nines/requests", "labels": map[string]string{"result": kind}}, "resource": map[string]any{"type": "global", "labels": map[string]string{"project_id": "nines-test"}}, "points": points})
		}
		w := httptest.NewRecorder()
		if err = json.NewEncoder(w).Encode(map[string]any{"timeSeries": rows}); err != nil {
			t.Fatal(err)
		}
		return w.Result(), nil
	})
	c, err := googlecloud.New(googlecloud.Endpoint, "nines-test", "checkout", "access-token", "synthetic-key")
	if err != nil {
		t.Fatal(err)
	}
	s := model.SLO{ID: "checkout", Name: "Checkout availability", Project: "production", Service: "checkout", QueryLanguage: "googlecloud", Target: .99, WindowSeconds: 120, StepSeconds: 60, GoodQuery: `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global" AND metric.labels.result = "good"`, TotalQuery: `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global"`, Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, MinCoverage: 1}}
	from := time.Unix(1800000000, 0).UTC()
	to := from.Add(2 * time.Minute)
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "googlecloud"
	svc.Now = func() time.Time { return to.Add(time.Minute) }
	d := model.DefinitionFromSLO(s)
	if _, err = svc.ApplyDefinition(context.Background(), model.Definition{SLO: &d}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Collect(context.Background(), s.ID, from, to); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.Status(context.Background(), s.ID, to)
	if err != nil || snapshot.Reliability == nil || *snapshot.Reliability != .9 || snapshot.Coverage != 1 || snapshot.Alert.Phase != "firing" {
		t.Fatal(snapshot, err)
	}
	healthy = true
	svc.Now = func() time.Time { return to.Add(3 * time.Minute) }
	if _, err = svc.Collect(context.Background(), s.ID, to, to.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	record, err := st.Read(context.Background(), s.ID)
	if err != nil || record.Alert.Phase != "inactive" {
		t.Fatal(record, err)
	}
}
