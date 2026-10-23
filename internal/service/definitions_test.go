package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestSLODefinitionCreateCASAndConfigurationUpdates(t *testing.T) {
	ctx := context.Background()
	s := service.New(store.NewMemory(), nil)
	d, err := model.ParseDefinition([]byte(examples.APIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PlanDefinition(ctx, d)
	if err != nil || p.Action != "create" || p.ExpectedRevision != 0 {
		t.Fatal(p, err)
	}
	var successes atomic.Int64
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := s.ApplyDefinition(ctx, d, 0)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, model.ErrRevisionConflict) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("non-atomic create", successes.Load())
	}
	p, err = s.PlanDefinition(ctx, d)
	if err != nil || p.Action != "unchanged" || p.ExpectedRevision != 1 {
		t.Fatal(p, err)
	}
	r, err := s.ApplyDefinition(ctx, d, 1)
	if err != nil || r.Revision != 1 || r.Digest != p.Digest {
		t.Fatal(r, err)
	}
	d.SLO.Spec.Target = .95
	p, err = s.PlanDefinition(ctx, d)
	if err != nil || p.Action != "update" {
		t.Fatal(p, err)
	}
	r, err = s.ApplyDefinition(ctx, d, 1)
	if err != nil || r.Revision != 2 {
		t.Fatal(r, err)
	}
	if _, err = s.ApplyDefinition(ctx, d, 1); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale config accepted", err)
	}
	d.SLO.Spec.StepSeconds = 30
	if plan, err := s.PlanDefinition(ctx, d); err != nil || !plan.MeasurementChanged || plan.Action != "update" || len(plan.Warnings) == 0 {
		t.Fatal("measurement edit cannot be reviewed", plan, err)
	}
	rec, err := s.Store.Read(ctx, d.SLO.Metadata.Name)
	if err != nil || rec.SLO.Target != .95 || rec.SLO.Revision != 2 {
		t.Fatal(rec, err)
	}
}
