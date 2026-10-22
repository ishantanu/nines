import { context, SpanKind, SpanStatusCode, trace } from "@opentelemetry/api";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { WebTracerProvider } from "@opentelemetry/sdk-trace-web";
import { BatchSpanProcessor } from "@opentelemetry/sdk-trace-base";
import {
  MeterProvider,
  PeriodicExportingMetricReader,
} from "@opentelemetry/sdk-metrics";
import {
  LoggerProvider,
  BatchLogRecordProcessor,
} from "@opentelemetry/sdk-logs";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { OTLPMetricExporter } from "@opentelemetry/exporter-metrics-otlp-http";
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http";
import { telemetryRoute, type TelemetryFetch } from "./observability";

export function setupBrowserTelemetry(endpoint: string): TelemetryFetch {
  const resource = resourceFromAttributes({
    "service.name": "nines-web",
    "service.instance.id": crypto.randomUUID(),
    "service.version": "0.1.0",
  });
  const provider = new WebTracerProvider({
    resource,
    spanProcessors: [
      new BatchSpanProcessor(
        new OTLPTraceExporter({ url: `${endpoint}/v1/traces` }),
        { scheduledDelayMillis: 1000 },
      ),
    ],
  });
  const meters = new MeterProvider({
    resource,
    readers: [
      new PeriodicExportingMetricReader({
        exporter: new OTLPMetricExporter({ url: `${endpoint}/v1/metrics` }),
        exportIntervalMillis: 10000,
      }),
    ],
  });
  const logs = new LoggerProvider({
    resource,
    processors: [
      new BatchLogRecordProcessor({
        exporter: new OTLPLogExporter({ url: `${endpoint}/v1/logs` }),
        scheduledDelayMillis: 1000,
      }),
    ],
  });
  const tracer = provider.getTracer("reliability.platform/browser");
  const meter = meters.getMeter("reliability.platform/browser");
  const duration = meter.createHistogram("http.client.request.duration", {
    unit: "s",
  });
  const count = meter.createCounter("http.client.request.count", {
    unit: "{request}",
  });
  const logger = logs.getLogger("reliability.platform/browser");
  // pagehide is best effort; browsers may terminate before queued export finishes.
  window.addEventListener("pagehide", () => {
    void Promise.allSettled([
      provider.forceFlush(),
      meters.forceFlush(),
      logs.forceFlush(),
    ]);
  });
  return async (path, init) => {
    // Never inject trace context or observe requests to a third-party origin.
    if (!path.startsWith("/api/") || path.startsWith("//"))
      return fetch(path, init);
    const route = telemetryRoute(path);
    const method = (init.method ?? "GET").toUpperCase();
    const span = tracer.startSpan(`${method} ${route}`, {
      kind: SpanKind.CLIENT,
      attributes: { "http.request.method": method, "http.route": route },
    });
    const ctx = trace.setSpan(context.active(), span);
    const headers = new Headers(init.headers);
    const sc = span.spanContext();
    headers.set(
      "traceparent",
      `00-${sc.traceId}-${sc.spanId}-${sc.traceFlags.toString(16).padStart(2, "0")}`,
    );
    const started = performance.now();
    let status = 0;
    try {
      const response = await fetch(path, { ...init, headers });
      status = response.status;
      span.setAttribute("http.response.status_code", status);
      if (status >= 500)
        span.setStatus({
          code: SpanStatusCode.ERROR,
          message: "API request failed",
        });
      return response;
    } catch (error) {
      span.setStatus({
        code: SpanStatusCode.ERROR,
        message: "API transport failed",
      });
      throw error;
    } finally {
      const seconds = (performance.now() - started) / 1000;
      const attributes = {
        "http.request.method": method,
        "http.route": route,
        "http.response.status_code": status,
      };
      duration.record(seconds, attributes, ctx);
      count.add(1, attributes, ctx);
      logger.emit({
        context: ctx,
        severityNumber: status >= 500 || status === 0 ? 17 : 9,
        severityText: status >= 500 || status === 0 ? "ERROR" : "INFO",
        body: "Browser API request completed",
        attributes: { ...attributes, duration_seconds: seconds },
      });
      span.end();
    }
  };
}
