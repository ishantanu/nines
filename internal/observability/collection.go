package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Tenant and fixed outcome labels only: never SLO IDs, URLs, queries or errors.
func CollectionCompleted(ctx context.Context, tenant string, duration, lag time.Duration, failed bool) {
	outcome := "succeeded"
	if failed {
		outcome = "failed"
	}
	opts := metric.WithAttributes(attribute.String("nines.tenant", tenant), attribute.String("outcome", outcome))
	meter := otel.Meter(scope)
	count, _ := meter.Int64Counter("nines.collector.jobs", metric.WithUnit("{job}"))
	count.Add(ctx, 1, opts)
	durations, _ := meter.Float64Histogram("nines.collector.duration", metric.WithUnit("s"))
	durations.Record(ctx, duration.Seconds(), opts)
	lags, _ := meter.Float64Histogram("nines.collector.schedule.lag", metric.WithUnit("s"))
	lags.Record(ctx, lag.Seconds(), opts)
}
