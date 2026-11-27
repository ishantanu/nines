package observability

import (
	"context"
	"errors"
	"net"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// TenantCount is how many tenants a process serves with one state and
// manager: state active or retired, managedBy configuration or console.
type TenantCount struct {
	State, ManagedBy string
	Count            int64
}

// ObserveTenants reports nines.tenants from count on each collection, and
// nines.tenant.registry.last_success from lastSuccess (zero until the first
// successful reconcile). Alert on time() minus the last success to find a
// process that has stopped following the registry.
func ObserveTenants(count func() []TenantCount, lastSuccess func() time.Time) error {
	meter := otel.Meter(scope)
	tenants, err := meter.Int64ObservableGauge("nines.tenants", metric.WithUnit("{tenant}"), metric.WithDescription("Tenants this process serves, by state and manager"))
	if err != nil {
		return err
	}
	last, err := meter.Float64ObservableGauge("nines.tenant.registry.last_success", metric.WithUnit("s"), metric.WithDescription("Unix time of this process's last successful tenant registry reconcile"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, c := range count() {
			o.ObserveInt64(tenants, c.Count, metric.WithAttributes(attribute.String("state", c.State), attribute.String("managed_by", c.ManagedBy)))
		}
		if at := lastSuccess(); !at.IsZero() {
			o.ObserveFloat64(last, float64(at.UnixNano())/1e9)
		}
		return nil
	}, tenants, last)
	return err
}

// TenantsReconciled records one pass over the tenant registry.
func TenantsReconciled(ctx context.Context, duration time.Duration, err error) {
	outcome := "succeeded"
	if err != nil {
		outcome = "failed"
	}
	dims := metric.WithAttributes(attribute.String("outcome", outcome))
	meter := otel.Meter(scope)
	count, _ := meter.Int64Counter("nines.tenant.reconcile", metric.WithUnit("{reconcile}"), metric.WithDescription("Tenant registry reconciles, by outcome"))
	count.Add(ctx, 1, dims)
	durations, _ := meter.Float64Histogram("nines.tenant.reconcile.duration", metric.WithUnit("s"))
	durations.Record(ctx, duration.Seconds(), dims)
}

// TenantOpenFailed counts a registered tenant that could not be served. stage
// is where it failed (open, schedule or mount); the class never carries the
// error text, which can include configuration values.
func TenantOpenFailed(ctx context.Context, stage string, err error) {
	failures, _ := otel.Meter(scope).Int64Counter("nines.tenant.open.failures", metric.WithUnit("{failure}"), metric.WithDescription("Registered tenants that could not be served, by stage and error class"))
	failures.Add(ctx, 1, metric.WithAttributes(attribute.String("stage", stage), attribute.String("error.class", ErrorClass(err))))
}

// ErrorClass is a coarse, secret-free description of an error for logs and
// metrics: timeout, canceled, network or other.
func ErrorClass(err error) string {
	var network net.Error
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.As(err, &network):
		if network.Timeout() {
			return "timeout"
		}
		return "network"
	default:
		return "other"
	}
}
