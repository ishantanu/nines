// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { CohortConcentrationSummary } from "./cohort-concentration";
import type { CohortConcentration } from "@/lib/api";

const base: CohortConcentration = {
  state: "available",
  verdict: "concentrated",
  failures: 70,
  events: 100050,
  topShare: 0.714,
  effectiveCohorts: 2.2,
  entropy: 0.8,
  contributors: [
    {
      labels: { region: "ap" },
      failures: 50,
      events: 50,
      failureShare: 0.714,
      eventShare: 0.0005,
      representation: 1428.6,
    },
    {
      labels: { region: "eu" },
      failures: 20,
      events: 100000,
      failureShare: 0.286,
      eventShare: 0.9995,
      representation: 0.3,
    },
  ],
  limitations: ["It shows where failures landed, not why."],
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
const show = (c: CohortConcentration) =>
  act(async () =>
    root.render(<CohortConcentrationSummary concentration={c} />),
  );

test("a concentrated result names the cohort and compares failure share with traffic share", async () => {
  await show(base);
  const text = host.textContent!;
  expect(text).toContain("Failures are concentrated");
  expect(text).toContain("region=ap");
  expect(text).toContain("71.4%");
  expect(text).toContain("0.1%");
  expect(text).toContain("1428.6×");
  expect(text).toContain("It shows where failures landed, not why.");
});

test("thin, empty or incomplete evidence is explained instead of scored", async () => {
  await show({
    ...base,
    state: "insufficient_failures",
    verdict: undefined,
    failures: 5,
    contributors: [],
  });
  expect(host.textContent).toContain("Too few failures to read");
  expect(host.textContent).toContain("at least 30 are needed");
  expect(host.querySelector("table")).toBeNull();
  await show({
    ...base,
    state: "incomplete",
    verdict: undefined,
    contributors: [],
  });
  expect(host.textContent).toContain("unknown numerator");
  await show({
    ...base,
    state: "no_failures",
    verdict: undefined,
    contributors: [],
  });
  expect(host.textContent).toContain("No failures");
});
