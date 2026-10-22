package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type requestKey struct{}
type requestInfo struct{ route, tenant string }

func Route(ctx context.Context, pattern string) {
	if info, ok := ctx.Value(requestKey{}).(*requestInfo); ok && pattern != "" {
		if _, path, found := strings.Cut(pattern, " "); found {
			pattern = path
		}
		info.route = pattern
	}
}

// Tenant is called only after successful server-side tenant authorization.
func Tenant(ctx context.Context, id string) {
	if info, ok := ctx.Value(requestKey{}).(*requestInfo); ok {
		info.tenant = id
	}
}
func fallbackRoute(path string) string {
	switch {
	case path == "/healthz":
		return "/healthz"
	case path == "/api/tenants":
		return "/api/tenants"
	case strings.HasPrefix(path, "/api/"):
		return "/api/*"
	case strings.HasPrefix(path, "/auth/"):
		return "/auth/*"
	case strings.HasPrefix(path, "/assets/"):
		return "/assets/*"
	default:
		return "/"
	}
}
func method(m string) string {
	switch m {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
		return m
	}
	return "_OTHER"
}
func HTTP(next http.Handler, log *slog.Logger) http.Handler {
	meter := otel.Meter(scope)
	duration, _ := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 120))
	requests, _ := meter.Int64Counter("http.server.request.count", metric.WithUnit("{request}"))
	active, _ := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := &requestInfo{route: fallbackRoute(r.URL.Path)}
		parent := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer(scope).Start(context.WithValue(parent, requestKey{}, info), method(r.Method)+" "+info.route, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		active.Add(ctx, 1)
		defer active.Add(ctx, -1)
		metrics := httpsnoop.CaptureMetricsFn(w, func(w http.ResponseWriter) { next.ServeHTTP(w, r.WithContext(ctx)) })
		attrs := []attribute.KeyValue{attribute.String("http.request.method", method(r.Method)), attribute.String("http.route", info.route), attribute.Int("http.response.status_code", metrics.Code)}
		if info.tenant != "" {
			attrs = append(attrs, attribute.String("nines.tenant", info.tenant))
		}
		span.SetName(method(r.Method) + " " + info.route)
		span.SetAttributes(attrs...)
		if metrics.Code >= 500 {
			span.SetStatus(codes.Error, "server request failed")
		}
		opts := metric.WithAttributes(attrs...)
		duration.Record(ctx, metrics.Duration.Seconds(), opts)
		requests.Add(ctx, 1, opts)
		fields := []any{"http.request.method", method(r.Method), "http.route", info.route, "http.response.status_code", metrics.Code, "duration_seconds", metrics.Duration.Seconds(), "response_bytes", metrics.Written}
		if info.tenant != "" {
			fields = append(fields, "nines.tenant", info.tenant)
		}
		log.Log(ctx, slog.LevelInfo, "HTTP request completed", fields...)
	})
}

type Transport struct {
	Base        http.RoundTripper
	PeerService string
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	attrs := []attribute.KeyValue{attribute.String("http.request.method", method(r.Method)), attribute.String("server.address", r.URL.Hostname())}
	if port, err := strconv.Atoi(r.URL.Port()); err == nil {
		attrs = append(attrs, attribute.Int("server.port", port))
	}
	name := "HTTP " + method(r.Method)
	if t.PeerService != "" {
		attrs = append(attrs, attribute.String("peer.service", t.PeerService))
		name = t.PeerService + " " + name
	}
	ctx, span := otel.Tracer(scope).Start(r.Context(), name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	defer span.End()
	request := r.Clone(ctx)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))
	start := time.Now()
	response, err := base.RoundTrip(request)
	status := 0
	if response != nil {
		status = response.StatusCode
		span.SetAttributes(attribute.Int("http.response.status_code", status))
	}
	if err != nil || status >= 500 {
		span.SetStatus(codes.Error, "outbound HTTP failed")
		if err != nil {
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		}
	}
	histogram, _ := otel.Meter(scope).Float64Histogram("http.client.request.duration", metric.WithUnit("s"))
	histogram.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(append(attrs, attribute.Int("http.response.status_code", status))...))
	return response, err
}
func Operation(ctx context.Context, name, tenant string) (context.Context, func(error)) {
	attrs := []attribute.KeyValue{attribute.String("nines.operation", name)}
	if tenant != "" {
		attrs = append(attrs, attribute.String("nines.tenant", tenant))
	}
	ctx, span := otel.Tracer(scope).Start(ctx, name, trace.WithAttributes(attrs...))
	start := time.Now()
	return ctx, func(err error) {
		outcome := "succeeded"
		if err != nil {
			outcome = "failed"
			span.SetStatus(codes.Error, "operation failed")
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		}
		dims := metric.WithAttributes(append(attrs, attribute.String("outcome", outcome))...)
		histogram, _ := otel.Meter(scope).Float64Histogram("nines.operation.duration", metric.WithUnit("s"))
		histogram.Record(ctx, time.Since(start).Seconds(), dims)
		counter, _ := otel.Meter(scope).Int64Counter("nines.operation.count", metric.WithUnit("{operation}"))
		counter.Add(ctx, 1, dims)
		slog.InfoContext(ctx, "Platform operation completed", "operation", name, "nines.tenant", tenant, "outcome", outcome, "duration_seconds", time.Since(start).Seconds())
		span.End()
	}
}
