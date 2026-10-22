package main

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestDemoRefreshAfterWindowEdit(t *testing.T) {
	ctx := context.Background()
	svc := service.New(store.NewMemory(), nil)
	now := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return now }
	if err := seed(ctx, svc); err != nil {
		t.Fatal(err)
	}
	rec, err := svc.Store.Read(ctx, "checkout-success")
	if err != nil {
		t.Fatal(err)
	}
	definition := model.DefinitionFromSLO(rec.SLO)
	definition.Spec.WindowSeconds = 3600
	// Keep both severity policies valid for the shortened objective window.
	for i := range definition.Spec.Policies {
		definition.Spec.Policies[i].ShortWindowSeconds = 300
		definition.Spec.Policies[i].LongWindowSeconds = 3600
	}
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &definition}, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err = refreshDemoSLO(ctx, svc, rec.SLO.ID, 1, now); err != nil {
		t.Fatal(err)
	}
	rec, err = svc.Store.Read(ctx, rec.SLO.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Points) != 120 {
		t.Fatalf("wrong retained sample count: %d", len(rec.Points))
	}
	status, err := svc.Status(ctx, rec.SLO.ID, time.Time{})
	if err != nil || status.Coverage != 1 {
		t.Fatal(status, err)
	}
}
