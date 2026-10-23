import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { SLOHealthNotice } from "./slo-health-notice";
import type { Snapshot } from "@/lib/api";
const snapshot: Snapshot = {
  sloId: "api",
  at: "now",
  from: "earlier",
  good: 100,
  total: 100,
  bad: 0,
  coverage: 0.1,
  reliability: 1,
  budgetRemainingFraction: 1,
  burnRate: 0,
  dataStatus: "partial",
  alert: { phase: "inactive" },
};
test("perfect observed ratios cannot hide incomplete window coverage", () => {
  const html = renderToStaticMarkup(<SLOHealthNotice status={snapshot} />);
  expect(html).toContain("Partial measurement window.");
  expect(html).toContain("10.000%");
  expect(html).toContain(
    "SLI attainment is calculated from the available measurements",
  );
  expect(html).toContain("not yet a full-window compliance result");
});
test("zero traffic has its own explanation even with complete sampling", () => {
  const html = renderToStaticMarkup(
    <SLOHealthNotice
      status={{
        ...snapshot,
        coverage: 1,
        reliability: null,
        dataStatus: "no_traffic",
      }}
    />,
  );
  expect(html).toContain("Measurements contain no traffic");
  expect(html).not.toContain("missing intervals");
});
test("complete healthy measurements need no unknown notice", () => {
  expect(
    renderToStaticMarkup(
      <SLOHealthNotice
        status={{ ...snapshot, coverage: 1, dataStatus: "complete" }}
      />,
    ),
  ).toBe("");
});
