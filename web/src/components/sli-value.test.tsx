import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { SLIValue } from "./sli-value";
import { Objectives } from "./objectives";
import { sliTimelineLines } from "./slo-performance";
import type { HistoryPoint } from "@/lib/history";
import type { Objective, Snapshot } from "@/lib/api";
const snapshot: Snapshot = {
  sloId: "api",
  at: "2026-10-23T12:00:00Z",
  from: "2026-10-22T12:00:00Z",
  good: 9995,
  total: 10000,
  bad: 5,
  coverage: 0.1,
  reliability: 0.9995,
  budgetRemainingFraction: 0.5,
  burnRate: 0.5,
  dataStatus: "partial",
  alert: { phase: "inactive" },
};
const objective = (status: Snapshot): Objective => ({
  slo: {
    id: "api",
    name: "API availability",
    service: "api",
    project: "production",
    target: 0.999,
    windowSeconds: 86400,
    stepSeconds: 60,
    goodQuery: "good",
    totalQuery: "total",
  },
  status,
});
test("partial windows show actual SLI and target without implying a healthy full window", () => {
  const html = renderToStaticMarkup(
    <SLIValue status={snapshot} target={0.999} />,
  );
  expect(html).toContain("99.950% SLI");
  expect(html).toContain("99.900% target");
  expect(html).toContain("Partial window");
  expect(html).toContain("10.0% coverage");
  expect(html).not.toMatch(/unknown|healthy|Full window/);
});
test("tiny positive coverage is distinct from no measurements", () => {
  const html = renderToStaticMarkup(
    <SLIValue status={{ ...snapshot, coverage: 1 / 40320 }} target={0.999} />,
  );
  expect(html).toContain("&lt;0.1% coverage");
  expect(html).toContain("99.950% SLI");
});
test("missing telemetry and sampled zero traffic never become fabricated percentages", () => {
  for (const [dataStatus, text] of [
    ["no_data", "No SLI data"],
    ["no_traffic", "No traffic"],
  ]) {
    const html = renderToStaticMarkup(
      <SLIValue
        status={{ ...snapshot, dataStatus, reliability: null }}
        target={0.999}
      />,
    );
    expect(html).toContain(text);
    expect(html).not.toContain("% SLI");
    expect(html).not.toContain("Unknown");
  }
});
test("a continuing alert remains visible even when the measurement window is incomplete", () => {
  const html = renderToStaticMarkup(
    <SLIValue
      status={{ ...snapshot, alert: { phase: "firing" } }}
      target={0.999}
    />,
  );
  expect(html).toContain("Alert firing");
  expect(html).toContain("99.950% SLI");
  expect(html).toContain("Partial window");
});
test("objective rows show attainment and independent alert state", () => {
  const html = renderToStaticMarkup(
    <Objectives
      items={[objective(snapshot)]}
      journeys={[]}
      onSelect={() => {}}
      onJourney={() => {}}
    />,
  );
  expect(html).toContain("SLI / SLO target");
  expect(html).toContain("99.950% SLI");
  expect(html).toContain("Based on observed data");
  expect(html).toContain("No active alerts");
  expect(html).not.toContain("unknown");
});
test("a missing budget is not rendered as an exhausted progress bar", () => {
  const html = renderToStaticMarkup(
    <Objectives
      items={[
        objective({
          ...snapshot,
          good: 0,
          total: 0,
          reliability: null,
          coverage: 0,
          dataStatus: "no_data",
          budgetRemainingFraction: null,
          burnRate: null,
        }),
      ]}
      journeys={[]}
      onSelect={() => {}}
      onJourney={() => {}}
    />,
  );
  expect(html).toContain("No SLI data");
  expect(html).toContain("No estimate");
  expect(html).toContain("Not evaluated");
  expect(html).not.toContain('role="progressbar"');
});

test("charts retain observed rolling SLI while distinguishing partial and complete windows", () => {
  const point = {
    at: snapshot.at,
    coverage: 0.1,
    reliability: snapshot.reliability,
    intervalCoverage: 1,
    intervalReliability: snapshot.reliability,
  } as HistoryPoint;
  expect(sliTimelineLines).toHaveLength(1);
  const measured = sliTimelineLines[0];
  expect(measured.name).toBe("Measured SLI");
  expect(measured.get(point)).toBe(snapshot.reliability);
  expect(measured.partial?.(point)).toBe(true);
  expect(measured.get({ ...point, coverage: 1 })).toBe(snapshot.reliability);
  expect(measured.partial?.({ ...point, coverage: 1 })).toBe(false);
  expect(measured.get({ ...point, coverage: 0, reliability: null })).toBeNull();
});

test("compact journey-map values retain the partial-window qualification", () => {
  const html = renderToStaticMarkup(<SLIValue compact status={snapshot} />);
  expect(html).toContain("99.950% SLI");
  expect(html).toContain("Partial window");
});

test("lifecycle suppression remains separate from measured attainment", () => {
  const html = renderToStaticMarkup(
    <SLIValue
      status={{
        ...snapshot,
        alert: {
          phase: "inactive",
          suppressedReason: "lifecycle_aspirational",
        },
      }}
      target={0.999}
    />,
  );
  expect(html).toContain("99.950% SLI");
  expect(html).toContain("Alerts disabled");
  expect(html).not.toContain("No active alerts");
});
