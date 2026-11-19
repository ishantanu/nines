package service

import (
	"context"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestShortObjectiveHistoryOutlivesEvaluationWindow(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	old := now.Add(-time.Hour)
	if err := s.Ingest(ctx, "availability", []model.Point{{Time: old, Good: 90, Total: 100}, {Time: now, Good: 100, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	history, err := s.History(ctx, "availability", old.Add(-time.Minute), now, 60)
	if err != nil {
		t.Fatal(err)
	}
	if history.WindowSeconds != 600 || history.Target != .99 {
		t.Fatal("history altered objective semantics", history)
	}
	found := false
	for _, point := range history.Points {
		if point.At.Equal(old) {
			found = point.Reliability != nil && *point.Reliability == .9
		}
	}
	if !found {
		t.Fatal("missing retained observation beyond two objective windows")
	}
	last := history.Points[len(history.Points)-1]
	if last.Reliability == nil || *last.Reliability != 1 {
		t.Fatal("older history leaked into the current objective window", last)
	}
	if _, err := s.History(ctx, "availability", now.Add(-24*time.Hour-time.Minute), now, 60); err == nil {
		t.Fatal("accepted history outside retention")
	}
	if err := s.Ingest(ctx, "availability", []model.Point{{Time: now.Add(-24 * time.Hour), Good: 1, Total: 1}}); err == nil {
		t.Fatal("accepted observation at expired boundary")
	}
}

func TestHistoryRetentionPreservesLongObjectiveHorizon(t *testing.T) {
	for _, test := range []struct{ window, want int64 }{{60, 86400}, {600, 86400}, {86400, 172800}, {28 * 86400, 56 * 86400}} {
		if got := (model.SLO{WindowSeconds: test.window}).RetentionSeconds(); got != test.want {
			t.Fatalf("window %d: got %d want %d", test.window, got, test.want)
		}
	}
}

func TestMinimumRetentionAlignsToMeasurementIntervals(t *testing.T) {
	c := model.SLO{StepSeconds: 7, WindowSeconds: 70}
	if c.RetentionSeconds() != 86401 || c.RetentionSeconds()%c.StepSeconds != 0 {
		t.Fatal("retention must span complete intervals", c.RetentionSeconds())
	}
}

func TestProviderWindowCollectionRetainsOlderHistoryWithoutChangingLiveSLI(t *testing.T) {
	ctx := context.Background()
	at := now.Add(-10 * time.Second)
	source := &windowSource{}
	svc := New(store.NewMemory(), source)
	svc.Now = func() time.Time { return now }
	c := model.SLO{ID: "window-history", Name: "Window history", Project: "p", Service: "api", Target: .99, WindowSeconds: 120, StepSeconds: 10, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 2}}
	if err := svc.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	for _, cutoff := range []time.Time{at.Add(-time.Hour), at} {
		source.observations = nil
		good := 90.0
		if cutoff.Equal(at) {
			good = 100
		}
		for _, window := range c.Windows() {
			source.observations = append(source.observations, model.WindowObservation{Time: cutoff, WindowSeconds: window, Revision: 1, Good: good, Total: 100, Coverage: 1})
		}
		if _, err := svc.Collect(ctx, c.ID, cutoff.Add(-10*time.Second), cutoff); err != nil {
			t.Fatal(err)
		}
	}
	history, err := svc.History(ctx, c.ID, at.Add(-time.Hour-10*time.Second), at, 10)
	if err != nil {
		t.Fatal(err)
	}
	old := history.Points[1]
	if old.Reliability == nil || *old.Reliability != .9 {
		t.Fatal("old provider observation was pruned", old)
	}
	status, err := svc.Status(ctx, c.ID, time.Time{})
	if err != nil || status.Reliability == nil || *status.Reliability != 1 {
		t.Fatal("old provider history changed live SLI", status, err)
	}
}
