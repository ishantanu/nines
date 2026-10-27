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
)

type retiringCollector struct {
	entered chan struct{}
	release chan struct{}
}

func (c *retiringCollector) Collect(_ context.Context, _ model.SLO, _, to time.Time) ([]model.Point, error) {
	close(c.entered)
	<-c.release
	return []model.Point{{Time: to, Good: 99, Total: 100}}, nil
}
func TestRetirementFencesInflightCollectionAndPendingNotifications(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	collector := &retiringCollector{make(chan struct{}), make(chan struct{})}
	svc := service.New(st, collector)
	now := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return now }
	c := model.SLO{ID: "retiring", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if e := svc.Create(ctx, c); e != nil {
		t.Fatal(e)
	}
	if e := st.Update(ctx, c.ID, func(r *model.Record) error {
		r.Alert = model.AlertState{Phase: "firing"}
		r.Events = []model.AlertEvent{{ID: model.NewID(), SLOID: c.ID, Kind: "firing", Time: now}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := svc.Collect(ctx, c.ID, now.Add(-time.Minute), now); done <- e }()
	<-collector.entered
	plan, e := svc.PlanRetirement(ctx, c.ID, "archive")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ApplyRetirement(ctx, c.ID, v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: plan.Digest}); e != nil {
		t.Fatal(e)
	}
	close(collector.release)
	if e = <-done; e == nil {
		t.Fatal("in-flight collector wrote after archival")
	}
	r, e := st.Read(ctx, c.ID)
	if e != nil || len(r.Points) != 0 || r.Alert.Phase != "archived" {
		t.Fatal(r, e)
	}
	events, e := st.Deliveries(ctx)
	if e != nil {
		t.Fatal(e)
	}
	cancelled, resolved := false, false
	for _, d := range events {
		cancelled = cancelled || d.Event.Kind == "firing" && d.Status == "failed"
		resolved = resolved || d.Event.Kind == "resolved" && d.Event.Reason == "slo_archived" && d.Status == "pending"
	}
	if !cancelled || !resolved {
		t.Fatal("notification lifecycle not closed", events)
	}
	definition := model.DefinitionFromSLO(r.SLO)
	if _, e = svc.ApplyDefinition(ctx, model.Definition{SLO: &definition}, 2); e == nil || errors.Is(e, model.ErrNotFound) {
		t.Fatal(e)
	}
}

func TestRetirementKeepsProviderWindowHistoryAtOriginalRevision(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	svc := service.New(st, nil)
	now := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return now }
	c := model.SLO{ID: "window-archive", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 60, WindowSeconds: 3600, ProviderWindow: &v1.ProviderWindow{GoodSelector: "requests_total{code=\"200\"}", TotalSelector: "requests_total", ScrapeIntervalSeconds: 15}}
	if e := svc.Create(ctx, c); e != nil {
		t.Fatal(e)
	}
	at := now
	if e := st.Update(ctx, c.ID, func(r *model.Record) error {
		r.Windows = map[string]model.WindowObservation{model.WindowKey(3600, at): {Time: at, WindowSeconds: 3600, Revision: 1, Good: 99, Total: 100, Coverage: 1}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	plan, e := svc.PlanRetirement(ctx, c.ID, "archive")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ApplyRetirement(ctx, c.ID, v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: plan.Digest}); e != nil {
		t.Fatal(e)
	}
	now = now.Add(90 * 24 * time.Hour)
	status, e := svc.Status(ctx, c.ID, time.Time{})
	if e != nil || status.Reliability == nil || *status.Reliability != .99 || status.Alert.Phase != "archived" {
		t.Fatal("archived window was reinterpreted or aged out", status, e)
	}
	history, e := svc.History(ctx, c.ID, time.Time{}, time.Time{}, 60)
	if e != nil || history.To.After(at) {
		t.Fatal("archived history moved with clock", history, e)
	}
}
