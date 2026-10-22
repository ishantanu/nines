package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestJourneyAPILinksHealthRevisionsAndEvidence(t *testing.T) {
	ctx := context.Background()
	svc := service.New(store.NewMemory(), nil)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	h := New(svc, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := func(method, path string, body any, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	payload := map[string]any{"manifest": examples.AgentResolution, "expectedRevision": 0}
	request("POST", "/api/journeys/plan", payload, 400) // No dangling references.
	for _, id := range []string{"api-availability", "background-jobs"} {
		if err := svc.Create(ctx, model.SLO{ID: id, Name: id, Project: "demo", Service: "s", Target: .99, WindowSeconds: 60, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
	}
	request("POST", "/api/journeys/plan", payload, 200)
	request("POST", "/api/journeys/apply", map[string]any{"manifest": examples.AgentResolution}, 400)
	request("POST", "/api/journeys/apply", payload, 200)
	request("POST", "/api/journeys/apply", payload, 409)
	var view service.JourneyView
	_ = json.Unmarshal(request("GET", "/api/journeys/assisted-resolution", nil, 200), &view)
	if view.Health != "unknown" {
		t.Fatal("missing data became healthy", view)
	}
	request("GET", "/api/slos/api-availability/journeys", nil, 200)
	request("GET", "/api/slos/missing/journeys", nil, 404)
	for _, id := range []string{"api-availability", "background-jobs"} {
		if err := svc.Ingest(ctx, id, []model.Point{{Time: now, Good: 100, Total: 100}}); err != nil {
			t.Fatal(err)
		}
	}
	_ = json.Unmarshal(request("GET", "/api/journeys/assisted-resolution", nil, 200), &view)
	if view.Health != "healthy" {
		t.Fatal(view)
	}
	if err := svc.Ingest(ctx, "background-jobs", []model.Point{{Time: now, Good: 90, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(request("GET", "/api/journeys/assisted-resolution", nil, 200), &view)
	if view.Health != "degraded" {
		t.Fatal(view)
	}
	changed := strings.Replace(examples.AgentResolution, "maxCostUSD: 0.25", "maxCostUSD: 0.05", 1)
	request("POST", "/api/journeys/apply", map[string]any{"manifest": changed, "expectedRevision": 1}, 200)
	cost := .1
	steps := 5
	approved := true
	e := model.RunEvidence{ID: "test-run", JourneyRevision: 1, StartedAt: now.Add(-time.Minute), FinishedAt: now, Outcome: "success", CostUSD: &cost, Steps: &steps, ApprovalObserved: &approved}
	var run model.JourneyRun
	_ = json.Unmarshal(request("POST", "/api/journeys/assisted-resolution/runs", e, 200), &run)
	if run.Verdict != "passed" {
		t.Fatal("used latest rather than historical contract", run)
	}
	request("POST", "/api/journeys/assisted-resolution/runs", e, 200)
	e.Outcome = "failure"
	request("POST", "/api/journeys/assisted-resolution/runs", e, 409)
	e.ID = "new-contract"
	e.Outcome = "success"
	e.JourneyRevision = 2
	_ = json.Unmarshal(request("POST", "/api/journeys/assisted-resolution/runs", e, 200), &run)
	if run.Verdict != "violated" {
		t.Fatal(run)
	}
	request("GET", "/api/journeys/assisted-resolution/runs", nil, 200)
	request("GET", "/api/journeys/assisted-resolution/history", nil, 200)
	yaml := request("GET", "/api/journeys/assisted-resolution/manifest?revision=1", nil, 200)
	if _, err := model.ParseJourney(yaml); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/journeys/assisted-resolution/manifest?revision=-1", nil, 400)
	// Serving the actual bundled React build is part of the server contract.
	index := request("GET", "/", nil, 200)
	if !bytes.Contains(index, []byte("/assets/")) {
		t.Fatal("missing bundled UI")
	}
	request("GET", "/api/not-a-route", nil, 404)
}
func TestRequiredAndAdvisoryHealth(t *testing.T) {
	ctx := context.Background()
	svc := service.New(store.NewMemory(), nil)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	j, _ := model.ParseJourney([]byte(examples.Checkout))
	for _, link := range j.Spec.SLOs {
		if err := svc.Create(ctx, model.SLO{ID: link.ID, Name: link.ID, Project: "demo", Service: "s", Target: .99, WindowSeconds: 60, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
		good := 100.
		if !link.Required {
			good = 0
		}
		if err := svc.Ingest(ctx, link.ID, []model.Point{{Time: now, Good: good, Total: 100}}); err != nil {
			t.Fatal(err)
		}
	}
	rev, err := svc.ApplyJourney(ctx, j, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.JourneyView(ctx, rev)
	if err != nil || v.Health != "healthy" || v.Links[2].Health != "degraded" {
		t.Fatal("advisory SLO incorrectly gates journey", v, err)
	}
}
