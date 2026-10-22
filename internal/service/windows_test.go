package service

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

type windowSource struct {
	observations []model.WindowObservation
	fail         bool
}

func (w *windowSource) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	panic("interval collector used for provider windows")
}
func (w *windowSource) CollectWindows(context.Context, model.SLO, time.Time, time.Time) ([]model.WindowObservation, error) {
	if w.fail {
		return nil, errors.New("unavailable")
	}
	return w.observations, nil
}
func TestWindowCollectionPersistsAndReplacesOnlyMatchingCutoffs(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000030, 0).UTC()
	at := now.Add(-10 * time.Second)
	source := &windowSource{}
	svc := New(store.NewMemory(), source)
	svc.Now = func() time.Time { return now }
	s := model.SLO{ID: "api", Name: "API", Service: "api", Project: "test", Target: .99, StepSeconds: 10, WindowSeconds: 120, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 2}, Policy: &model.Policy{ShortWindowSeconds: 20, LongWindowSeconds: 40, BurnRate: 5, MinCoverage: 1}}
	if err := svc.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	for _, w := range s.Windows() {
		source.observations = append(source.observations, model.WindowObservation{Time: at, WindowSeconds: w, Revision: 1, Good: 82, Total: 100, Coverage: 1})
	}
	if _, err := svc.Collect(ctx, s.ID, at.Add(-10*time.Second), at); err != nil {
		t.Fatal(err)
	}
	status, err := svc.Status(ctx, s.ID, time.Time{})
	if err != nil || status.Reliability == nil || *status.Reliability != .82 || status.Alert.Phase != "firing" {
		t.Fatal(status, err)
	}
	context, err := svc.SLOContext(ctx, s.ID, v1.ContextOptions{})
	if err != nil || context.Freshness.State != "fresh" || context.Measurement.Reliability == nil || *context.Measurement.Reliability != .82 {
		t.Fatal(context, err)
	}
	history, err := svc.History(ctx, s.ID, at.Add(-10*time.Second), at, 10)
	if err != nil || history.Points[1].Reliability == nil || *history.Points[1].Reliability != .82 {
		t.Fatal(history, err)
	}
	if err = svc.Ingest(ctx, s.ID, []model.Point{{Time: at, Good: 100, Total: 100}}); err == nil {
		t.Fatal("overlapping counts accepted as intervals")
	}
	// Read/clone mutation must not corrupt committed observations.
	rec, _ := svc.Store.Read(ctx, s.ID)
	delete(rec.Windows, model.WindowKey(120, at))
	rec.SLO.ProviderWindow.TotalSelector = "changed"
	rec, _ = svc.Store.Read(ctx, s.ID)
	if len(rec.Windows) != 4 || rec.SLO.ProviderWindow.TotalSelector != "total" {
		t.Fatal(rec)
	}
	source.observations = nil
	if _, err = svc.Collect(ctx, s.ID, at.Add(-10*time.Second), at); err != nil {
		t.Fatal(err)
	}
	status, _ = svc.Status(ctx, s.ID, at)
	if status.Reliability != nil || status.Alert.Phase != "firing" {
		t.Fatal("missing window filled or alert resolved", status)
	}
}

func TestWindowAlertPendingSurvivesWorkerCadence(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000010, 0).UTC()
	source := &windowSource{}
	svc := New(store.NewMemory(), source)
	svc.Now = func() time.Time { return now }
	s := model.SLO{ID: "pending", Name: "Pending", Service: "api", Project: "test", Target: .99, StepSeconds: 10, WindowSeconds: 120, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 2}, Policy: &model.Policy{ShortWindowSeconds: 20, LongWindowSeconds: 40, BurnRate: 5, MinCoverage: 1, ForSeconds: 40}}
	if err := svc.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	origin := now
	// Collection every 15 seconds alternates 10s and 20s jumps in cutoff.
	for i := 0; i < 5; i++ {
		now = origin.Add(time.Duration(i) * 15 * time.Second)
		to := svc.nominalEvaluationAt(s, now)
		from := to.Add(-20 * time.Second)
		source.observations = nil
		for at := from.Add(10 * time.Second); !at.After(to); at = at.Add(10 * time.Second) {
			for _, w := range s.Windows() {
				source.observations = append(source.observations, model.WindowObservation{Time: at, WindowSeconds: w, Revision: 1, Good: 80, Total: 100, Coverage: 1})
			}
		}
		if _, err := svc.Collect(ctx, s.ID, from, to); err != nil {
			t.Fatal(err)
		}
	}
	r, err := svc.Store.Read(ctx, s.ID)
	if err != nil || r.Alert.Phase != "firing" {
		t.Fatal("worker cadence prevented pending completion", r.Alert, err)
	}
	events, _ := svc.Store.Deliveries(ctx)
	if len(events) != 1 {
		t.Fatal(events)
	}
}
