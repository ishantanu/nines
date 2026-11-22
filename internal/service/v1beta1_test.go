package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func apply(t *testing.T, svc *service.Service, text string) (model.Definition, error) {
	t.Helper()
	d, err := model.ParseDefinition([]byte(text))
	if err != nil {
		return d, err
	}
	_, err = svc.ApplyDefinition(context.Background(), d, 0)
	return d, err
}

const betaSLO = `apiVersion: nines.dev/v1beta1
kind: ServiceLevelObjective
metadata:
  name: search-latency
  project: storefront
spec:
  displayName: Search latency
  service: search
  target: 0.99
  queries:
    good: sum(increase(search_fast_total[1m]))
    total: sum(increase(search_total[1m]))
`

// A v1beta1 SLO written with no window, step or policies is stored with the
// documented defaults, and the conversion notes say so.
func TestV1beta1SLOAppliesWithDefaults(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	d, err := apply(t, svc, betaSLO)
	if err != nil {
		t.Fatal(err)
	}
	if d.SourceAPIVersion != "nines.dev/v1beta1" || !strings.Contains(strings.Join(d.Notes, "\n"), "window defaults to 28d") {
		t.Fatal(d.SourceAPIVersion, d.Notes)
	}
	r, err := svc.Store.Read(context.Background(), "search-latency")
	if err != nil {
		t.Fatal(err)
	}
	if r.SLO.WindowSeconds != 28*86400 || r.SLO.StepSeconds != 60 || len(r.SLO.Policies) != 3 {
		t.Fatalf("stored window %d step %d policies %d", r.SLO.WindowSeconds, r.SLO.StepSeconds, len(r.SLO.Policies))
	}
}

func TestV1beta1JourneyDimensionsFollowStoredSLOs(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	latency := strings.Replace(betaSLO, "  queries:\n    good: sum(increase(search_fast_total[1m]))\n    total: sum(increase(search_total[1m]))\n", `  measurement:
    type: latency
    metric: http_server_request_duration_seconds
    threshold: 300ms
    observationPoint: Search API handler.
`, 1)
	if _, err := apply(t, svc, latency); err != nil {
		t.Fatal(err)
	}
	journey := `apiVersion: nines.dev/v1beta1
kind: CriticalUserJourney
metadata:
  name: find-product
  project: storefront
spec:
  displayName: Find a product
  description: A shopper searches the catalog.
  owner: team-discovery
  criticality: high
  slos:
    - id: search-latency
      required: true
  steps:
    - id: search
      name: Search
      actor: user
      sloRefs: [search-latency]
`
	if _, err := apply(t, svc, journey); err != nil {
		t.Fatal(err)
	}
	views, err := svc.ListJourneys(context.Background())
	if err != nil || len(views) != 1 || views[0].Resource.Manifest.Spec.SLOs[0].Dimension != "latency" {
		t.Fatal("omitted dimension was not taken from the latency SLO", views, err)
	}
	contradiction := strings.Replace(strings.Replace(journey, "find-product", "browse", 1), "      required: true\n", "      required: true\n      dimension: availability\n", 1)
	if _, err := apply(t, svc, contradiction); err == nil || !strings.Contains(err.Error(), "contradicts the SLO, which measures latency") {
		t.Fatal(err)
	}
}

func TestV1beta1BindingAndMapParse(t *testing.T) {
	for _, text := range []string{`apiVersion: nines.dev/v1beta1
kind: MeasurementBinding
metadata:
  name: checkout-requests
  project: storefront
spec:
  displayName: Checkout requests
  service: checkout
  type: availability
  metric: http_requests_total
  labels:
    service: checkout
  successLabels:
    outcome: success
  scrapeInterval: 15s
  observationPoint: Checkout API handler.
`, `apiVersion: nines.dev/v1beta1
kind: ServiceDependencyMap
metadata:
  name: commerce
  project: storefront
spec:
  displayName: Commerce
  services:
    - name: checkout
      displayName: Checkout
      sloRefs: []
  dependencies: []
`} {
		d, err := model.ParseDefinition([]byte(text))
		if err != nil || d.SourceAPIVersion != "nines.dev/v1beta1" {
			t.Fatal(d.Kind(), err)
		}
	}
	if _, err := model.ParseDefinition([]byte(strings.Replace(betaSLO, "target: 0.99", "target: 0.99\n  windowSeconds: 86400", 1))); err == nil {
		t.Fatal("a v1alpha1 field was accepted in a v1beta1 definition")
	}
}
