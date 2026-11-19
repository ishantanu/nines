package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestJourneyRunObjectiveDrivesHealthAndNeverCountsUnjudgedRunsAsSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	s := service.New(store.NewMemory(), nil)
	s.Now = func() time.Time { return now }
	for _, id := range []string{"api-availability", "background-jobs"} {
		if err := s.Create(ctx, model.SLO{ID: id, Name: id, Project: "p", Service: "api", Target: .99, WindowSeconds: 60, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
		if err := s.Ingest(ctx, id, []model.Point{{Time: now, Good: 100, Total: 100}}); err != nil {
			t.Fatal(err)
		}
	}
	j, err := model.ParseJourney([]byte(examples.AgentResolution))
	if err != nil {
		t.Fatal(err)
	}
	j.Spec.RunObjective = &v1.RunObjective{Target: 0.8, WindowSeconds: 24 * 3600, MinRuns: 4}
	rev, err := s.ApplyJourney(ctx, j, 0)
	if err != nil {
		t.Fatal(err)
	}
	view := func() service.JourneyView {
		t.Helper()
		views, err := s.ListJourneys(ctx)
		if err != nil || len(views) != 1 {
			t.Fatal(views, err)
		}
		return views[0]
	}
	// No runs yet: the objective cannot establish health, so it stays unknown.
	if v := view(); v.RunObjective == nil || v.RunObjective.State != "no_runs" || v.Health != "unknown" || len(v.Reasons) == 0 {
		t.Fatalf("a journey with no runs must be unknown, not healthy: %+v", v)
	}
	record := func(id, outcome string, steps int, approved bool) {
		t.Helper()
		cost := 0.05
		_, err := s.RecordRun(ctx, j.Metadata.Name, model.RunEvidence{ID: id, JourneyRevision: rev.Revision, StartedAt: now.Add(-2 * time.Minute), FinishedAt: now.Add(-time.Minute), Outcome: outcome, CostUSD: &cost, Steps: &steps, ApprovalObserved: &approved})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	// Unjudged runs (an unknown outcome is inconclusive) must not make it healthy.
	for i := 0; i < 5; i++ {
		record(fmt.Sprintf("u%d", i), "unknown", 3, true)
	}
	if v := view(); v.RunObjective.State != "no_runs" || v.RunObjective.Unjudged != 5 || v.Health != "unknown" {
		t.Fatalf("inconclusive runs are not successes: %+v", v.RunObjective)
	}
	for i := 0; i < 4; i++ {
		record(fmt.Sprintf("p%d", i), "success", 3, true)
	}
	if v := view(); v.RunObjective.State != "available" || v.RunObjective.Passed != 4 || v.Health != "healthy" {
		t.Fatalf("four passes meet the objective: %+v health %s", v.RunObjective, v.Health)
	}
	// Failures push the pass rate below the 80% target: degraded, with the reason stated.
	record("f1", "failure", 3, true)
	record("f2", "failure", 3, true)
	v := view()
	if v.Health != "degraded" || v.RunObjective.Reliability == nil || *v.RunObjective.Reliability >= 0.8 {
		t.Fatalf("a missed objective must degrade health: %+v health %s", v.RunObjective, v.Health)
	}
	reason := strings.Join(v.Reasons, " ")
	if !strings.Contains(reason, "run objective") || !strings.Contains(reason, "target") {
		t.Fatalf("the reason must say why: %v", v.Reasons)
	}
	// A journey without a run objective is unchanged.
	plain, _ := model.ParseJourney([]byte(examples.Checkout))
	if plain.Spec.RunObjective != nil {
		t.Fatal("the checkout example must not declare a run objective")
	}
}

func TestRunObjectiveRequiresAnAgenticContract(t *testing.T) {
	ctx := context.Background()
	s := service.New(store.NewMemory(), nil)
	for _, id := range []string{"api-availability", "background-jobs"} {
		_ = s.Create(ctx, model.SLO{ID: id, Name: id, Project: "p", Service: "api", Target: .99, WindowSeconds: 60, StepSeconds: 60})
	}
	j, err := model.ParseJourney([]byte(examples.Checkout))
	if err != nil {
		t.Fatal(err)
	}
	j.Spec.RunObjective = &v1.RunObjective{Target: 0.9, WindowSeconds: 86400, MinRuns: 5}
	if _, err = s.ApplyJourney(ctx, j, 0); err == nil {
		t.Fatal("a run objective was accepted on a journey that cannot record runs")
	}
}

func TestPlanComparesATargetChangeWithDeliveredReliability(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	s := service.New(store.NewMemory(), nil)
	s.Now = func() time.Time { return now }
	if err := s.Create(ctx, model.SLO{ID: "api", Name: "API", Project: "p", Service: "api", Target: .9995, WindowSeconds: 60, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	// A perfect window: users have received 100%.
	if err := s.Ingest(ctx, "api", []model.Point{{Time: now, Good: 1000, Total: 1000}}); err != nil {
		t.Fatal(err)
	}
	relaxed := model.DefinitionFromSLO(model.SLO{ID: "api", Name: "API", Project: "p", Service: "api", Target: .99, WindowSeconds: 60, StepSeconds: 60})
	plan, err := s.PlanDefinition(ctx, model.Definition{SLO: &relaxed})
	if err != nil || plan.Action != "update" {
		t.Fatalf("%+v %v", plan, err)
	}
	found := false
	for _, w := range plan.Warnings {
		found = found || strings.Contains(w, "allows about")
	}
	if !found {
		t.Fatalf("relaxing a target far below delivered reliability must be called out: %v", plan.Warnings)
	}
	same := model.DefinitionFromSLO(model.SLO{ID: "api", Name: "API", Project: "p", Service: "api", Target: .9995, WindowSeconds: 60, StepSeconds: 60})
	plan, err = s.PlanDefinition(ctx, model.Definition{SLO: &same})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range plan.Warnings {
		if strings.Contains(w, "delivered") || strings.Contains(w, "allowed failure rate") {
			t.Fatalf("an unchanged target must not produce comparison notes: %v", plan.Warnings)
		}
	}
}
