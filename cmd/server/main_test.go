package main

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestDemoRefreshKeepsJourneyDataCurrent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	svc := service.New(store.NewMemory(), nil)
	svc.Now = func() time.Time { return now }
	if err := seed(ctx, svc); err != nil {
		t.Fatal(err)
	}
	views, err := svc.ListJourneys(ctx)
	if err != nil || len(views) != 2 {
		t.Fatal(views, err)
	}
	for _, v := range views {
		if v.Resource.Manifest.Metadata.Name == "assisted-resolution" && v.Health != "healthy" {
			t.Fatal(v)
		}
	}
	now = now.Add(2 * time.Minute)
	for n, id := range []string{"api-availability", "checkout-success", "background-jobs"} {
		if err := refreshDemoSLO(ctx, svc, id, n, now); err != nil {
			t.Fatal(err)
		}
	}
	views, err = svc.ListJourneys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		for _, l := range v.Links {
			if l.Status.Coverage != 1 {
				t.Fatal("demo aged into missing coverage", l)
			}
		}
	}
	runs, err := svc.Journeys.Runs(ctx, "assisted-resolution")
	if err != nil || len(runs) != 8 {
		t.Fatal(runs, err)
	}
}
