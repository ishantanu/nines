// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { CohortOutlierSummary } from "./cohort-outliers";
import type { CohortOutliers } from "@/lib/api";

let root: Root, host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
const base: CohortOutliers = {
  state: "available",
  threshold: -3,
  items: [],
  limitations: ["Not an alert."],
};
const show = (o: CohortOutliers) =>
  act(async () => root.render(<CohortOutlierSummary outliers={o} />));

test("a flagged cohort shows its counts against its peers", async () => {
  await show({
    ...base,
    items: [
      {
        labels: { region: "eu" },
        total: 10000,
        reliability: 0.97,
        peerReliability: 0.999,
        zScore: -16.3,
      },
    ],
  });
  const t = host.textContent!;
  expect(t).toContain("1 below peers");
  expect(t).toContain("region=eu: 97.00% against 99.90%");
  expect(t).toContain("z = -16.3");
  expect(t).toContain("Not an alert.");
});

test("states without a comparison say why instead of showing none", async () => {
  await show({ ...base, state: "insufficient" });
  expect(host.textContent).toContain("Too few cohorts to compare");
  await show({ ...base, state: "incomplete" });
  expect(host.textContent).toContain("Incomplete evidence");
  await show(base);
  expect(host.textContent).toContain("None below peers");
});
