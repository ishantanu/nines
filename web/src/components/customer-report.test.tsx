// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { CustomerReportPanel } from "./customer-report";
import { createTenantSession, type CustomerReport } from "@/lib/api";

const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));
let root: Root, host: HTMLDivElement;
const scope = createTenantSession("synthetic", "alpha", "viewer");
const report: CustomerReport = {
  sloId: "api",
  revision: 3,
  tenant: "alpha",
  state: "available",
  message: "Observed per-customer estimates at the same cutoff as the SLO.",
  at: "2026-11-18T10:00:00Z",
  windowSeconds: 3600,
  label: "account",
  serviceBurnRate: 0.5,
  limitations: ["A customer with no matching series is reported as no data."],
  items: [
    {
      name: "Acme",
      alias: "customer-1",
      state: "observed",
      good: 900,
      total: 1000,
      target: 0.99,
      reliability: 0.9,
      burnRate: 10,
      budgetRemainingFraction: -9,
      maskedByAggregate: true,
    },
    {
      name: "ghost",
      alias: "customer-2",
      state: "no_series",
      good: null,
      total: 0,
      target: 0.99,
      reliability: null,
      burnRate: null,
      budgetRemainingFraction: null,
      maskedByAggregate: false,
    },
  ],
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
  act(async () =>
    root.render(<CustomerReportPanel sloId="api" revision={3} scope={scope} />),
  );
const inspect = () =>
  act(async () =>
    [...host.querySelectorAll("button")]
      .find((b) => b.textContent?.includes("Inspect customers"))!
      .click(),
  );

test("nothing is read until the user asks, then a hidden customer is flagged and no-data is not healthy", async () => {
  mocks.api.mockResolvedValue(report);
  await open();
  expect(mocks.api).not.toHaveBeenCalled();
  await inspect();
  expect(mocks.api.mock.calls[0][0]).toBe(
    "/api/slos/api/customers?expectedRevision=3",
  );
  const text = host.textContent!;
  expect(text).toContain("Acme");
  expect(text).toContain("10.00×");
  expect(text).toContain("Hidden by the average");
  expect(text).toContain("Service-wide burn 0.50×");
  expect(text).toContain("Unavailable");
  expect(text).toContain("No data");
  expect(text).toContain("reported as no data");
});

test("an unsupported provider is explained and errors stay visible", async () => {
  mocks.api.mockResolvedValueOnce({
    ...report,
    state: "unsupported",
    message:
      "This provider or measurement mode cannot supply per-customer counts.",
    items: [],
  });
  await open();
  await inspect();
  expect(host.textContent).toContain("cannot supply per-customer counts");
  expect(host.querySelector("table")).toBeNull();
  mocks.api.mockRejectedValueOnce(new Error("boom"));
  await inspect();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain("boom");
});
