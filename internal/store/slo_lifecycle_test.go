package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestLifecycleRevisionContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { lifecycleRevisionContract(t, store.NewMemory()) })
	t.Run("postgres", func(t *testing.T) { st, _ := postgres(t); lifecycleRevisionContract(t, st) })
}

func lifecycleRevisionContract(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	svc := service.New(st, nil)
	at := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return at }
	s := model.SLO{ID: "lifecycle", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60, Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 2, MinCoverage: 1, MinEvents: 100}}
	if err := svc.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	var points []model.Point
	for i := 0; i < 3; i++ {
		points = append(points, model.Point{Time: at.Add(-time.Duration(i) * time.Minute), Total: 100})
	}
	if err := svc.Ingest(ctx, s.ID, points); err != nil {
		t.Fatal(err)
	}
	before, err := st.Read(ctx, s.ID)
	if err != nil || before.Alert.Phase != "firing" {
		t.Fatal(before, err)
	}
	d := model.DefinitionFromSLO(before.SLO)
	d.Spec.Lifecycle = v1.LifecycleAspirational
	plan, err := svc.PlanDefinition(ctx, model.Definition{SLO: &d})
	if err != nil || plan.MeasurementChanged || !strings.Contains(strings.Join(plan.Warnings, " "), "disabled") {
		t.Fatal(plan, err)
	}
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); err != nil {
		t.Fatal(err)
	}
	current, err := st.Read(ctx, s.ID)
	if err != nil || current.SLO.Lifecycle != v1.LifecycleAspirational || model.ConfigRevision(current.SLO) != 2 || len(current.Points) != 3 || current.SLO.Policy.MinEvents != 100 {
		t.Fatal(current, err)
	}
	snapshot, err := svc.Status(ctx, s.ID, at)
	if err != nil || snapshot.Reliability == nil || *snapshot.Reliability != 0 || snapshot.Alert.SuppressedReason != "lifecycle_aspirational" {
		t.Fatal(snapshot, err)
	}
	contextView, err := svc.SLOContext(ctx, s.ID, v1.ContextOptions{})
	if err != nil || contextView.Definition.Lifecycle != v1.LifecycleAspirational || contextView.Measurement.AlertSuppressedReason != "lifecycle_aspirational" {
		t.Fatal(contextView, err)
	}
	archive, err := svc.SLORevision(ctx, s.ID, 1)
	if err != nil || !archive.Record.SLO.AlertsEnabled() || archive.Record.Alert.Phase != "firing" || len(archive.Record.Points) != 3 {
		t.Fatal(archive, err)
	}
	d.Spec.Lifecycle = v1.LifecycleCommitted
	d.Spec.Policy.MinEvents = 200
	plan, err = svc.PlanDefinition(ctx, model.Definition{SLO: &d})
	if err != nil || plan.MeasurementChanged || !strings.Contains(strings.Join(plan.Warnings, " "), "Promoting") {
		t.Fatal(plan, err)
	}
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale promotion accepted", err)
	}
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 2); err != nil {
		t.Fatal(err)
	}
	if err = svc.Ingest(ctx, s.ID, points); err != nil {
		t.Fatal(err)
	}
	current, err = st.Read(ctx, s.ID)
	if err != nil || current.Alert.Phase != "inactive" || current.SLO.Lifecycle != v1.LifecycleCommitted || current.SLO.Policy.MinEvents != 200 || len(current.Points) != 3 {
		t.Fatal(current, err)
	}
	archive, err = svc.SLORevision(ctx, s.ID, 2)
	if err != nil || archive.Record.SLO.Lifecycle != v1.LifecycleAspirational || archive.Record.SLO.Policy.MinEvents != 100 {
		t.Fatal(archive, err)
	}
}

func TestOperationalPolicyRevisionsRetainMeasurement(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var st store.Store = store.NewMemory()
			if backend == "postgres" {
				st, _ = postgres(t)
			}
			ctx := context.Background()
			svc := service.New(st, nil)
			at := time.Now().UTC().Truncate(time.Minute)
			svc.Now = func() time.Time { return at }
			original := model.SLO{ID: "quality-budget", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60}
			if err := svc.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if err := svc.Ingest(ctx, original.ID, []model.Point{{Time: at.Add(-time.Duration(i) * time.Minute), Good: 100, Total: 100}}); err != nil {
					t.Fatal(err)
				}
			}
			d := model.DefinitionFromSLO(original)
			d.Spec.Quality = &v1.MeasurementQualityPolicy{MinEvents: 200, CohortLabels: []string{"region"}}
			d.Spec.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
			plan, err := svc.PlanDefinition(ctx, model.Definition{SLO: &d})
			if err != nil || plan.MeasurementChanged {
				t.Fatal(plan, err)
			}
			if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); err != nil {
				t.Fatal(err)
			}
			current, _ := st.Read(ctx, original.ID)
			current.SLO.Quality.CohortLabels[0] = "mutated"
			current.SLO.ErrorBudgetPolicy.Owner = "mutated"
			saved, _ := st.Read(ctx, original.ID)
			if len(saved.Points) != 3 || saved.SLO.Quality.CohortLabels[0] != "region" || saved.SLO.ErrorBudgetPolicy.Owner != "oncall" {
				t.Fatal("stored policies escaped by alias", saved)
			}
			decision, err := svc.ErrorBudgetDecision(ctx, original.ID, 2)
			if err != nil || decision.State != "proceed" {
				t.Fatal(decision, err)
			}
			d.Spec.Quality.MinEvents = 301
			if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 2); err != nil {
				t.Fatal(err)
			}
			decision, err = svc.ErrorBudgetDecision(ctx, original.ID, 3)
			if err != nil || decision.State != "insufficient_evidence" {
				t.Fatal(decision, err)
			}
			archive, err := svc.SLORevision(ctx, original.ID, 2)
			if err != nil || archive.Record.SLO.Quality.MinEvents != 200 || len(archive.Record.Points) != 3 {
				t.Fatal(archive, err)
			}
		})
	}
}
