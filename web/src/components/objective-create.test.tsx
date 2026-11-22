// @vitest-environment jsdom
import { typeInto } from "@/test-support/fields";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { parse, stringify } from "yaml";
import { ObjectiveCreate } from "./objective-create";
import { createTenantSession, type SLO } from "@/lib/api";
import { activeTab, tabLabel, tabs } from "@/test-support/tabs";
import { definitionFromSLO } from "@/lib/definitions";
let root: Root, host: HTMLDivElement;
const saved = vi.fn();
let calls: { path: string; init?: RequestInit }[] = [];
const slo: SLO = {
  id: "api",
  project: "production",
  name: "API availability",
  service: "api",
  source: "primary",
  target: 0.999,
  windowSeconds: 86400,
  stepSeconds: 60,
  measurement: {
    type: "availability",
    metric: "requests_total",
    labels: {},
    successLabels: { status: "200" },
    scrapeIntervalSeconds: 15,
    observationPoint: "Server observes every completed request.",
  },
};
beforeEach(() => {
  calls = [];
  saved.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      expect(new Headers(init?.headers).get("X-Nines-Tenant")).toBe("alpha");
      if (path === "/api/connections")
        return new Response(
          JSON.stringify({
            connections: [{ id: "primary", kind: "prometheus" }],
          }),
        );
      if (path === "/api/slo-templates")
        return new Response(
          JSON.stringify({
            apiVersion: "nines.dev/v1alpha1",
            tenant: "alpha",
            categories: [],
            templates: [],
          }),
        );
      if (path.includes("/revisions?"))
        return new Response(
          JSON.stringify({
            sloId: "api",
            revisions: [],
            nextBeforeRevision: 0,
          }),
        );
      if (path.endsWith("/compile"))
        return new Response(
          JSON.stringify({
            manifest: parse(String(init?.body)),
            evidence: {
              from: "2026-10-26T00:00:00Z",
              to: "2026-10-26T01:00:00Z",
            },
            warnings: [],
          }),
        );
      if (path.endsWith("/plan"))
        return new Response(
          JSON.stringify({
            manifest: parse(String(init?.body)),
            expectedRevision: 5,
            measurementChanged: true,
            action: "update",
            digest: "reviewed",
          }),
        );
      return new Response(JSON.stringify({ ok: true }));
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
function button(text: string) {
  return [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => b.textContent?.trim() === text,
  )!;
}
async function click(text: string) {
  await act(async () => button(text).click());
}
async function tab(name: string) {
  const b = tabs().find((b) => tabLabel(b) === name)!;
  await act(async () => b.click());
}
function input(label: string) {
  const l = [...document.querySelectorAll("label")].find(
    (l) => l.textContent?.trim() === label,
  )!;
  return document.getElementById(l.htmlFor) as
    HTMLInputElement | HTMLTextAreaElement;
}
async function field(label: string, value: string) {
  const e = input(label);
  await typeInto(e, value);
}
async function render(resource: SLO | null = slo) {
  await act(async () =>
    root.render(
      <ObjectiveCreate
        scope={createTenantSession("fixture", "alpha", "owner")}
        resource={resource ?? undefined}
        onCreated={saved}
      />,
    ),
  );
  await click(resource ? "Edit definition" : "Define objective");
}
test("guided edits reach YAML and settings edits survive returning to measurement", async () => {
  await render();
  await field("Display name", "Checkout availability");
  await field("Metric name", "checkout_requests_total");
  await field("Target (%)", "99.5");
  await field("Rolling window", "7d");
  await tab("YAML");
  let d = parse(input("ServiceLevelObjective manifest (YAML)").value);
  expect(d).toMatchObject({
    metadata: { name: "api" },
    spec: {
      displayName: "Checkout availability",
      target: 0.995,
      window: "7d",
      measurement: { metric: "checkout_requests_total" },
    },
  });
  await tab("Settings");
  await field("SLO target (fraction)", "0.98");
  await tab("Measurement");
  expect(input("Target (%)").value).toBe("98");
  expect(input("Metric name").value).toBe("checkout_requests_total");
  expect(input("Rolling window").value).toBe("7d");
  expect(calls.some((c) => c.init?.method === "POST")).toBe(false);
});
test("YAML can change measurement type and rehydrates the matching guided fields", async () => {
  await render();
  await tab("YAML");
  const d = definitionFromSLO(slo);
  d.spec.measurement = {
    type: "latency",
    metric: "duration_seconds",
    labels: { service: "api" },
    thresholdSeconds: 0.3,
    scrapeIntervalSeconds: 10,
    observationPoint: "Completed response duration.",
  };
  d.spec.target = 0.99;
  await field("ServiceLevelObjective manifest (YAML)", stringify(d));
  await tab("Measurement");
  expect(input("Histogram metric family (seconds)").value).toBe(
    "duration_seconds",
  );
  expect(input("Good events complete within").value).toBe("300ms");
  expect(input("Target (%)").value).toBe("99");
  await tab("YAML");
  // The YAML tab shows v1beta1: durations are strings.
  expect(
    parse(input("ServiceLevelObjective manifest (YAML)").value).spec
      .measurement,
  ).toMatchObject({
    type: "latency",
    metric: "duration_seconds",
    labels: { service: "api" },
    threshold: "300ms",
    scrapeInterval: "10s",
    observationPoint: "Completed response duration.",
  });
});
test("invalid YAML stays in YAML and custom queries cannot silently be replaced by the form", async () => {
  await render();
  await tab("YAML");
  await field("ServiceLevelObjective manifest (YAML)", "null");
  await tab("Measurement");
  expect(tabLabel(activeTab())).toBe("YAML");
  expect(document.querySelector('[role="alert"]')).not.toBeNull();
  const d = definitionFromSLO(slo);
  delete d.spec.measurement;
  d.spec.goodQuery = "vector(99)";
  d.spec.totalQuery = "vector(100)";
  await field("ServiceLevelObjective manifest (YAML)", stringify(d));
  await tab("Measurement");
  expect(tabLabel(activeTab())).toBe("YAML");
  await tab("Settings");
  expect(input("Good interval-count query").value).toBe("vector(99)");
  expect(
    tabs()
      .find((b) => tabLabel(b) === "Measurement")!
      .getAttribute("aria-disabled") === "true",
  ).toBe(true);
});
test("the guided save applies the exact confirmed plan with its revision precondition", async () => {
  await render();
  await field("Metric name", "reviewed_requests_total");
  await click("Verify telemetry and review SLO");
  expect(button("Save reviewed SLO").getAttribute("aria-disabled")).toBe(
    "true",
  );
  const label = [...document.querySelectorAll("label")].find((l) =>
    l.textContent?.includes("I reviewed the measurement, target"),
  )!;
  await act(async () =>
    label.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click(),
  );
  await click("Save reviewed SLO");
  const planned = calls.find((c) => c.path.endsWith("/plan"))!,
    applied = calls.find((c) => c.path.endsWith("/apply"))!;
  expect(new Headers(applied.init?.headers).get("If-Match")).toBe('"5"');
  expect(parse(String(applied.init?.body))).toEqual(
    parse(String(planned.init?.body)),
  );
  expect(saved).toHaveBeenCalledOnce();
});
test("changing tabs or the definition invalidates previous review and confirmation", async () => {
  await render();
  await click("Verify telemetry and review SLO");
  await field("Display name", "New name");
  expect(button("Save reviewed SLO").getAttribute("aria-disabled")).toBe(
    "true",
  );
  await click("Verify telemetry and review SLO");
  await tab("YAML");
  expect(button("Apply definition").getAttribute("aria-disabled")).toBe("true");
  await tab("Measurement");
  expect(button("Save reviewed SLO").getAttribute("aria-disabled")).toBe(
    "true",
  );
});
test("an incomplete new definition survives a YAML visit without acquiring example metrics", async () => {
  await render(null);
  await field("Display name", "New checkout objective");
  await tab("YAML");
  const d = parse(input("ServiceLevelObjective manifest (YAML)").value);
  expect(d.spec.displayName).toBe("New checkout objective");
  // The identifier follows the display name until it is edited.
  expect(d.metadata.name).toBe("new-checkout-objective");
  expect(d.spec.measurement.metric).toBe("");
  await tab("Measurement");
  expect(input("Display name").value).toBe("New checkout objective");
  expect(input("Metric name").value).toBe("");
});

test("YAML cannot rename an existing SLO into a different resource through the edit dialog", async () => {
  await render();
  await tab("YAML");
  const d = definitionFromSLO(slo);
  d.metadata.name = "different-objective";
  await field("ServiceLevelObjective manifest (YAML)", stringify(d));
  await click("Validate and plan");
  expect(
    [...document.querySelectorAll('[role="alert"]')].some((e) =>
      e.textContent?.includes("existing SLO identifier"),
    ),
  ).toBe(true);
  expect(calls.some((c) => c.init?.method === "POST")).toBe(false);
  expect(button("Apply definition").getAttribute("aria-disabled")).toBe("true");
});

test("the common path asks only for name, service, metric and filters, with the rest tucked away", async () => {
  await render(null);
  const dialog = document.querySelector('[role="dialog"]')!;
  const advanced = [...dialog.querySelectorAll("details")].find((d) =>
    d.textContent?.includes("Advanced: identifier, project and intervals"),
  )!;
  expect(advanced.open).toBe(false);
  const optional = [...dialog.querySelectorAll("details")].find((d) =>
    d.textContent?.includes("Optional settings"),
  )!;
  expect(optional.open).toBe(false);
  // The identifier is derived, and editing the display name keeps it in step.
  await field("Display name", "Checkout availability");
  await tab("YAML");
  expect(
    parse(input("ServiceLevelObjective manifest (YAML)").value).metadata.name,
  ).toBe("checkout-availability");
  await tab("Measurement");
  // One compact choice, not tiles, named like the badges and docs.
  // Per-customer reports and journey objectives are not offered here as
  // alternatives to the measurement.
  const text = dialog.textContent ?? "";
  expect(text).toContain("SLO type");
  expect(text).not.toContain("Other kinds of SLO");
  expect(text).not.toContain("Journey run objective");
  expect(dialog.querySelectorAll("[data-slo-kind]").length).toBe(0);
  const open = async () =>
    act(async () =>
      (document.querySelector("#slo-measure") as HTMLElement).click(),
    );
  const options = () =>
    [...document.querySelectorAll('[role="option"]')].map((o) =>
      o.textContent?.trim(),
    );
  await open();
  expect(options()).toEqual(
    expect.arrayContaining([
      "Availability",
      "Latency",
      "Sessions",
      "Good-minute",
    ]),
  );
  await act(async () => {
    [...document.querySelectorAll('[role="option"]')]
      .find((o) => o.textContent?.trim() === "Sessions")!
      .dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await tab("YAML");
  expect(
    parse(input("ServiceLevelObjective manifest (YAML)").value).spec.measurement
      .type,
  ).toBe("sessions");
  await tab("Measurement");
  await open();
  await act(async () => {
    [...document.querySelectorAll('[role="option"]')]
      .find((o) => o.textContent?.includes("Good-minute"))!
      .dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  // Good minutes are judged from interval-count queries, so it opens Settings.
  expect(document.querySelector('[role="dialog"]')!.textContent).toContain(
    "Judge intervals instead of events",
  );
});

test("every SLI kind from the book is offered with its own wording", async () => {
  await render(null);
  const dialog = () => document.querySelector('[role="dialog"]')!;
  const choose = async (name: string) => {
    await act(async () =>
      (document.querySelector("#slo-measure") as HTMLElement).click(),
    );
    await act(async () => {
      [...document.querySelectorAll('[role="option"]')]
        .find((o) => o.textContent?.trim() === name)!
        .dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
  };
  await act(async () =>
    (document.querySelector("#slo-measure") as HTMLElement).click(),
  );
  const offered = [...document.querySelectorAll('[role="option"]')].map((o) =>
    o.textContent?.trim(),
  );
  for (const name of [
    "Availability",
    "Latency",
    "Correctness",
    "Durability",
    "Freshness",
    "Coverage",
    "Sessions",
    "Good-minute",
    "Throughput",
    "Per-customer",
  ])
    expect(offered).toContain(name);
  // Freshness judges a data-age histogram at a threshold. The list is open.
  await act(async () => {
    [...document.querySelectorAll('[role="option"]')]
      .find((o) => o.textContent?.trim() === "Freshness")!
      .dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  expect(dialog().textContent).toContain("Data age histogram (seconds)");
  expect(dialog().textContent).toContain(
    "Data is fresh when its age is at most",
  );
  // Correctness counts results that passed validation.
  await choose("Correctness");
  expect(dialog().textContent).toContain("Correct results");
  // Per-customer adds the report to the current type and opens its settings.
  await choose("Per-customer");
  const optional = [...dialog().querySelectorAll("details")].find((d) =>
    d.textContent?.includes("Optional settings"),
  )!;
  expect(optional.open).toBe(true);
  expect(dialog().textContent).toContain("Customer label on the metric");
  expect(dialog().textContent).toContain("Per-customer reporting is on");
  // Throughput is judged from interval-count queries in Settings.
  await choose("Throughput");
  expect(dialog().textContent).toContain(
    "Interval is good with at least this many good events",
  );
});
