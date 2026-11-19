// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { RunObjectiveCard } from "./run-objective";
import type { JourneyRunObjective } from "@/lib/api";

const base: JourneyRunObjective = {
  state: "available",
  target: 0.9,
  windowSeconds: 86400,
  minRuns: 5,
  from: "2026-11-17T12:00:00Z",
  to: "2026-11-18T12:00:00Z",
  passed: 4,
  failed: 1,
  violated: 1,
  unjudged: 2,
  otherRevision: 3,
  judged: 6,
  reliability: 4 / 6,
  burnRate: 3.33,
  budgetRemainingFraction: -2.33,
  truncated: false,
  limitations: [
    "Incomplete and inconclusive runs are never counted as success.",
  ],
};
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
const show = (o: JourneyRunObjective) =>
  act(async () => root.render(<RunObjectiveCard objective={o} />));

test("a missed objective shows the counts, unjudged and other-contract runs separately", async () => {
  await show(base);
  const text = host.textContent!;
  expect(text).toContain("Not met");
  expect(text).toContain("66.7% of 6 judged runs passed");
  expect(text).toContain("Unjudged");
  expect(text).toContain("Other contract");
  expect(text).toContain("never counted as success");
});

test("too few runs are explained instead of scored, and truncation is stated", async () => {
  await show({
    ...base,
    state: "insufficient_runs",
    judged: 2,
    reliability: null,
    burnRate: null,
    budgetRemainingFraction: null,
    truncated: true,
  });
  const text = host.textContent!;
  expect(text).toContain("Too few runs");
  expect(text).toContain("2 of 5 judged runs so far");
  expect(text).not.toContain("Met");
  expect(text).toContain("latest 200 runs");
});
