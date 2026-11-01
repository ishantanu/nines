package service

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

type scheduledSource struct{ badUntil time.Time }

func (c *scheduledSource) Collect(_ context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	points := []model.Point{}
	for at := from.Add(time.Duration(s.StepSeconds) * time.Second); !at.After(to); at = at.Add(time.Duration(s.StepSeconds) * time.Second) {
		good := 100.0
		if !at.After(c.badUntil) {
			good = 50
		}
		points = append(points, model.Point{Time: at, Good: good, Total: 100})
	}
	return points, nil
}
func TestDelayedScheduledCollectionRecoversOnlyWithObservedCoverage(t *testing.T) {
	for _, catchup := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed-tail-leaves-gaps", true: "watermark-catchup"}[catchup], func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1800000000, 0).UTC()
			initial := now
			source := &scheduledSource{badUntil: now.Add(-10 * time.Second)}
			svc := New(store.NewMemory(), source)
			svc.Now = func() time.Time { return now }
			c := model.SLO{ID: "api", Name: "API", Project: "test", Service: "api", Target: .99, WindowSeconds: 120, StepSeconds: 10, GoodQuery: "good", TotalQuery: "total", Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 6, MinCoverage: 1}}
			if err := svc.Create(ctx, c); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Collect(ctx, c.ID, now.Add(-130*time.Second), now.Add(-10*time.Second)); err != nil {
				t.Fatal(err)
			}
			before, err := svc.Status(ctx, c.ID, time.Time{})
			if err != nil || before.Alert.Phase != "firing" {
				t.Fatal(before, err)
			}
			now = now.Add(90 * time.Second)
			end := now.Add(-10 * time.Second)
			from := end.Add(-20 * time.Second)
			if catchup {
				var err error
				from, end, err = svc.scheduledCollectionRange(ctx, c)
				if err != nil {
					t.Fatal(err)
				}
				if !from.Before(initial) {
					t.Fatal("did not recover delayed intervals", from)
				}
			}
			if _, err := svc.Collect(ctx, c.ID, from, end); err != nil {
				t.Fatal(err)
			}
			after, err := svc.Status(ctx, c.ID, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if catchup {
				if after.Coverage != 1 || after.Alert.Phase != "inactive" {
					t.Fatal("observed healthy traffic did not resolve", after)
				}
			} else if after.Coverage == 1 || after.Alert.Phase != "firing" {
				t.Fatal("missing data falsely resolved the alert", after)
			}
		})
	}
}
func TestScheduledCatchupIsBoundedAndRespectsRetention(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0).UTC()
	source := &scheduledSource{}
	svc := New(store.NewMemory(), source)
	svc.Now = func() time.Time { return now }
	for _, window := range []int64{120, 86400} {
		c := model.SLO{ID: "api", Name: "API", Project: "test", Service: "api", Target: .99, WindowSeconds: window, StepSeconds: 1, GoodQuery: "good", TotalQuery: "total"}
		if window == 86400 {
			c.ID = "large"
		}
		if err := svc.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Collect(ctx, c.ID, now.Add(-2*time.Second), now.Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		now = now.Add(10000 * time.Second)
		from, to, err := svc.scheduledCollectionRange(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if to.Sub(from) > 300*time.Second || from.Before(now.Add(-2*time.Duration(window)*time.Second)) || !to.Before(now) {
			t.Fatal("unbounded or future collection", from, to)
		}
	}
}
