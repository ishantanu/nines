package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type countedSource struct{ calls int }

func (c *countedSource) Collect(_ context.Context, _ model.SLO, from, to time.Time) ([]model.Point, error) {
	c.calls++
	var points []model.Point
	for at := from.Add(time.Minute); !at.After(to); at = at.Add(time.Minute) {
		points = append(points, model.Point{Time: at, Good: 99, Total: 100})
	}
	return points, nil
}
func TestManualConnectionKeepsExplicitCollectionAndOtherSourcesAutomatic(t *testing.T) {
	collector := &countedSource{}
	svc := service.New(store.NewMemory(), collector)
	svc.Sources = map[string]service.MetricSource{"trial": {Kind: "prometheus", Collector: collector, CollectionMode: "manual"}, "regular": {Kind: "prometheus", Collector: collector}}
	slo := model.SLO{ID: "manual-trial", Name: "Manual trial", Project: "test", Service: "api", Source: "trial", Target: .99, WindowSeconds: 120, StepSeconds: 60, GoodQuery: "good_total", TotalQuery: "requests_total"}
	if svc.AutomaticCollection(slo) {
		t.Fatal("manual connection admitted background queries")
	}
	for _, source := range []string{"primary", "regular"} {
		one := slo
		one.Source = source
		if !svc.AutomaticCollection(one) {
			t.Fatal("normal source unexpectedly disabled", source)
		}
	}
	now := time.Unix(1800000300, 0).UTC()
	svc.Now = func() time.Time { return now }
	if e := svc.Create(context.Background(), slo); e != nil {
		t.Fatal(e)
	}
	count, e := svc.Collect(context.Background(), slo.ID, now.Add(-3*time.Minute), now.Add(-time.Minute))
	if e != nil || count != 2 || collector.calls != 1 {
		t.Fatal("explicit collection blocked", count, collector.calls, e)
	}
	svc.CollectionMode = "manual"
	one := slo
	one.Source = "primary"
	if svc.AutomaticCollection(one) {
		t.Fatal("primary manual policy ignored")
	}
	one.Source = "regular"
	if !svc.AutomaticCollection(one) {
		t.Fatal("named source inherited primary manual policy")
	}
}
