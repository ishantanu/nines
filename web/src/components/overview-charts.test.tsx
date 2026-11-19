// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { createTenantSession, type Objective } from "@/lib/api";
import {
  overviewMeasurements,
  OverviewCharts,
  SLOTrend,
} from "./overview-charts";
import { Sparkline } from "./ui/notchset/sparkline";
import { shares } from "./ui/notchset/donut-chart";

function objective(
  id: string,
  service: string,
  burn: number | null,
  coverage = 1,
): Objective {
  return {
    slo: {
      id,
      name: id,
      service,
      project: "p",
      target: 0.99,
      windowSeconds: 60,
      stepSeconds: 10,
      revision: 1,
    },
    status: {
      sloId: id,
      at: "2026-11-02T12:00:00Z",
      from: "2026-11-02T11:59:00Z",
      good: 99,
      total: 100,
      bad: 1,
      reliability: 0.99,
      burnRate: burn,
      budgetRemainingFraction: 0,
      coverage,
      dataStatus: coverage === 1 ? "complete" : "partial",
      alert: { phase: "inactive" },
    },
  };
}
let resizeBar: (
  entries: { contentRect: { width: number; height: number } }[],
) => void;
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: typeof resizeBar) {
        resizeBar = callback;
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => vi.unstubAllGlobals());
test("coverage counts retain unknowns and service burn uses a maximum, never an average", () => {
  const rows = [
    objective("a", "api", 2),
    objective("b", "api", 6),
    objective("c", "worker", 12, 0.5),
    objective("d", "db", null, 0),
    objective("e", "invalid", NaN),
  ];
  const out = overviewMeasurements(rows);
  expect(out.services).toEqual([{ label: "api", value: 6 }]);
  expect(out.coverage.map((c) => c.value)).toEqual([3, 1, 1]);
  expect(out.total).toBe(5);
  expect(shares([0, 0, 0])).toEqual({ raw: [0, 0, 0], pct: [0, 0, 0] });
  expect(shares([1, 1, 1]).pct.reduce((a, b) => a + b, 0)).toBe(100);
  const archived = {
    ...rows[0],
    slo: {
      ...rows[0].slo,
      retirement: {
        archivedAt: "2026-11-02T12:00:00Z",
        cleanup: "complete" as const,
        attempts: 1,
        message: "",
        audit: [],
      },
    },
  };
  expect(overviewMeasurements([archived]).total).toBe(0);
});
test("sparkline keeps timestamp gaps and does not interpolate missing observations", () => {
  const html = renderToStaticMarkup(
    <Sparkline
      label="Test SLI"
      points={[
        { at: "2026-11-02T11:00:00Z", value: 0.9 },
        { at: "2026-11-02T11:01:00Z", value: null },
        { at: "2026-11-02T12:00:00Z", value: 1 },
      ]}
    />,
  );
  expect(html).toContain("2 observations");
  expect(html).not.toMatch(/d="M[^\"]*L/);
  expect(html).not.toMatch(/NaN|Infinity/);
  expect(
    renderToStaticMarkup(<Sparkline label="Empty" points={[]} />),
  ).toContain("No trend data");
});
test("overview charts retain keyboard access and accessible data without cross-SLO scores", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  try {
    await act(async () =>
      root.render(
        <OverviewCharts
          objectives={[
            objective("a", "api", 6),
            objective("b", "worker", 0, 0.5),
          ]}
        />,
      ),
    );
    expect(host.textContent).toContain("Not an availability score");
    const bar = host.querySelector<SVGElement>(
      'svg[role="group"] [role="button"]',
    )!;
    await act(async () =>
      bar.dispatchEvent(new FocusEvent("focusin", { bubbles: true })),
    );
    expect(host.textContent).toContain("api: 6.00×");
    expect(
      host
        .querySelector('svg[role="group"] rect[stroke]')
        ?.getAttribute("stroke"),
    ).toBe("var(--chart-sli)");
    expect(host.querySelector('[role="tooltip"]')?.textContent).toContain(
      "6.00×",
    );
    const plot = host.querySelector<SVGElement>('svg[role="group"]')!;
    for (const width of [240, 800]) {
      await act(async () =>
        resizeBar([{ contentRect: { width, height: 260 } }]),
      );
      expect(plot.getAttribute("viewBox")).toBe(`0 0 ${width} 260`);
      expect(
        [...plot.querySelectorAll("text")].every(
          (text) =>
            text.getAttribute("style")?.includes("--font-size-sm") === true,
        ),
      ).toBe(true);
      expect(plot.innerHTML).not.toMatch(/NaN|Infinity/);
    }
    await act(async () =>
      bar.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
      ),
    );
    expect(host.querySelector('[role="tooltip"]')).toBeNull();
    expect(host.querySelector("table caption")?.textContent).toContain("burn");
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
  }
});
test("trend requests are local tenant-scoped history reads and canceled results cannot cross tenants", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  let resolveA!: (response: Response) => void;
  const pendingA = new Promise<Response>((resolve) => {
    resolveA = resolve;
  });
  const response = (revision: number, value: number) =>
    new Response(
      JSON.stringify({
        sloId: "a",
        configRevision: revision,
        points: [
          { at: "2026-11-02T11:59:00Z", reliability: value, coverage: 1 },
          { at: "2026-11-02T12:00:00Z", reliability: value, coverage: 1 },
        ],
      }),
    );
  const fetcher = vi
    .fn()
    .mockReturnValueOnce(pendingA)
    .mockResolvedValueOnce(response(2, 0.8));
  vi.stubGlobal("fetch", fetcher);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const a = createTenantSession("local-test", "alpha", "owner"),
    b = createTenantSession("local-test", "beta", "owner");
  try {
    const row = objective("a", "api", 1);
    await act(async () => root.render(<SLOTrend objective={row} scope={a} />));
    expect(String(fetcher.mock.calls[0][0])).toMatch(
      /^\/api\/slos\/a\/history\?/,
    );
    expect(
      new Headers(fetcher.mock.calls[0][1].headers).get("X-Nines-Tenant"),
    ).toBe("alpha");
    await act(async () =>
      root.render(
        <SLOTrend
          objective={{ ...row, slo: { ...row.slo, revision: 2 } }}
          scope={b}
        />,
      ),
    );
    expect(
      new Headers(fetcher.mock.calls[1][1].headers).get("X-Nines-Tenant"),
    ).toBe("beta");
    expect(host.querySelector("svg")?.getAttribute("aria-label")).toContain(
      "80.000%",
    );
    await act(async () => resolveA(response(1, 0.99)));
    expect(host.querySelector("svg")?.getAttribute("aria-label")).toContain(
      "80.000%",
    );
  } finally {
    await act(async () => root.unmount());
    a.close();
    b.close();
    host.remove();
  }
});

test("compact SLI includes the target and preserves the requested time extent", () => {
  const html = renderToStaticMarkup(
    <Sparkline
      label="API SLI"
      target={0.999}
      from="2026-11-03T10:00:00Z"
      to="2026-11-03T10:15:00Z"
      points={[
        { at: "2026-11-03T10:14:00Z", value: 1 },
        { at: "2026-11-03T10:15:00Z", value: 1 },
      ]}
    />,
  );
  const doc = new DOMParser().parseFromString(html, "text/html");
  expect(doc.querySelector('[data-slot="sparkline-target"]')).not.toBeNull();
  expect(doc.querySelector("svg")?.getAttribute("aria-label")).toContain(
    "SLO target 99.900%",
  );
  // Observations cover the final minute, rather than stretching to fill 15m.
  const segment = doc.querySelector("path")?.getAttribute("d") ?? "";
  expect(Number(segment.match(/M([0-9.]+)/)?.[1])).toBeGreaterThan(130);
});

test("card trend range changes stay bounded and cancel the previous history read", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const fetcher = vi
    .fn()
    .mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({ sloId: "a", configRevision: 1, points: [] }),
        ),
      ),
    );
  vi.stubGlobal("fetch", fetcher);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const scope = createTenantSession("local-test", "alpha", "owner");
  try {
    const row = objective("a", "api", 1);
    await act(async () =>
      root.render(
        <SLOTrend objective={row} scope={scope} rangeSeconds={3600} />,
      ),
    );
    const first = fetcher.mock.calls[0];
    const q = new URL(String(first[0]), "http://localhost").searchParams;
    expect(q.get("rangeSeconds")).toBe("3600");
    expect(Number(q.get("resolutionSeconds"))).toBeGreaterThanOrEqual(60);
    await act(async () =>
      root.render(
        <SLOTrend objective={row} scope={scope} rangeSeconds={86400} />,
      ),
    );
    expect(first[1].signal.aborted).toBe(true);
    expect(String(fetcher.mock.calls[1][0])).toContain("rangeSeconds=86400");
  } finally {
    await act(async () => root.unmount());
    host.remove();
    scope.close();
  }
});
