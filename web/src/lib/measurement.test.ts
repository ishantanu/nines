import { afterEach, expect, test, vi } from "vitest";
import { parse } from "yaml";
import { createTenantSession } from "./api";
import { bindingStarter } from "./topology";
import {
  bindingFromDraft,
  filterLabels,
  measurementDraft,
  prepareMeasuredObjective,
  prepareInlineObjective,
  sloFromMeasurementDraft,
} from "./measurement";
import { definitionFromSLO } from "./definitions";

afterEach(() => vi.unstubAllGlobals());
test("structured measurements preserve populations, units, cadence and observation point", () => {
  const binding = bindingStarter("mimir", "http_duration_seconds", {
    environment: "production",
    service: "checkout",
  });
  binding.spec.measurement = "latency";
  delete binding.spec.successLabels;
  binding.spec.thresholdSeconds = 0.3;
  expect(bindingFromDraft(measurementDraft(binding))).toEqual(binding);
});
test("success labels are literal strings and cannot narrow the denominator silently", () => {
  const draft = measurementDraft();
  draft.population = [{ label: "environment", value: "production" }];
  draft.success = [{ label: "status", value: "200" }];
  expect(bindingFromDraft(draft).spec.successLabels).toEqual({ status: "200" });
  draft.success = [{ label: "environment", value: "production" }];
  expect(() => bindingFromDraft(draft)).toThrow(
    "without repeating or overriding",
  );
  draft.success = [];
  expect(() => bindingFromDraft(draft)).toThrow("at least one label");
});
test("duplicate and reserved population filters cannot disappear during serialization", () => {
  expect(() =>
    filterLabels([
      { label: "service", value: "a" },
      { label: "service", value: "b" },
    ]),
  ).toThrow("repeated");
  for (const label of ["__name__", "le", "environment=~", "__proto__"]) {
    expect(() => filterLabels([{ label, value: "ignored" }])).toThrow(
      "non-reserved",
    );
  }
});
test("invalid scrape cadence blocks compilation and changing outcome drops hidden success fields", () => {
  const d = measurementDraft();
  d.scrape = "40";
  expect(() => bindingFromDraft(d)).toThrow("evenly spaced scrapes");
  d.scrape = "15";
  d.measurement = "latency";
  d.threshold = ".3";
  d.success = [{ label: "invalid=label", value: "200" }];
  const compiled = bindingFromDraft(d);
  expect(compiled.spec.successLabels).toBeUndefined();
  expect(compiled.spec.countFrom).toBeUndefined();
  expect(compiled.spec.thresholdSeconds).toBe(0.3);
});
const scope = () => createTenantSession("fixture-token", "alpha", "owner");
const objective = {
  name: "checkout",
  displayName: "Checkout success",
  target: "99.9",
  window: "28d",
  policies: [],
};
const manifest = definitionFromSLO({
  id: "checkout",
  project: "production",
  service: "api",
  name: "Checkout",
  target: 0.999,
  windowSeconds: 2419200,
  stepSeconds: 60,
  providerWindow: {
    goodSelector: 'requests_total{status="200"}',
    totalSelector: "requests_total",
    scrapeIntervalSeconds: 15,
  },
  measurementBinding: {
    name: "requests",
    revision: 3,
    digest: "binding-digest",
  },
});
test("measurement preview checks telemetry then plans with the pinned revision and never applies", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({ manifest, binding: manifest.spec.measurementBinding }),
      ),
    )
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          action: "create",
          expectedRevision: 0,
          digest: "plan",
          manifest,
        }),
      ),
    );
  vi.stubGlobal("fetch", fetcher);
  const policies = [
    {
      name: "fast",
      severity: "critical" as const,
      shortWindowSeconds: 300,
      longWindowSeconds: 3600,
      burnRate: 14.4,
      forSeconds: 120,
      minCoverage: 1,
    },
  ];
  const result = await prepareMeasuredObjective(
    scope(),
    { name: "requests", revision: 3 },
    { ...objective, policies },
    new AbortController().signal,
  );
  expect(result.plan.manifest.spec.measurementBinding?.revision).toBe(3);
  expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
    "/api/bindings/requests/compile",
    "/api/resources/plan",
  ]);
  const compile = fetcher.mock.calls[0][1] as RequestInit;
  expect(JSON.parse(compile.body as string)).toMatchObject({
    expectedRevision: 3,
    target: 0.999,
    windowSeconds: 2419200,
    policies,
  });
  const plan = fetcher.mock.calls[1][1] as RequestInit;
  expect(parse(plan.body as string)).toEqual(manifest);
  for (const [, options] of fetcher.mock.calls) {
    expect(new Headers(options.headers).get("X-Nines-Tenant")).toBe("alpha");
  }
});
test("missing telemetry or a stale measurement prevents SLO planning", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValue(
      new Response(
        JSON.stringify({ error: { message: "measurement changed" } }),
        { status: 409 },
      ),
    );
  vi.stubGlobal("fetch", fetcher);
  await expect(
    prepareMeasuredObjective(
      scope(),
      { name: "requests", revision: 3 },
      objective,
      new AbortController().signal,
    ),
  ).rejects.toThrow("measurement changed");
  expect(fetcher).toHaveBeenCalledTimes(1);
});
test("switching tenant while verifying cannot start an SLO plan even if the provider resolves late", async () => {
  const session = scope();
  const fetcher = vi.fn().mockImplementation(() => {
    session.close();
    return Promise.resolve(new Response(JSON.stringify({ manifest })));
  });
  vi.stubGlobal("fetch", fetcher);
  await expect(
    prepareMeasuredObjective(
      session,
      { name: "requests", revision: 3 },
      objective,
      new AbortController().signal,
    ),
  ).rejects.toThrow();
  expect(fetcher).toHaveBeenCalledTimes(1);
});
test.each([
  { target: "100" },
  { target: "0" },
  { target: "" },
  { window: "32d" },
  { window: "0s" },
  { window: "last month" },
])("invalid objective %j never queries a provider", async (override) => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  await expect(
    prepareMeasuredObjective(
      scope(),
      { name: "requests", revision: 3 },
      { ...objective, ...override },
      new AbortController().signal,
    ),
  ).rejects.toThrow();
  expect(fetcher).not.toHaveBeenCalled();
});

test("inline preview verifies and plans a single SLO without writing a binding", async () => {
  const d = measurementDraft();
  const inline = sloFromMeasurementDraft(d, "99.9", "28d", []);
  expect(inline.kind).toBe("ServiceLevelObjective");
  expect(inline.spec.measurement?.metric).toBe(d.metric);
  expect(inline.spec.measurementBinding).toBeUndefined();
  expect(inline.spec.providerWindow).toBeUndefined();
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          manifest: inline,
          measurementDigest: "contract",
          warnings: [],
          evidence: {},
        }),
      ),
    )
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          action: "create",
          expectedRevision: 0,
          manifest: inline,
        }),
      ),
    );
  vi.stubGlobal("fetch", fetcher);
  const result = await prepareInlineObjective(
    scope(),
    inline,
    new AbortController().signal,
  );
  expect(result.plan.expectedRevision).toBe(0);
  expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
    "/api/slos/compile",
    "/api/resources/plan",
  ]);
  for (const [, request] of fetcher.mock.calls) {
    expect(parse(request.body).spec.measurement).toEqual(
      inline.spec.measurement,
    );
    expect(new Headers(request.headers).get("X-Nines-Tenant")).toBe("alpha");
  }
});
test("inline windows enforce the interval bound before submitting", () => {
  const d = measurementDraft();
  d.step = "15";
  d.scrape = "5";
  expect(() => sloFromMeasurementDraft(d, "99.9", "28d", [])).toThrow("100000");
  d.step = "60";
  expect(sloFromMeasurementDraft(d, "99.9", "28d", []).spec.stepSeconds).toBe(
    60,
  );
});
