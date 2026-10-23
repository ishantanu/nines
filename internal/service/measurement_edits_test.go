package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestMeasurementEditArchivesEvidenceAndStartsFresh(t *testing.T) {
	ctx := tenancy.WithPrincipal(context.Background(), tenancy.Principal{Subject: "engineer", Name: "Test Engineer", Kind: "local_user", TenantID: "default", Role: "owner"})
	st := store.NewMemory()
	svc := service.New(st, nil)
	at := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return at }
	original := model.SLO{ID: "editable", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if err := svc.Create(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := svc.Ingest(ctx, original.ID, []model.Point{{Time: at, Good: 99, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	before, _ := st.Read(ctx, original.ID)
	if err := st.Update(ctx, original.ID, func(r *model.Record) error { r.Alert = model.AlertState{Phase: "firing"}; return nil }); err != nil {
		t.Fatal(err)
	}
	next := model.DefinitionFromSLO(before.SLO)
	next.Spec.Service = "checkout"
	next.Spec.GoodQuery = "vector(95)"
	next.Spec.StepSeconds = 30
	plan, err := svc.PlanDefinition(ctx, model.Definition{SLO: &next})
	if err != nil || plan.Action != "update" || !plan.MeasurementChanged || len(plan.Warnings) == 0 {
		t.Fatal(plan, err)
	}
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &next}, 1); err != nil {
		t.Fatal(err)
	}
	current, err := st.Read(ctx, original.ID)
	if err != nil || model.ConfigRevision(current.SLO) != 2 || len(current.Points) != 0 || len(current.Windows) != 0 || current.Alert.Phase != "inactive" || current.Collection.State != "not_observed" {
		t.Fatal(current, err)
	}
	status, err := svc.Status(ctx, original.ID, at)
	if err != nil || status.Reliability != nil || status.Coverage != 0 {
		t.Fatal("old samples evaluated under new queries", status, err)
	}
	archive, err := svc.SLORevision(ctx, original.ID, 1)
	if err != nil || archive.Record.SLO.GoodQuery != "vector(99)" || len(archive.Record.Points) != 1 || archive.Summary.ArchivedBy.Subject != "engineer" {
		t.Fatal(archive, err)
	}
	history, err := svc.HistoryForRevision(ctx, original.ID, 1, at.Add(-time.Minute), at, 60, "")
	if err != nil || history.ConfigRevision != 1 || history.Points[len(history.Points)-1].Reliability == nil || *history.Points[len(history.Points)-1].Reliability != .99 {
		t.Fatal(history, err)
	}
	deliveries, _ := st.Deliveries(ctx)
	if len(deliveries) != 1 || deliveries[0].Event.Reason != "configuration_changed" || deliveries[0].Event.ConfigRevision != 1 {
		t.Fatal(deliveries)
	}
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &next}, 1); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale edit accepted", err)
	}
}
func TestMeasurementModeChangesDoNotReuseWindowsOrIntervalSamples(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	svc := service.New(st, nil)
	at := time.Now().UTC().Truncate(time.Minute)
	original := model.SLO{ID: "modes", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
	if err := svc.Create(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := svc.Ingest(ctx, original.ID, []model.Point{{Time: at, Good: 99, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	d := model.DefinitionFromSLO(original)
	d.Spec.ProviderWindow = &v1.ProviderWindow{GoodSelector: "requests_total{code=\"200\"}", TotalSelector: "requests_total", ScrapeIntervalSeconds: 15}
	if _, err := svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(ctx, original.ID, func(r *model.Record) error {
		o := model.WindowObservation{Time: at, WindowSeconds: 3600, Revision: 2, Good: 99, Total: 100, Coverage: 1}
		r.Windows = map[string]model.WindowObservation{model.WindowKey(3600, at): o}
		r.Collection = v1.CollectionHealth{State: "succeeded", SLORevision: 2, Points: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d.Spec.ProviderWindow = nil
	d.Spec.GoodQuery = "vector(90)"
	d.Spec.TotalQuery = "vector(100)"
	if _, err := svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 2); err != nil {
		t.Fatal(err)
	}
	current, _ := st.Read(ctx, original.ID)
	if len(current.Windows) != 0 || len(current.Points) != 0 || current.Collection.State != "not_observed" {
		t.Fatal("old mode leaked", current)
	}
	archive, err := svc.SLORevision(ctx, original.ID, 2)
	if err != nil || len(archive.Record.Windows) != 1 || archive.Summary.WindowObservations != 1 {
		t.Fatal(archive, err)
	}
}

type delayedEditCollector struct {
	started, release chan struct{}
	fail             bool
}

func (c delayedEditCollector) Collect(_ context.Context, _ model.SLO, _, to time.Time) ([]model.Point, error) {
	close(c.started)
	<-c.release
	if c.fail {
		return nil, errors.New("old provider failed")
	}
	return []model.Point{{Time: to, Good: 99, Total: 100}}, nil
}
func TestLateCollectorCannotPopulateOrPoisonEditedMeasurement(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			c := delayedEditCollector{started: make(chan struct{}), release: make(chan struct{}), fail: fail}
			svc := service.New(store.NewMemory(), c)
			at := time.Now().UTC().Truncate(time.Minute)
			svc.Now = func() time.Time { return at }
			original := model.SLO{ID: "late", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
			ctx := context.Background()
			if err := svc.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := svc.Collect(ctx, original.ID, at.Add(-time.Minute), at); done <- err }()
			<-c.started
			d := model.DefinitionFromSLO(original)
			d.Spec.GoodQuery = "vector(98)"
			if _, err := svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); err != nil {
				close(c.release)
				t.Fatal(err)
			}
			close(c.release)
			if err := <-done; err == nil {
				t.Fatal("stale collection succeeded")
			}
			current, _ := svc.Store.Read(ctx, original.ID)
			if current.Collection.State != "not_observed" || len(current.Points) != 0 {
				t.Fatal("old collector mutated new revision", current)
			}
		})
	}
}
