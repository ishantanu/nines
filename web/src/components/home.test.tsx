// @vitest-environment jsdom
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { Home } from "./home";
import type { Data, Objective } from "@/lib/api";

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => vi.unstubAllGlobals());

const archived: Objective = {
  slo: {
    id: "retired",
    name: "Retired checkout",
    service: "checkout",
    project: "p",
    target: 0.999,
    windowSeconds: 86400,
    stepSeconds: 60,
    retirement: {
      archivedAt: "2026-10-27T00:00:00Z",
      cleanup: "complete",
      attempts: 1,
      message: "",
      audit: [],
    },
  },
  status: {
    sloId: "retired",
    at: "",
    from: "",
    good: 0,
    total: 100,
    bad: 100,
    coverage: 1,
    reliability: 0,
    budgetRemainingFraction: -1,
    burnRate: 1000,
    dataStatus: "complete",
    alert: { phase: "firing" },
  },
};
const data: Data = {
  objectives: [archived],
  journeys: [],
  events: [],
  workspace: {
    mode: "demo",
    prometheusConfigured: false,
    journeysConfigured: true,
    tenant: { id: "default", name: "Demo" },
    source: { kind: "manual", configured: false },
    role: "owner",
    workflowsConfigured: true,
  },
};
const render = (value: Data) =>
  renderToStaticMarkup(
    <Home
      data={value}
      onNavigate={() => {}}
      onSLO={() => {}}
      onJourney={() => {}}
    />,
  );
test("archived failures do not appear as active work on the overview", () => {
  const html = render(data);
  expect(html).not.toContain("Retired checkout");
  expect(html).toContain(
    "Define an SLO to start monitoring service reliability.",
  );
  expect(html).toContain("Open objectives");
});
test("unknown journey evidence is not described as confirmed service impact", () => {
  const html = render({
    ...data,
    journeys: [
      {
        health: "unknown",
        links: [],
        reasons: [],
        resource: {
          revision: 1,
          digest: "x",
          appliedAt: "",
          manifest: {
            apiVersion: "nines.dev/v1alpha1",
            kind: "CriticalUserJourney",
            metadata: { name: "checkout", project: "p" },
            spec: {
              displayName: "Checkout",
              description: "",
              owner: "platform",
              criticality: "high",
              slos: [],
              steps: [],
            },
          },
        },
      },
    ],
  });
  expect(html).toContain(
    "incomplete evidence. Missing data does not confirm service",
  );
  expect(html).not.toContain("Affected journeys");
});

test("review filter prioritises firing alerts, preserves missing evidence and opens the selected SLO", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("main");
  document.body.append(host);
  const root = createRoot(host);
  const selected = vi.fn();
  const objective = (
    id: string,
    status: Partial<Objective["status"]>,
  ): Objective => ({
    slo: { ...archived.slo, id, name: id, retirement: undefined },
    status: {
      ...archived.status,
      sloId: id,
      budgetRemainingFraction: 0.8,
      reliability: 0.9998,
      burnRate: 0.2,
      coverage: 1,
      alert: { phase: "inactive" },
      ...status,
    },
  });
  const healthy = objective("Healthy API", {});
  const missing = objective("Missing evidence", {
    reliability: null,
    coverage: 0,
    budgetRemainingFraction: null,
  });
  const firing = objective("Firing API", { alert: { phase: "firing" } });
  try {
    await act(async () =>
      root.render(
        <Home
          data={{ ...data, objectives: [healthy, missing, firing, archived] }}
          onNavigate={() => {}}
          onSLO={selected}
          onJourney={() => {}}
        />,
      ),
    );
    expect(host.querySelector("h2")?.textContent).toBe(
      "2 objectives need review",
    );
    const rows = () => [
      ...host.querySelectorAll<HTMLButtonElement>(
        '[data-slot="slo-metric-card"] h3 button',
      ),
    ];
    expect(rows()[0].textContent).toContain("Firing API");
    const filter = [
      ...host.querySelectorAll<HTMLButtonElement>("[aria-pressed]"),
    ].find((b) => b.textContent?.startsWith("Needs review"))!;
    await act(async () => filter.click());
    expect(filter.getAttribute("aria-pressed")).toBe("true");
    expect(rows()).toHaveLength(2);
    expect(host.textContent).not.toContain("Healthy API");
    expect(host.textContent).toContain("No SLI data");
    await act(async () => rows()[0].click());
    expect(selected).toHaveBeenCalledWith("Firing API");
    const { default: axe } = await import("axe-core");
    const results = await axe.run(host, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"],
      },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(results.violations).toEqual([]);
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});

test("Home bounds compact cards to six and opens the full inventory", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("main");
  document.body.append(host);
  const root = createRoot(host);
  const navigate = vi.fn();
  const items = Array.from({ length: 8 }, (_, i) => ({
    slo: {
      ...archived.slo,
      id: `slo-${i}`,
      name: `Objective ${i}`,
      retirement: undefined,
    },
    status: {
      ...archived.status,
      sloId: `slo-${i}`,
      reliability: 0.9999,
      budgetRemainingFraction: (8 - i) / 10,
      burnRate: 0.1,
      alert: { phase: "inactive" as const },
    },
  }));
  try {
    await act(async () =>
      root.render(
        <Home
          data={{ ...data, objectives: [archived, ...items] }}
          onNavigate={navigate}
          onSLO={() => {}}
          onJourney={() => {}}
        />,
      ),
    );
    const cards = host.querySelectorAll('[data-slot="slo-metric-card"]');
    expect(cards).toHaveLength(6);
    expect(cards[0].textContent).toContain("Objective 7");
    expect(host.textContent).not.toContain("Retired checkout");
    expect(host.textContent).toContain("Showing 6 of 8 objectives");
    const all = [...host.querySelectorAll("button")].find(
      (b) => b.textContent?.trim() === "View all SLOs",
    )!;
    await act(async () => all.click());
    expect(navigate).toHaveBeenCalledWith("objectives");
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
