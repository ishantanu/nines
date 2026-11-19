// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import {
  IntentUsagePanel,
  humanDuration,
  type IntentUsage,
} from "./intent-usage";
import { createTenantSession } from "@/lib/api";
import { chooseOption } from "@/test-support/fields";

const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));
let root: Root, host: HTMLDivElement;
const scope = createTenantSession("synthetic", "alpha", "viewer");
const usage: IntentUsage = {
  tenant: "alpha",
  from: "2026-01-01T00:00:00Z",
  to: "2026-04-01T00:00:00Z",
  windowDays: 90,
  inspected: 4,
  started: 4,
  proposed: 3,
  approved: 2,
  applied: 2,
  rejected: 1,
  appliedFirstProposal: 1,
  rolledBack: 0,
  backtestAttached: 1,
  guidedRuns: 3,
  modelRuns: 0,
  medianSecondsToProposal: 600,
  medianSecondsToApplied: 7200,
  limitations: ["Counts describe workflow progress only."],
};
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  mocks.api.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
const open = () =>
  act(async () => root.render(<IntentUsagePanel scope={scope} />));

test("shows stages, medians and the limitations without any identity", async () => {
  mocks.api.mockResolvedValue(usage);
  await open();
  expect(mocks.api.mock.calls[0][0]).toBe(
    "/api/intent-conversations/usage?days=90",
  );
  const text = host.textContent!;
  expect(text).toContain("Workflow progress");
  expect(host.querySelectorAll('ol[aria-label="Stages"] li')).toHaveLength(4);
  expect(text).toContain("10 min");
  expect(text).toContain("2.0 h");
  expect(text).toContain("1 of 2");
  expect(text).toContain("50% needed no re-proposal");
  expect(text).toContain("Counts describe workflow progress only.");
});

test("an empty window says so and never invents medians", async () => {
  mocks.api.mockResolvedValue({
    ...usage,
    started: 0,
    proposed: 0,
    applied: 0,
    medianSecondsToProposal: undefined,
    medianSecondsToApplied: undefined,
  });
  await open();
  expect(host.textContent).toContain("No goals were started in this window.");
  expect(host.textContent).not.toContain("NaN");
});

test("changing the window refetches and errors stay visible", async () => {
  mocks.api
    .mockResolvedValueOnce(usage)
    .mockRejectedValueOnce(new Error("boom"));
  await open();
  await chooseOption("usage-days", "Last 7 days");
  expect(mocks.api.mock.calls[1][0]).toBe(
    "/api/intent-conversations/usage?days=7",
  );
  expect(host.querySelector('[role="alert"]')?.textContent).toContain("boom");
});

test("durations are rounded for reading, not for measurement", () => {
  expect(humanDuration(45)).toBe("45 s");
  expect(humanDuration(600)).toBe("10 min");
  expect(humanDuration(7200)).toBe("2.0 h");
  expect(humanDuration(172800)).toBe("2.0 d");
});
