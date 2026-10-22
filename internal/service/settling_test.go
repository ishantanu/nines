package service

import (
	"context"
	"net/url"
	"slices"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestSourceContextUsesCompletedWindowOnlyWithinCollectorCadence(t *testing.T) {
	ctx := context.Background()
	base := time.Unix(1800000000, 0).UTC()
	now := base.Add(32 * time.Second)
	s := New(store.NewMemory(), &contextCollector{})
	s.Now = func() time.Time { return now }
	c := model.SLO{ID: "api", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 120, StepSeconds: 10, GoodQuery: "good", TotalQuery: "total"}
	if err := s.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	end := base.Add(10 * time.Second)
	if err := s.Store.Update(ctx, c.ID, func(r *model.Record) error {
		for i := 0; i < 12; i++ {
			at := end.Add(-time.Duration(i) * 10 * time.Second)
			r.Points[at.Unix()] = model.Point{Time: at, Good: 100, Total: 100}
		}
		r.Collection = v1.CollectionHealth{State: "succeeded", SLORevision: 1, Points: 2, To: end, CompletedAt: base.Add(20 * time.Second)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.SLOContext(ctx, c.ID, v1.ContextOptions{})
	if err != nil || out.Measurement.Coverage != 1 || out.Health != "healthy" || !out.Measurement.EvaluatedAt.Equal(end) || out.Freshness.State != "fresh" || !slices.Contains(out.Warnings, "collection_settling_delay") {
		t.Fatal(out, err)
	}
	ref, err := url.Parse(out.Evidence[0].Href)
	if err != nil {
		t.Fatal(err)
	}
	refTime, err := time.Parse(time.RFC3339Nano, ref.Query().Get("at"))
	if err != nil {
		t.Fatal(err)
	}
	referenced, err := s.SLOContext(ctx, c.ID, v1.ContextOptions{At: refTime})
	if err != nil || referenced.Measurement.Coverage != 1 || !referenced.Measurement.EvaluatedAt.Equal(out.Measurement.EvaluatedAt) {
		t.Fatal("evidence reference changed the measurement cutoff", referenced, err)
	}
	history, err := s.History(ctx, c.ID, time.Time{}, time.Time{}, 0, 240)
	if err != nil || !history.To.Equal(out.Measurement.EvaluatedAt) || history.To.Sub(history.From) != 240*time.Second {
		t.Fatal("rolling history did not use the same completed cutoff", history, err)
	}
	// An explicit wall-clock request retains its missing current interval; it is not silently moved.
	explicit, err := s.SLOContext(ctx, c.ID, v1.ContextOptions{At: now})
	if err != nil || explicit.Measurement.Coverage >= 1 || !explicit.Measurement.EvaluatedAt.Equal(base.Add(20*time.Second)) {
		t.Fatal(explicit, err)
	}
	now = base.Add(38 * time.Second)
	late, err := s.SLOContext(ctx, c.ID, v1.ContextOptions{})
	if err != nil || late.Measurement.Coverage >= 1 || late.Health == "healthy" {
		t.Fatal("overdue collector borrowed old health", late, err)
	}
	now = base.Add(32 * time.Second)
	for _, state := range []string{"failed", "empty"} {
		if err := s.Store.Update(ctx, c.ID, func(r *model.Record) error {
			r.Collection.State = state
			if state == "empty" {
				r.Collection.State = "succeeded"
				r.Collection.Points = 0
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		out, err = s.SLOContext(ctx, c.ID, v1.ContextOptions{})
		if err != nil || out.Measurement.Coverage >= 1 {
			t.Fatal(state, out, err)
		}
	}
}
