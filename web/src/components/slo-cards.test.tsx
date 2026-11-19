// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, expect, test, vi } from "vitest";
import type { Objective } from "@/lib/api";
import { SLOCards } from "./slo-cards";
import { Objectives } from "./objectives";

function objective(id: string, reliability: number | null = 0.995): Objective {
  return {
    slo: {
      id,
      name: id,
      project: "production",
      service: "checkout",
      target: 0.999,
      stepSeconds: 60,
      windowSeconds: 86400,
      revision: 2,
    },
    status: {
      sloId: id,
      at: "2026-11-03T12:00:00Z",
      from: "2026-11-02T12:00:00Z",
      good: 995,
      total: 1000,
      bad: 5,
      reliability,
      coverage: 0.5,
      dataStatus: reliability === null ? "no_data" : "partial",
      budgetRemainingFraction: -0.5,
      burnRate: 5,
      alert: { phase: "inactive" },
    },
  } as Objective;
}
afterEach(() => vi.unstubAllGlobals());
test("SLO cards preserve partial data, negative budgets and unavailable SLI without fabricated trends", () => {
  const html = renderToStaticMarkup(
    <SLOCards
      items={[objective("partial"), objective("missing", null)]}
      journeys={[]}
      rangeSeconds={3600}
      onSelect={() => {}}
      onJourney={() => {}}
    />,
  );
  expect(html).toContain("99.500% SLI");
  expect(html).toContain("99.900% target");
  expect(html).toContain("Partial window");
  expect(html).toContain("-50.0%");
  expect(html).toContain("No SLI data");
  expect(html).not.toContain("<svg");
  expect(html).not.toContain("NaN");
});
test("cards bound the displayed inventory and retain table navigation and lifecycle filters", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const select = vi.fn();
  const click = async (text: string) => {
    const el = [...host.querySelectorAll("button")].find(
      (b) => b.textContent === text,
    )!;
    expect(el).toBeTruthy();
    await act(async () => el.click());
  };
  try {
    await act(async () =>
      root.render(
        <Objectives
          items={Array.from({ length: 13 }, (_, i) => objective(`slo-${i}`))}
          journeys={[]}
          onSelect={select}
          onJourney={() => {}}
        />,
      ),
    );
    expect(host.querySelectorAll('[data-slot="slo-metric-card"]')).toHaveLength(
      12,
    );
    await click("Next");
    expect(host.querySelectorAll('[data-slot="slo-metric-card"]')).toHaveLength(
      1,
    );
    await click("slo-12");
    expect(select).toHaveBeenCalledWith("slo-12");
    await click("Table");
    expect(host.querySelectorAll("tbody tr")).toHaveLength(13);
    expect(host.querySelector('[data-slot="slo-metric-card"]')).toBeNull();
    await click("Archived objectives");
    expect(host.textContent).toContain("No objectives match this view.");
    await click("Cards");
    await click("Active objectives");
    const { default: axe } = await import("axe-core");
    const result = await axe.run(host, {
      rules: {
        "color-contrast": { enabled: false },
        region: { enabled: false },
      },
    });
    expect(result.violations).toEqual([]);
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
