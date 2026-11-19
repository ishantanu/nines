package service_test

import (
	"context"
	"testing"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestSLOLabelsAreStoredReturnedAndBounded(t *testing.T) {
	ctx := context.Background()
	s := service.New(store.NewMemory(), &testtelemetry.Source{})
	m := inlineSLO()
	m.Spec.Labels = map[string]string{"team": "payments", "product": "Checkout"}
	if _, err := s.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&m)}, 0); err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadSLO(ctx, m.Metadata.Name)
	if err != nil || view.SLO.Labels["team"] != "payments" || view.SLO.Labels["product"] != "Checkout" {
		t.Fatalf("labels not returned: %+v %v", view.SLO.Labels, err)
	}
	// Labels are classification only: the measurement and rules are unchanged.
	if view.SLO.Measurement == nil || view.SLO.ProviderWindow == nil {
		t.Fatalf("measurement lost: %+v", view.SLO)
	}
	bad := inlineSLO()
	bad.Metadata.Name = "bad-labels"
	bad.Spec.Labels = map[string]string{"Team": "x"}
	if _, err := s.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&bad)}, 0); err == nil {
		t.Fatal("an invalid label key was accepted")
	}
	if _, err := s.ReadSLO(ctx, "bad-labels"); err == nil {
		t.Fatal("a rejected definition was stored")
	}
}
