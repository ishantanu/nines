package datadog_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type serviceTransport func(*http.Request) (*http.Response, error)

func (f serviceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDatadogCollectionThroughSLOService(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	from := time.Unix(1800000000, 0).UTC()
	to := from.Add(120 * time.Second)
	http.DefaultTransport = serviceTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"type":"count"}`
		if req.URL.Path == "/api/v1/query" {
			body = `{"status":"ok","res_type":"time_series","series":[{"interval":60,"aggr":"sum","pointlist":[[1800000000000,100],[1800000060000,100]]}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	c, err := datadog.New("https://api.uk1.datadoghq.com", "synthetic-api", "synthetic-app")
	if err != nil {
		t.Fatal(err)
	}
	s := model.SLO{ID: "availability", Name: "API availability", Project: "production", Service: "api", Target: .99, WindowSeconds: 120, StepSeconds: 60, QueryLanguage: "datadog", GoodQuery: "sum:api.good{env:prod}.as_count().rollup(sum,60)", TotalQuery: "sum:api.total{env:prod}.as_count().rollup(sum,60)"}
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "datadog"
	svc.Now = func() time.Time { return to.Add(time.Minute) }
	d := model.DefinitionFromSLO(s)
	definition := model.Definition{SLO: &d}
	plan, err := svc.PlanDefinition(context.Background(), definition)
	if err != nil || plan.Action != "create" {
		t.Fatal(plan, err)
	}
	if _, err = svc.ApplyDefinition(context.Background(), definition, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Collect(context.Background(), s.ID, from, to); err != nil {
		t.Fatal(err)
	}
	record, err := st.Read(context.Background(), s.ID)
	if err != nil || len(record.Points) != 2 || record.SLO.QueryLanguage != "datadog" {
		t.Fatal(record, err)
	}
	status, err := svc.IntegrationStatus(context.Background(), s.ID)
	if err != nil || status.Deployment.State != "unsupported" || status.Evaluation.State != "unsupported" {
		t.Fatal(status, err)
	}
}
