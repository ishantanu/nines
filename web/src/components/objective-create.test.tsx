// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { parse, stringify } from "yaml";
import { ObjectiveCreate } from "./objective-create";
import { createTenantSession, type SLO } from "@/lib/api";
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
  const b = [
    ...document.querySelectorAll<HTMLButtonElement>('[role="tab"]'),
  ].find((b) => b.textContent === name)!;
  await act(async () =>
    b.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 })),
  );
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
  await act(async () => {
    const p =
      e.tagName === "TEXTAREA"
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(p, "value")!.set!.call(e, value);
    e.dispatchEvent(new Event("input", { bubbles: true }));
  });
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
      windowSeconds: 604800,
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
  expect(input("Good events complete within (seconds)").value).toBe("0.3");
  expect(input("Target (%)").value).toBe("99");
  await tab("YAML");
  expect(
    parse(input("ServiceLevelObjective manifest (YAML)").value).spec
      .measurement,
  ).toMatchObject(d.spec.measurement);
});
test("invalid YAML stays in YAML and custom queries cannot silently be replaced by the form", async () => {
  await render();
  await tab("YAML");
  await field("ServiceLevelObjective manifest (YAML)", "null");
  await tab("Measurement");
  expect(
    document.querySelector('[role="tab"][data-state="active"]')?.textContent,
  ).toBe("YAML");
  expect(document.querySelector('[role="alert"]')).not.toBeNull();
  const d = definitionFromSLO(slo);
  delete d.spec.measurement;
  d.spec.goodQuery = "vector(99)";
  d.spec.totalQuery = "vector(100)";
  await field("ServiceLevelObjective manifest (YAML)", stringify(d));
  await tab("Measurement");
  expect(
    document.querySelector('[role="tab"][data-state="active"]')?.textContent,
  ).toBe("YAML");
  await tab("Settings");
  expect(input("Good interval-count query").value).toBe("vector(99)");
  expect(
    [...document.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(
      (b) => b.textContent === "Measurement",
    )!.disabled,
  ).toBe(true);
});
test("the guided save applies the exact confirmed plan with its revision precondition", async () => {
  await render();
  await field("Metric name", "reviewed_requests_total");
  await click("Verify telemetry and review SLO");
  expect(button("Save reviewed SLO").disabled).toBe(true);
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
  expect(button("Save reviewed SLO").disabled).toBe(true);
  await click("Verify telemetry and review SLO");
  await tab("YAML");
  expect(button("Apply definition").disabled).toBe(true);
  await tab("Measurement");
  expect(button("Save reviewed SLO").disabled).toBe(true);
});
test("an incomplete new definition survives a YAML visit without acquiring example metrics", async () => {
  await render(null);
  await field("Display name", "New checkout objective");
  await tab("YAML");
  const d = parse(input("ServiceLevelObjective manifest (YAML)").value);
  expect(d.spec.displayName).toBe("New checkout objective");
  expect(d.metadata.name).toBe("");
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
  expect(button("Apply definition").disabled).toBe(true);
});
