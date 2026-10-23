package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestHistoryAndRuleDeploymentRoleAndRevisionBoundaries(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Minute)
	svc := service.New(store.NewMemory(), nil)
	svc.Now = func() time.Time { return at }
	slo := model.SLO{ID: "availability", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)", Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 600, BurnRate: 2, MinCoverage: 1}}
	if err := svc.Create(context.Background(), slo); err != nil {
		t.Fatal(err)
	}
	points := []model.Point{}
	for i := 0; i < 120; i++ {
		ts := at.Add(-time.Duration(i) * time.Minute)
		points = append(points, model.Point{Time: ts, Good: 95, Total: 100})
	}
	if err := svc.Ingest(context.Background(), slo.ID, points); err != nil {
		t.Fatal(err)
	}
	manager := &rules.Manager{Tenant: "alpha", Targets: map[string]rules.Target{"primary": {Connection: "primary", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir()}}}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Rules: manager, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		return w
	}
	var history engine.History
	if err := json.Unmarshal(request("viewer", "GET", "/api/slos/availability/history?rangeSeconds=1800&resolutionSeconds=60", "", 200).Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Points) != 31 || !history.Points[30].TargetViolated || len(history.Violations) == 0 {
		t.Fatal(history)
	}
	request("viewer", "GET", "/api/slos/availability/history?rangeSeconds=99999999", "", 400)
	request("viewer", "GET", "/api/slos/availability/history?resolutionSeconds=9223372036854775800", "", 400)
	var plan rules.Plan
	if err := json.Unmarshal(request("agent", "POST", "/api/slos/availability/rules/plan", "", 200).Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"expectedDigest": plan.Digest})
	request("agent", "POST", "/api/slos/availability/rules/apply", string(body), 403)
	request("viewer", "POST", "/api/slos/availability/rules/apply", string(body), 403)
	request("owner", "POST", "/api/slos/availability/rules/apply", `{"expectedDigest":"wrong"}`, 409)
	var applied rules.Deployment
	if err := json.Unmarshal(request("owner", "POST", "/api/slos/availability/rules/apply", string(body), 200).Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.State != "written" || applied.Activation == nil || applied.Activation.State != "manual" || applied.Evaluation == nil || applied.Evaluation.State != "unknown" {
		t.Fatal("API implied file persistence proves activation", applied)
	}
	definition := model.DefinitionFromSLO(slo)
	definition.Spec.WindowSeconds = 7200
	if _, err := svc.ApplyDefinition(context.Background(), model.Definition{SLO: &definition}, 1); err != nil {
		t.Fatal(err)
	}
	request("owner", "POST", "/api/slos/availability/rules/apply", string(body), 409)
	request("viewer", "GET", "/api/slos/availability/manifest", "", 200)
}

// Metadata reads must not query a source, even when no primary is configured.
type metadataOnlyCollector struct{}

func (metadataOnlyCollector) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	panic("workspace metadata attempted collection")
}
func TestWorkspaceReportsNamedConnectionsWithoutPrimary(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	svc.Sources = map[string]service.MetricSource{
		"production": {Kind: "mimir", Collector: metadataOnlyCollector{}},
		"staging":    {Kind: "prometheus", Collector: metadataOnlyCollector{}},
	}
	h := New(svc, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/workspace", nil))
	var got struct {
		PrometheusConfigured bool `json:"prometheusConfigured"`
		Source               struct {
			Kind            string
			Configured      bool
			ConnectionCount int
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !got.PrometheusConfigured || !got.Source.Configured || got.Source.Kind != "multiple" || got.Source.ConnectionCount != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
}
