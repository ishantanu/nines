package telemetryprovider

import (
	"context"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type adapter struct{}

func (*adapter) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	return nil, nil
}
func (*adapter) ValidateSLO(model.SLO) error { return nil }
func (*adapter) SourceFingerprint() string   { return "test" }
func (*adapter) OptionalFeature() bool       { return true }
func TestExplicitExtensionsAndDescriptorIsolation(t *testing.T) {
	r := NewRegistry()
	d := Descriptor{Kind: "custom", QueryLanguages: []string{"promql"}, Capabilities: Capabilities{IntervalCounts: true}}
	if err := r.Register(d, func(cfg Configuration) (Collector, error) {
		if cfg.Endpoint != "https://trusted.test" {
			t.Fatal("route lost")
		}
		return &adapter{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	d.QueryLanguages[0] = "mutated"
	got, _ := r.Describe("custom")
	if got.QueryLanguages[0] != "promql" {
		t.Fatal("registration aliases descriptor")
	}
	got.QueryLanguages[0] = "mutated"
	got, _ = r.Describe("custom")
	if got.QueryLanguages[0] != "promql" {
		t.Fatal("description mutates registry")
	}
	c, err := r.Open("custom", Configuration{Endpoint: "https://trusted.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(interface{ OptionalFeature() bool }); !ok {
		t.Fatal("factory erased optional capabilities")
	}
	if err = r.Register(Descriptor{Kind: "custom", QueryLanguages: []string{"promql"}}, func(Configuration) (Collector, error) { return &adapter{}, nil }); err == nil {
		t.Fatal("replacement allowed")
	}
	if _, err = r.Open("uninstalled", Configuration{}); err == nil {
		t.Fatal("uninstalled adapter accepted")
	}
	if err = r.Register(Descriptor{Kind: "unknown-language", QueryLanguages: []string{"missing"}}, func(Configuration) (Collector, error) { return &adapter{}, nil }); err == nil {
		t.Fatal("unknown language accepted")
	}
}
