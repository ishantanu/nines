package telemetryprovider

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type WindowCollector interface {
	CollectWindows(context.Context, model.SLO, time.Time, time.Time) ([]model.WindowObservation, error)
}
type WindowPreviewer interface {
	CollectWindowRange(context.Context, model.SLO, time.Time, time.Time, int64) ([]model.WindowObservation, error)
}
type MetricNamesSource interface {
	MetricNames(context.Context, string, time.Time) ([]string, bool, error)
}
type MetricDiscoverySource interface {
	Discover(context.Context, string, map[string]string, time.Time) (v1.TelemetryDiscovery, error)
}
type DependencySource interface {
	ObserveDependencies(context.Context, v1.GraphObservation, time.Time) ([]v1.ObservedEdge, bool, error)
}
type CohortSource interface {
	ReadCohorts(context.Context, model.SLO, []string, time.Time) (v1.CohortEvidence, error)
}
type Prober interface{ Probe(context.Context) error }
type RuleSupport interface{ SupportsRuleDeployment() bool }

// MeasurementLimitations supplies static provider caveats to quality reports.
type MeasurementLimitations interface{ MeasurementLimitations() []string }
