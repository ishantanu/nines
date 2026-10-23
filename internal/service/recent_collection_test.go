package service

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

type recentCollector struct{ from, to time.Time }

func (c *recentCollector) Collect(_ context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	c.from = from
	c.to = to
	return []model.Point{{Time: to, Good: 99, Total: 100}}, nil
}
func TestRecentCollectionBoundsSingleAndLargeSteps(t *testing.T) {
	for _, step := range []int64{1, 60, 3600} {
		ctx := context.Background()
		source := &recentCollector{}
		svc := New(store.NewMemory(), source)
		now := time.Date(2026, 10, 23, 12, 0, 32, 0, time.UTC)
		svc.Now = func() time.Time { return now }
		slo := model.SLO{ID: "api", Name: "API", Service: "api", Project: "p", Target: .99, WindowSeconds: step, StepSeconds: step, GoodQuery: "good", TotalQuery: "total"}
		if err := svc.Create(ctx, slo); err != nil {
			t.Fatal(err)
		}
		if count, err := svc.CollectRecent(ctx, slo.ID); err != nil || count != 1 {
			t.Fatal(step, count, err)
		}
		d := time.Duration(step) * time.Second
		if !source.to.Equal(now.Add(-d).Truncate(d)) || source.to.Sub(source.from) != d {
			t.Fatal(step, source.from, source.to)
		}
	}
}
