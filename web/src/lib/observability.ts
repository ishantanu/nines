export type TelemetryFetch = (
  input: string,
  init: RequestInit,
) => Promise<Response>;
let instrumentedFetch: TelemetryFetch | undefined;

// Fixed route templates keep IDs, labels and query strings out of telemetry.
export function telemetryRoute(path: string): string {
  const route = path.split("?", 1)[0];
  const fixed = new Set([
    "/api/tenants",
    "/api/workspace",
    "/api/investigations",
    "/api/investigations/capabilities",
    "/api/access",
    "/api/connections",
    "/api/slos",
    "/api/journeys",
    "/api/events",
    "/api/resources/search",
    "/api/intents/plan",
    "/api/intents/draft",
    "/api/intents/backtest",
    "/api/intents/capabilities",
    "/api/resources/validate",
    "/api/resources/plan",
    "/api/resources/apply",
  ]);
  if (fixed.has(route)) return route;
  if (
    /^\/api\/investigations\/[^/]+\/runs\/[^/]+(?:\/(?:cancel|events))?$/.test(
      route,
    )
  )
    return `/api/investigations/{id}/runs/{run}${route.endsWith("/cancel") ? "/cancel" : route.endsWith("/events") ? "/events" : ""}`;
  const resource =
    /^\/api\/(slos|journeys|bindings|dependency-maps|connections|workflows|investigations)\/[^/]+(?:\/(messages|evidence|context|history|manifest|samples|collect|rules|rules\/plan|rules\/apply|runs|compile|health|view|test|approve|reject|apply))?$/.exec(
      route,
    );
  if (resource)
    return `/api/${resource[1]}/{id}${resource[2] ? `/${resource[2]}` : ""}`;
  return route.startsWith("/auth/") ? "/auth/*" : "/api/*";
}
export async function startBrowserTelemetry(): Promise<void> {
  const endpoint = import.meta.env.VITE_OTEL_EXPORTER_OTLP_ENDPOINT as
    string | undefined;
  if (!endpoint) return;
  try {
    const url = new URL(endpoint);
    if (
      !["http:", "https:"].includes(url.protocol) ||
      url.username ||
      url.password ||
      url.search ||
      url.hash
    )
      return;
    const sdk = await import("./otel-browser");
    instrumentedFetch = sdk.setupBrowserTelemetry(endpoint.replace(/\/$/, ""));
  } catch {
    /* An unavailable telemetry SDK must not block the application. */
  }
}
export function observedFetch(
  path: string,
  init: RequestInit,
): Promise<Response> {
  return instrumentedFetch ? instrumentedFetch(path, init) : fetch(path, init);
}
