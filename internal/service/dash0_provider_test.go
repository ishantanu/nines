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

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/dash0"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type dash0WindowTransport func(*http.Request) (*http.Response, error)

func (f dash0WindowTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWholeWindowCollectionFeedsNinesSLIAndBurnPolicy(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataset") != "production" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatal("dataset or credential changed")
		}
		if r.Method != "GET" || r.URL.Path != "/api/prometheus/api/v1/query_range" {
			t.Fatal("wrong query route")
		}
		q := r.URL.Query()
		from, err := strconv.ParseInt(q.Get("start"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		to, err := strconv.ParseInt(q.Get("end"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		value := "100"
		if strings.Contains(q.Get("query"), `code="200"`) {
			value = "90"
		}
		if strings.Contains(q.Get("query"), "count_over_time") {
			value = "1"
		}
		values := []any{}
		for at := from; at <= to; at += 60 {
			values = append(values, []any{at, value})
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": map[string]string{}, "values": values}}}}); err != nil {
			t.Fatal(err)
		}
	})
	http.DefaultTransport = dash0WindowTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Result(), nil
	})
	c, err := dash0.New("https://api.eu-west-1.aws.dash0.com", "production", "synthetic-key")
	if err != nil {
		t.Fatal(err)
	}
	s := model.SLO{ID: "api-availability", Project: "production", Service: "api", Name: "API availability", Target: .999, WindowSeconds: 120, StepSeconds: 60, ProviderWindow: &v1.ProviderWindow{GoodSelector: `requests_total{code="200"}`, TotalSelector: `requests_total`, ScrapeIntervalSeconds: 15}, Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, MinCoverage: 1}}
	s.WindowSeconds = 120
	s.Policy.ShortWindowSeconds = 60
	s.Policy.LongWindowSeconds = 120
	s.Policy.BurnRate = 2
	from := time.Unix(1800000000, 0).UTC()
	to := from.Add(2 * time.Minute)
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "dash0"
	svc.Now = func() time.Time { return to.Add(time.Minute) }
	d := model.DefinitionFromSLO(s)
	if _, err := svc.ApplyDefinition(context.Background(), model.Definition{SLO: &d}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Collect(context.Background(), s.ID, from, to); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.Status(context.Background(), s.ID, to)
	if err != nil || snapshot.Reliability == nil || *snapshot.Reliability != .9 || snapshot.Coverage != 1 || snapshot.Alert.Phase != "firing" {
		t.Fatal(snapshot, err)
	}
}
