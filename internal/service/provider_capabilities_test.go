package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type namesOnly struct{}

func (namesOnly) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	return nil, nil
}
func (namesOnly) MetricNames(context.Context, string, time.Time) ([]string, bool, error) {
	return []string{"api.requests"}, false, nil
}
func TestMetricNameCapabilityIsIndependent(t *testing.T) {
	svc := service.New(store.NewMemory(), namesOnly{})
	names, err := svc.DiscoverMetricNames(context.Background(), "primary", "api")
	if err != nil || len(names.Names) != 1 {
		t.Fatal(names, err)
	}
	if _, err = svc.DiscoverTelemetry(context.Background(), "primary", "api_requests", nil); !errors.Is(err, model.ErrUnavailable) {
		t.Fatal("missing discovery capability was not explicit", err)
	}
}
