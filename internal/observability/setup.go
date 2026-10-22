// Package observability emits OTLP/HTTP metrics, logs and traces. Export is
// enabled only by explicit endpoint configuration; telemetry never gates work.
package observability

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Runtime struct {
	Logger *slog.Logger
	stop   []func(context.Context) error
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	var err error
	for i := len(r.stop) - 1; i >= 0; i-- {
		err = errors.Join(err, r.stop[i](ctx))
	}
	return err
}
func signalEnabled(signal string) (bool, error) {
	exporter := os.Getenv("OTEL_" + signal + "_EXPORTER")
	if exporter != "" && exporter != "otlp" && exporter != "none" {
		return false, fmt.Errorf("OTEL_%s_EXPORTER must be otlp or none", signal)
	}
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if exporter == "none" {
		return false, nil
	}
	if endpoint == "" {
		if exporter == "otlp" {
			return false, fmt.Errorf("OTLP %s requires an explicit endpoint", signal)
		}
		return false, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return false, fmt.Errorf("invalid OTLP %s endpoint", signal)
	}
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	if protocol != "" && protocol != "http/protobuf" {
		return false, fmt.Errorf("OTLP %s supports http/protobuf", signal)
	}
	return true, nil
}
func Setup(ctx context.Context, out io.Writer) (r *Runtime, err error) {
	r = &Runtime{}
	defer func() {
		if err != nil {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = r.Shutdown(shutdown)
		}
	}()
	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = "nines-server"
	}
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithTelemetrySDK(), resource.WithAttributes(attribute.String("service.name", name)))
	if err != nil {
		return r, fmt.Errorf("telemetry resource configuration failed")
	}
	hasInstance := false
	for _, a := range res.Attributes() {
		if a.Key == "service.instance.id" && a.Value.AsString() != "" {
			hasInstance = true
		}
	}
	if !hasInstance {
		res, _ = resource.Merge(res, resource.NewSchemaless(attribute.String("service.instance.id", rand.Text())))
	}
	disabled := false
	if v := os.Getenv("OTEL_SDK_DISABLED"); v != "" {
		disabled, err = strconv.ParseBool(v)
		if err != nil {
			return r, fmt.Errorf("OTEL_SDK_DISABLED must be a boolean")
		}
	}
	enabled := map[string]bool{}
	if !disabled {
		for _, signal := range []string{"TRACES", "METRICS", "LOGS"} {
			enabled[signal], err = signalEnabled(signal)
			if err != nil {
				return r, err
			}
		}
	}
	var handlers []slog.Handler
	handlers = append(handlers, consoleHandler(out, res))
	if enabled["TRACES"] {
		ratio := 1.0
		if v := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); v != "" {
			ratio, err = strconv.ParseFloat(v, 64)
			if err != nil || !(ratio >= 0 && ratio <= 1) {
				return r, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be between zero and one")
			}
		}
		sampler := os.Getenv("OTEL_TRACES_SAMPLER")
		if sampler != "" && sampler != "parentbased_traceidratio" && sampler != "parentbased_always_on" {
			return r, fmt.Errorf("unsupported OTEL_TRACES_SAMPLER")
		}
		if sampler == "parentbased_always_on" {
			ratio = 1
		}
		exporter, e := otlptracehttp.New(ctx)
		if e != nil {
			return r, fmt.Errorf("initialize OTLP traces exporter")
		}
		tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))), sdktrace.WithBatcher(exporter))
		r.stop = append(r.stop, tp.Shutdown)
		otel.SetTracerProvider(tp)
	}
	if enabled["METRICS"] {
		exporter, e := otlpmetrichttp.New(ctx)
		if e != nil {
			return r, fmt.Errorf("initialize OTLP metrics exporter")
		}
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithCardinalityLimit(2000), sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
		r.stop = append(r.stop, mp.Shutdown)
		otel.SetMeterProvider(mp)
		meter := mp.Meter(scope)
		goroutines, _ := meter.Int64ObservableGauge("go.goroutine.count", metric.WithUnit("{goroutine}"))
		heap, _ := meter.Int64ObservableGauge("go.memory.used", metric.WithUnit("By"))
		_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			o.ObserveInt64(goroutines, int64(runtime.NumGoroutine()))
			o.ObserveInt64(heap, int64(mem.HeapAlloc))
			return nil
		}, goroutines, heap)
		if err != nil {
			return r, fmt.Errorf("initialize runtime metrics")
		}
	}
	if enabled["LOGS"] {
		exporter, e := otlploghttp.New(ctx)
		if e != nil {
			return r, fmt.Errorf("initialize OTLP logs exporter")
		}
		lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)))
		r.stop = append(r.stop, lp.Shutdown)
		otel.SetLoggerProvider(lp)
		handlers = append(handlers, otelslog.NewHandler(scope, otelslog.WithLoggerProvider(lp)))
	}
	// Propagate trace context only. User-controlled baggage is not exported.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	r.Logger = slog.New(&logHandler{handlers: handlers})
	slog.SetDefault(r.Logger)
	// SDK failures stay on stderr and out of the exporter to avoid recursion.
	fallback := slog.New(&logHandler{handlers: []slog.Handler{consoleHandler(os.Stderr, res)}})
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) { fallback.Error("OpenTelemetry export failed") }))
	r.Logger.Info("observability configured", "traces.enabled", enabled["TRACES"], "metrics.enabled", enabled["METRICS"], "logs.enabled", enabled["LOGS"])
	return r, nil
}

const scope = "reliability.platform/observability"

func serviceAttributes(res *resource.Resource) []slog.Attr {
	attrs := []slog.Attr{}
	for _, a := range res.Attributes() {
		if strings.HasPrefix(string(a.Key), "service.") || a.Key == "deployment.environment.name" {
			attrs = append(attrs, slog.Any(string(a.Key), a.Value.AsInterface()))
		}
	}
	return attrs
}
