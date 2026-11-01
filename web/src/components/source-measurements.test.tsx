// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { SourceMeasurements } from "./source-measurements";
import { createTenantSession, type SLO } from "@/lib/api";
let root: Root, host: HTMLDivElement;
let role: "viewer" | "editor" = "editor";
let providerKind = "prometheus";
let count = 3,
  foreign = false;
const changed = vi.fn();
const slo: SLO = {
  id: "api",
  name: "API availability",
  project: "production",
  service: "api",
  source: "requests",
  target: 0.999,
  windowSeconds: 86400,
  stepSeconds: 60,
  providerWindow: {
    goodSelector: 'http_requests_total{code="200"}',
    totalSelector: "http_requests_total",
    scrapeIntervalSeconds: 15,
  },
};
beforeEach(() => {
  role = "editor";
  providerKind = "prometheus";
  count = 3;
  foreign = false;
  changed.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      expect(new Headers(init?.headers).get("X-Nines-Tenant")).toBe("alpha");
      const data =
        path === "/api/connections"
          ? { connections: [{ id: "requests", kind: providerKind }] }
          : path.endsWith("/collect")
            ? { collected: count }
            : path.includes("/metrics?")
              ? {
                  tenant: foreign ? "other" : "alpha",
                  source: "requests",
                  names: ["playground_requests_total"],
                  truncated: false,
                }
              : {
                  tenant: "alpha",
                  source: "requests",
                  metric: "playground_requests_total",
                  series: [
                    { __name__: "playground_requests_total", code: "200" },
                  ],
                  truncated: false,
                };
      return new Response(JSON.stringify(data), { status: 200 });
    }),
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
async function until(check: () => boolean) {
  for (let i = 0; i < 50; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (check()) return;
  }
  throw Error("UI did not settle: " + host.textContent);
}
function button(text: string) {
  return [...host.querySelectorAll("button")].find(
    (b) => b.textContent === text,
  )!;
}
async function render() {
  await act(async () =>
    root.render(
      <SourceMeasurements
        scope={createTenantSession("credential", "alpha", role)}
        slo={slo}
        collection={{ state: "succeeded", points: 0, sloRevision: 1 }}
        onCollected={changed}
        onConnections={() => {}}
      />,
    ),
  );
  await until(() =>
    host.textContent!.includes(
      providerKind === "newrelic" ? "New Relic" : "Prometheus",
    ),
  );
}
test("reads recent source measurements with no caller URL or query overrides", async () => {
  await render();
  expect(host.textContent).toContain(
    "latest source read returned no matching samples",
  );
  await act(async () => button("Read latest measurements").click());
  await until(() => changed.mock.calls.length === 1);
  const call = vi
    .mocked(fetch)
    .mock.calls.find(([path]) => String(path).endsWith("/collect"))!;
  expect(call[0]).toBe("/api/slos/api/collect");
  expect(JSON.parse(String(call[1]?.body))).toEqual({ recent: true });
  expect(host.textContent).toContain("Read 3 measurements from requests");
});
test("successful empty queries explain the missing metric rather than claim collection supplied SLI data", async () => {
  count = 0;
  await render();
  await act(async () => button("Read latest measurements").click());
  await until(() => changed.mock.calls.length === 1);
  expect(host.textContent).toContain(
    "saved measurement queries returned no matching samples",
  );
  expect(host.textContent).not.toContain("Read 0 measurements");
});
test("viewer browses actual metric names and labels without changing the SLO", async () => {
  role = "viewer";
  await render();
  expect(button("Read latest measurements").disabled).toBe(true);
  await act(async () => button("Browse metrics").click());
  await until(() => !!button("playground_requests_total"));
  await act(async () => button("playground_requests_total").click());
  await until(() => !!host.querySelector('[aria-label="Observed series"]'));
  const series = host.querySelector('[aria-label="Observed series"]')!;
  expect(series.querySelector("pre")).toBeNull();
  const code = [...series.querySelectorAll("dt")].find(
    (term) => term.textContent === "code",
  )!;
  expect(code.nextElementSibling?.textContent).toBe("200");
  const labels = series.closest("details")!;
  expect(labels.open).toBe(false);
  await act(async () => labels.querySelector("summary")!.click());
  expect(labels.open).toBe(true);
  expect(
    vi
      .mocked(fetch)
      .mock.calls.every(([, init]) => !init?.method || init.method === "GET"),
  ).toBe(true);
  expect(host.textContent).toContain("http_requests_total");
  expect(changed).not.toHaveBeenCalled();
});
test("source browsing refuses evidence for a different tenant", async () => {
  foreign = true;
  await render();
  await act(async () => button("Browse metrics").click());
  await until(() => !!host.querySelector('[role="alert"]'));
  expect(host.textContent).toContain("Metric source scope mismatch");
  expect(button("playground_requests_total")).toBeUndefined();
});

test("native count providers retain collection without offering unsupported metric browsing", async () => {
  providerKind = "newrelic";
  await render();
  expect(host.textContent).toContain("New Relic");
  expect(host.textContent).toContain("Metric browsing is unavailable");
  expect(button("Browse metrics")).toBeUndefined();
  expect(button("Read latest measurements").disabled).toBe(false);
  expect(host.textContent).not.toContain("every 15 seconds");
});

test("empty New Relic reads suggest checking time and fresh telemetry", async () => {
  providerKind = "newrelic";
  count = 0;
  await render();
  await act(async () => button("Read latest measurements").click());
  await until(() => changed.mock.calls.length === 1);
  expect(host.textContent).toContain("whether fresh telemetry is arriving");
  expect(host.textContent).not.toContain("Browse its metrics");
});
