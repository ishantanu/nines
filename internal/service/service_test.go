package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) *Service {
	t.Helper()
	s := New(store.NewMemory(), nil)
	s.Now = func() time.Time { return now }
	if err := s.Create(context.Background(), model.SLO{ID: "availability", Project: "p", Service: "api", Name: "Availability", Target: .99, WindowSeconds: 600, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestStreamingReplayCorrectionsAndRetryEquivalence(t *testing.T) {
	ctx := context.Background()
	live, replay := setup(t), setup(t)
	all := []model.Point{}
	for i := 9; i >= 0; i-- {
		p := model.Point{Time: now.Add(-time.Duration(i) * time.Minute), Good: float64(90 + i), Total: 100}
		all = append(all, p)
		if err := live.Ingest(ctx, "availability", []model.Point{p}); err != nil {
			t.Fatal(err)
		}
	}
	// Reverse order proves late/out-of-order inserts do not alter the result.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if err := replay.Ingest(ctx, "availability", all); err != nil {
		t.Fatal(err)
	}
	if err := replay.Ingest(ctx, "availability", all); err != nil {
		t.Fatal(err)
	}
	a, _ := live.Status(ctx, "availability", time.Time{})
	b, _ := replay.Status(ctx, "availability", time.Time{})
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("live=%+v replay=%+v", a, b)
	}
	if a.Total != 1000 || a.Good != 945 || math.Abs(*a.Reliability-.945) > 1e-12 {
		t.Fatal(a)
	}
	correction := model.Point{Time: now, Good: 100, Total: 100}
	if err := live.Ingest(ctx, "availability", []model.Point{correction}); err != nil {
		t.Fatal(err)
	}
	a, _ = live.Status(ctx, "availability", time.Time{})
	if a.Good != 955 || a.Total != 1000 {
		t.Fatal("correction was appended rather than replaced", a)
	}
}
func TestInvalidBatchAndCanceledContextAreAtomic(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	bad := []model.Point{{Time: now, Good: 99, Total: 100}, {Time: now.Add(-time.Minute), Good: 101, Total: 100}}
	if err := s.Ingest(ctx, "availability", bad); err == nil {
		t.Fatal("accepted invalid count")
	}
	r, _ := s.Store.Read(ctx, "availability")
	if len(r.Points) != 0 {
		t.Fatal("partially applied invalid batch")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Ingest(canceled, "availability", bad[:1]); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r, _ = s.Store.Read(ctx, "availability")
	if len(r.Points) != 0 {
		t.Fatal("committed canceled request")
	}
}

func TestRetentionPrunesExpiredIntervalsAndRejectsOldImports(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	clock := now
	s.Now = func() time.Time { return clock }
	old := model.Point{Time: now.Add(-19 * time.Minute), Good: 1, Total: 1}
	if err := s.Ingest(ctx, "availability", []model.Point{old}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Minute)
	if err := s.Ingest(ctx, "availability", []model.Point{{Time: clock, Good: 1, Total: 1}}); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Store.Read(ctx, "availability")
	if len(r.Points) != 1 {
		t.Fatal("expired sample was not pruned", r.Points)
	}
	if err := s.Ingest(ctx, "availability", []model.Point{old}); err == nil {
		t.Fatal("accepted sample outside retention")
	}
}

type fakeCollector struct {
	points []model.Point
	err    error
}

func (f *fakeCollector) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	return f.points, f.err
}
func TestBackfillReplacesMissingIntervalsWithoutPartialFailure(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if err := s.Store.Update(ctx, "availability", func(r *model.Record) error { r.SLO.GoodQuery = "good"; r.SLO.TotalQuery = "total"; return nil }); err != // Memory-only fixture setup.
		nil {
		t.Fatal(err)
	}
	fake := &fakeCollector{}
	s.Collector = fake
	if err := s.Ingest(ctx, "availability", []model.Point{{Time: now.Add(-time.Minute), Good: 90, Total: 100}, {Time: now.Add(-2 * time.Minute), Good: 99, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	fake.points = []model.Point{{Time: now.Add(-time.Minute), Good: 100, Total: 100}}
	if _, err := s.Collect(ctx, "availability", now.Add(-3*time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Store.Read(ctx, "availability")
	if len(r.Points) != 1 || r.Points[now.Add(-time.Minute).Unix()].Good != 100 {
		t.Fatal(r.Points)
	}
	fake.err = errors.New("source unavailable")
	if _, err := s.Collect(ctx, "availability", now.Add(-3*time.Minute), now.Add(-time.Minute)); err == nil {
		t.Fatal("collection succeeded")
	}
	r, _ = s.Store.Read(ctx, "availability")
	if len(r.Points) != 1 {
		t.Fatal("failed query removed existing data")
	}
}
func TestWebhookRetryAndStableIdempotencyKey(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	event := model.AlertEvent{ID: "stable-event", SLOID: "availability", Kind: "firing", Time: now}
	if err := s.Store.Update(ctx, "availability", func(r *model.Record) error { r.Events = append(r.Events, event); return nil }); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Idempotency-Key") != event.ID {
			t.Error("missing stable idempotency key")
		}
		var received model.AlertEvent
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil || received.ID != event.ID {
			t.Error("invalid event payload")
		}
		if calls == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	d := NewDispatcher(s.Store, server.URL)
	clock := now
	d.Now = func() time.Time { return clock }
	if ok, err := d.Once(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := d.Once(ctx); ok || err != nil {
		t.Fatal("retried before backoff elapsed", ok, err)
	}
	clock = clock.Add(3 * time.Second)
	if ok, err := d.Once(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	events, _ := s.Store.Deliveries(ctx)
	if len(events) != 1 || events[0].Status != "sent" || events[0].Attempts != 2 {
		t.Fatal(events)
	}
}
