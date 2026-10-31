export const telemetryProviders = [
  {
    kind: "prometheus",
    label: "Prometheus",
    endpoint: "http://prometheus:9090",
  },
  { kind: "mimir", label: "Mimir", endpoint: "http://mimir:9009/prometheus" },
  {
    kind: "datadog",
    label: "Datadog",
    endpoint: "https://api.uk1.datadoghq.com",
  },
  {
    kind: "newrelic",
    label: "New Relic",
    endpoint: "https://api.eu.newrelic.com/graphql",
  },
  {
    kind: "dash0",
    label: "Dash0",
    endpoint: "https://api.eu-west-1.aws.dash0.com",
  },
  {
    kind: "victoriametrics",
    label: "VictoriaMetrics",
    endpoint: "http://victoriametrics:8428",
  },
  {
    kind: "googlecloud",
    label: "Google Cloud Monitoring",
    endpoint: "https://monitoring.googleapis.com",
  },
] as const;
// Google Cloud project/IAM routes remain server-owned.
export const managedProviders = telemetryProviders.filter(
  (p) => p.kind !== "googlecloud",
);
export function providerLabel(kind: string) {
  return (
    telemetryProviders.find((p) => p.kind === kind)?.label ??
    { manual: "Manual ingestion", multiple: "Multiple providers" }[kind] ??
    kind
  );
}
export function providerCredentialFields(kind: string) {
  switch (kind) {
    case "datadog":
      return [
        { key: "apiKey", label: "Datadog API key", required: true },
        {
          key: "applicationKey",
          label: "Datadog application key",
          required: true,
        },
      ];
    case "newrelic":
      return [
        { key: "userKey", label: "New Relic user API key", required: true },
      ];
    default:
      return [
        {
          key: "bearerToken",
          label: "Bearer token",
          required: kind === "dash0",
        },
      ];
  }
}

// The guided editor verifies telemetry metadata before saving.
export function supportsGuidedMeasurement(kind: string) {
  return ["prometheus", "mimir"].includes(kind);
}
export function supportsMetricDiscovery(kind: string) {
  return ["prometheus", "mimir"].includes(kind);
}
