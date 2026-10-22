// Package testtelemetry supplies synthetic data for isolated integration tests.
package testtelemetry

import (
	"context"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type Source struct {
	Route         string
	Fail          bool
	Truncated     bool
	MissingBucket bool
}

func (c *Source) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	return nil, nil
}
func (c *Source) MetricNames(context.Context, string, time.Time) ([]string, bool, error) {
	return []string{"http_requests_total", "http_duration_seconds"}, false, nil
}
func (c *Source) Discover(_ context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	if c.Fail {
		return v1.TelemetryDiscovery{}, errors.New("secret-provider-credential")
	}
	d := v1.TelemetryDiscovery{Metric: metric, From: at.Add(-15 * time.Minute), To: at, Truncated: c.Truncated, Warnings: []string{"Synthetic evidence, not a live provider."}}
	if metric == "http_duration_seconds" {
		d.Metadata = []v1.TelemetryMetadata{{Type: "histogram", Unit: "seconds"}}
		d.Series = []map[string]string{{"__name__": metric + "_count", "service": "api"}, {"__name__": metric + "_bucket", "service": "api", "le": "+Inf"}}
		if !c.MissingBucket {
			d.Series = append(d.Series, map[string]string{"__name__": metric + "_bucket", "service": "api", "le": "0.3"})
		}
	} else {
		d.Metadata = []v1.TelemetryMetadata{{Type: "counter"}}
		d.Series = []map[string]string{{"__name__": metric, "service": "api", "status": "200"}, {"__name__": metric, "service": "api", "status": "500"}}
	}
	return d, nil
}
func (c *Source) ObserveDependencies(context.Context, v1.GraphObservation, time.Time) ([]v1.ObservedEdge, bool, error) {
	if c.Fail {
		return nil, false, errors.New("secret-provider-credential")
	}
	return []v1.ObservedEdge{{Caller: "api", Callee: "database", ConnectionType: "database", Requests: 42}}, c.Truncated, nil
}
func Binding() model.MeasurementBinding {
	return model.MeasurementBinding{APIVersion: v1.APIVersion, Kind: v1.BindingKind, Metadata: v1.Metadata{Name: "requests", Project: "test"}, Spec: v1.BindingSpec{DisplayName: "Request success", Service: "api", Source: "primary", Measurement: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, StepSeconds: 60, ScrapeIntervalSeconds: 10, ObservationPoint: "API server completion"}}
}
func Graph(observed bool) model.ServiceDependencyMap {
	g := model.ServiceDependencyMap{APIVersion: v1.APIVersion, Kind: v1.DependencyMapKind, Metadata: v1.Metadata{Name: "services", Project: "test"}, Spec: v1.DependencyMapSpec{DisplayName: "Service dependencies", Services: []v1.MapService{{Name: "api", DisplayName: "API", SLORefs: []string{"api"}}, {Name: "database", DisplayName: "Database", SLORefs: []string{}}}, Dependencies: []v1.DeclaredDependency{{Caller: "api", Callee: "database", Critical: true}}}}
	if observed {
		g.Spec.Observed = &v1.GraphObservation{Source: "primary", Labels: map[string]string{"environment": "test"}, WindowSeconds: 900}
	}
	return g
}
func New() (s *service.Service, c *Source, err error) {
	c = &Source{}
	s = service.New(store.NewMemory(), c)
	ctx := context.Background()
	err = s.Create(ctx, model.SLO{ID: "api", Project: "test", Service: "api", Name: "API availability", Target: .999, WindowSeconds: 3600, StepSeconds: 60})
	if err != nil {
		return
	}
	b := Binding()
	_, err = s.ApplyDefinition(ctx, model.Definition{Binding: &b}, 0)
	if err != nil {
		return
	}
	g := Graph(true)
	_, err = s.ApplyDefinition(ctx, model.Definition{DependencyMap: &g}, 0)
	if err != nil {
		return
	}
	j := model.Journey{APIVersion: v1.APIVersion, Kind: v1.JourneyKind, Metadata: v1.Metadata{Name: "checkout", Project: "test"}, Spec: v1.JourneySpec{DisplayName: "Checkout", Owner: "commerce", Criticality: "high", SLOs: []v1.JourneySLO{{ID: "api", Dimension: "availability", Required: true}}, Steps: []v1.JourneyStep{{ID: "request", Name: "Complete checkout", Actor: "user", SLORefs: []string{"api"}}}}}
	_, err = s.ApplyJourney(ctx, j, 0)
	return
}

// CollectWindowRange returns explicitly synthetic provider-window observations for
// integration tests and local UI verification. It never contacts a provider.
func (c *Source) CollectWindowRange(ctx context.Context, s model.SLO, from, to time.Time, resolution int64) ([]model.WindowObservation, error) {
	if c.Fail {
		return nil, errors.New("synthetic provider unavailable")
	}
	out := []model.WindowObservation{}
	n := 0
	for at := from.Add(time.Duration(resolution) * time.Second); !at.After(to); at = at.Add(time.Duration(resolution) * time.Second) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n++
		for _, w := range s.Windows() {
			good, total, cov := 999.0, 1000.0, 1.0
			if n%9 == 0 {
				good = 980
			}
			if n%11 == 0 {
				cov = .6
			}
			if n%17 == 0 {
				good = 0
				total = 0
			}
			out = append(out, model.WindowObservation{Time: at, WindowSeconds: w, Revision: model.ConfigRevision(s), Good: good, Total: total, Coverage: cov})
		}
	}
	return out, nil
}

func (c *Source) SourceFingerprint() string {
	return model.DefinitionDigest(struct{ Kind, Route string }{"synthetic-test-telemetry", c.Route})
}
