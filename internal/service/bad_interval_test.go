package service_test

import (
	"context"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestBadIntervalSLOIsStoredAndNeverMixesWithInlineMeasurement(t *testing.T) {
	ctx := context.Background()
	s := service.New(store.NewMemory(), &testtelemetry.Source{})
	m := v1.NewSLO("noisy-client", "test", v1.SLOSpec{
		DisplayName: "Client success", Service: "client", Source: "primary", Target: .999, WindowSeconds: 3600, StepSeconds: 60,
		GoodQuery: `sum(increase(client_ok_total[1m]))`, TotalQuery: `sum(increase(client_total[1m]))`,
		BadInterval: &v1.BadIntervalPolicy{ErrorRate: 0.06, MinEvents: 20},
	})
	if _, err := s.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&m)}, 0); err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadSLO(ctx, "noisy-client")
	if err != nil || view.SLO.BadInterval == nil || view.SLO.BadInterval.ErrorRate != 0.06 || view.SLO.BadInterval.MinEvents != 20 {
		t.Fatalf("badInterval not stored: %+v %v", view.SLO.BadInterval, err)
	}
	// An inline measurement derives its own provider windows, so it cannot be judged per interval.
	mixed := inlineSLO()
	mixed.Metadata.Name = "mixed"
	mixed.Spec.BadInterval = &v1.BadIntervalPolicy{ErrorRate: 0.06}
	if _, err := s.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&mixed)}, 0); err == nil {
		t.Fatal("badInterval was accepted with an inline measurement")
	}
}

func TestSessionSLOIsAppliedAndItsMeasurementContractChecksBothCounters(t *testing.T) {
	ctx := context.Background()
	source := &testtelemetry.Source{}
	s := service.New(store.NewMemory(), source)
	m := v1.NewSLO("ios-crash-free", "test", v1.SLOSpec{
		DisplayName: "iOS crash-free sessions", Service: "ios", Source: "primary", Target: .995, WindowSeconds: 3600, StepSeconds: 60,
		Measurement: &v1.MeasurementSpec{Type: "sessions", Metric: "app_sessions_total", EventMetric: "app_sessions_crashed_total", Labels: map[string]string{"service": "api"}, ScrapeIntervalSeconds: 10, ObservationPoint: "One increment per app session; the event counter increments once per crashed session."},
	})
	if _, err := s.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&m)}, 0); err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadSLO(ctx, "ios-crash-free")
	if err != nil || view.SLO.ProviderWindow == nil || view.SLO.ProviderWindow.BadSelector == "" || view.SLO.ProviderWindow.GoodSelector != "" {
		t.Fatalf("%+v %v", view.SLO.ProviderWindow, err)
	}
	health, err := s.CheckMeasurement(ctx, "ios-crash-free")
	if err != nil || health.State != "compatible" || health.EvaluationBlocked {
		t.Fatalf("both counters exist, so the contract holds: %+v %v", health, err)
	}
	// Truncated evidence cannot establish the contract; it must not read as compatible.
	source.Truncated = true
	health, err = s.CheckMeasurement(ctx, "ios-crash-free")
	if err != nil || health.State == "compatible" {
		t.Fatalf("insufficient evidence must not be compatible: %+v %v", health, err)
	}
}
