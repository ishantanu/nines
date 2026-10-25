// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { Timeline, sliTimelineLines } from "./timeline";
import type { HistoryPoint, SLOHistory } from "@/lib/history";
let root: Root, host: HTMLDivElement;
let resize: (
  entries: { contentRect: { width: number; height: number } }[],
) => void;
const onIndex = vi.fn();
const point = (at: string, reliability: number | null): HistoryPoint => ({
  at,
  intervalReliability: reliability,
  reliability,
  budgetRemainingFraction: reliability,
  burnRate: null,
  shortBurnRate: null,
  longBurnRate: null,
  coverage: reliability === null ? 0 : 1,
  intervalCoverage: reliability === null ? 0 : 1,
  shortCoverage: 0,
  longCoverage: 0,
  belowTarget: reliability !== null && reliability < 0.999,
  targetViolated: false,
  burnThresholdBreached: false,
});
const history: SLOHistory = {
  source: "primary",
  sloId: "api",
  configRevision: 1,
  target: 0.999,
  windowSeconds: 3600,
  stepSeconds: 60,
  resolutionSeconds: 60,
  from: "2026-10-24T10:00:00Z",
  to: "2026-10-24T11:00:00Z",
  points: [
    point("2026-10-24T10:00:00Z", 0.99),
    point("2026-10-24T10:01:00Z", null),
    point("2026-10-24T11:00:00Z", 1),
  ],
  violations: [],
  alertEvents: [],
  alertEventsBounded: false,
};
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: typeof resize) {
        resize = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  onIndex.mockClear();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
async function render(data = history) {
  await act(async () =>
    root.render(
      <Timeline
        title="SLI and SLO target"
        history={data}
        lines={sliTimelineLines}
        threshold={data.target}
        thresholdLabel="Target 99.9%"
        percentages
        index={0}
        onIndex={onIndex}
      />,
    ),
  );
}
test("chart tracks both dimensions without a minimum-width overflow or scaled labels", async () => {
  await render();
  for (const [width, height] of [
    [800, 260],
    [240, 180],
    [1100, 320],
  ]) {
    await act(async () => resize([{ contentRect: { width, height } }]));
    expect(host.querySelector("svg")?.getAttribute("viewBox")).toBe(
      `0 0 ${width} ${height}`,
    );
  }
  expect(host.querySelectorAll("svg text").length).toBeGreaterThan(6);
});
test("missing data splits paths and single observed points remain visible", async () => {
  await render();
  const series = host.querySelector('[data-series="Interval SLI"]')!;
  expect(
    series.querySelector("path")?.getAttribute("d")?.match(/M/g),
  ).toHaveLength(2);
  expect(series.querySelector("path")?.getAttribute("d")).not.toContain("L");
  expect(series.querySelectorAll("circle")).toHaveLength(2);
  expect(host.innerHTML).not.toMatch(/NaN|Infinity/);
});
test("keyboard inspection supports endpoints and never fabricates a selection for empty data", async () => {
  await render();
  await act(async () =>
    host
      .querySelector("svg")!
      .dispatchEvent(
        new KeyboardEvent("keydown", { key: "End", bubbles: true }),
      ),
  );
  expect(onIndex).toHaveBeenLastCalledWith(2);
  onIndex.mockClear();
  await render({ ...history, points: [] });
  await act(async () =>
    host
      .querySelector("svg")!
      .dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
      ),
  );
  expect(onIndex).not.toHaveBeenCalled();
  expect(host.textContent).toContain("No measurements available");
});

test("tight SLO scales retain distinct labels instead of rounding every tick to 100%", async () => {
  await render({
    ...history,
    target: 0.99999,
    points: [point("2026-10-24T10:00:00Z", 0.99999)],
  });
  const yLabels = Array.from(host.querySelectorAll("svg text"))
    .slice(0, 5)
    .map((node) => node.textContent);
  expect(new Set(yLabels).size).toBe(5);
});
