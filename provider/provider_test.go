package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/provider"
)

type externalAdapter struct{}

func (externalAdapter) Collect(context.Context, provider.SLO, time.Time, time.Time) ([]provider.Point, error) {
	return nil, nil
}
func (externalAdapter) ValidateSLO(provider.SLO) error { return nil }
func (externalAdapter) SourceFingerprint() string      { return "synthetic" }
func TestPublicAdapterContract(t *testing.T) {
	r := provider.NewRegistry()
	err := r.Register(provider.Descriptor{Kind: "example", QueryLanguages: []string{"promql"}, Capabilities: provider.Capabilities{IntervalCounts: true}}, func(provider.Configuration) (provider.Collector, error) { return externalAdapter{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Open("example", provider.Configuration{}); err != nil {
		t.Fatal(err)
	}
}
