package observability

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	logpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestOTLPThreeSignalsCorrelationAndRedaction(t *testing.T) {
	var mu sync.Mutex
	received := map[string][]string{}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Error("not OTLP protobuf", r.Header)
		}
		var message proto.Message
		switch r.URL.Path {
		case "/v1/traces":
			message = &tracepb.ExportTraceServiceRequest{}
		case "/v1/metrics":
			message = &metricpb.ExportMetricsServiceRequest{}
		case "/v1/logs":
			message = &logpb.ExportLogsServiceRequest{}
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if err := proto.Unmarshal(body, message); err != nil {
			t.Error(err)
		}
		encoded, _ := protojson.Marshal(message)
		mu.Lock()
		received[r.URL.Path] = append(received[r.URL.Path], string(encoded))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer receiver.Close()
	for _, key := range []string{"OTEL_SDK_DISABLED", "OTEL_TRACES_EXPORTER", "OTEL_METRICS_EXPORTER", "OTEL_LOGS_EXPORTER", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "OTEL_RESOURCE_ATTRIBUTES"} {
		t.Setenv(key, "")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", receiver.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_SERVICE_NAME", "nines-test")
	var stdout bytes.Buffer
	oldDefault, oldError := slog.Default(), otel.GetErrorHandler()
	oldTrace, oldMetric, oldLog, oldProp := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetLoggerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(oldDefault)
		otel.SetErrorHandler(oldError)
		otel.SetTracerProvider(oldTrace)
		otel.SetMeterProvider(oldMetric)
		otel.SetLoggerProvider(oldLog)
		otel.SetTextMapPropagator(oldProp)
	})
	rt, err := Setup(context.Background(), &stdout)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("traceparent") == "" {
			t.Error("trace context not propagated")
		}
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	handler := HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Route(r.Context(), "GET /api/slos/{id}/context")
		Tenant(r.Context(), "broker-lab")
		ctx, done := Operation(r.Context(), "slo.collect", "broker-lab")
		tracer := PGTracer{}
		dbctx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT sensitive_sql", Args: []any{"sensitive_argument"}})
		tracer.TraceQueryEnd(dbctx, nil, pgx.TraceQueryEndData{})
		req, _ := http.NewRequestWithContext(ctx, "GET", upstream.URL+"/sensitive_path?password=sensitive_query", nil)
		resp, e := (&http.Client{Transport: Transport{}}).Do(req)
		if e != nil {
			t.Error(e)
		} else {
			_ = resp.Body.Close()
		}
		rt.Logger.InfoContext(ctx, "operation handled", "authorization", "sensitive_header", "password", "sensitive_password", "error", errors.New("sensitive_error"))
		done(nil)
		w.WriteHeader(503)
	}), rt.Logger)
	request := httptest.NewRequest("GET", "http://localhost/api/slos/sensitive_identifier/context?token=sensitive_query", nil)
	request.Header.Set("Authorization", "Bearer sensitive_header")
	request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = rt.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	all := stdout.String()
	for _, signal := range []string{"/v1/traces", "/v1/metrics", "/v1/logs"} {
		if len(received[signal]) == 0 {
			t.Fatal("missing signal", signal)
		}
		data := strings.Join(received[signal], "\n")
		all += data
		if !strings.Contains(data, "nines-test") {
			t.Fatal("resource missing", signal)
		}
	}
	if strings.Contains(all, "sensitive_") {
		t.Fatal("sensitive data exported", all)
	}
	if !strings.Contains(stdout.String(), `"trace_id":"11111111111111111111111111111111"`) || !strings.Contains(stdout.String(), `"severity_text":"INFO"`) || !strings.Contains(stdout.String(), `"body":"HTTP request completed"`) {
		t.Fatal(stdout.String())
	}
	if !strings.Contains(strings.Join(received["/v1/metrics"], ""), "http.server.request.duration") || !strings.Contains(strings.Join(received["/v1/traces"], ""), "/api/slos/{id}/context") {
		t.Fatal(received)
	}
}
func TestExplicitExportConfiguration(t *testing.T) {
	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"} {
		t.Setenv(key, "")
	}
	if enabled, err := signalEnabled("TRACES"); enabled || err != nil {
		t.Fatal(enabled, err)
	}
	for _, bad := range []string{"http://user:password@localhost:4318", "http://localhost:4318?token=secret", "file:///tmp/export"} {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", bad)
		if _, err := signalEnabled("TRACES"); err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
			t.Fatal(err)
		}
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	if enabled, err := signalEnabled("TRACES"); enabled || err != nil {
		t.Fatal(enabled, err)
	}
}
