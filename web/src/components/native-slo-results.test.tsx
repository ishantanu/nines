// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { NativeSLOResults } from "./native-slo-results";
import { createTenantSession, type SLO } from "@/lib/api";
const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));
let root: Root, host: HTMLDivElement;
const scope = createTenantSession("synthetic", "alpha", "viewer");
const slo = {
  id: "api",
  source: "dd",
  revision: 2,
  windowSeconds: 604800,
} as SLO;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
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
async function open() {
  await act(async () =>
    root.render(
      <NativeSLOResults slo={slo} scope={scope} provider="datadog" />,
    ),
  );
}
async function read() {
  await act(async () =>
    host
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
}
const result = {
  provider: "datadog",
  source: "dd",
  revision: 2,
  nativeID: "synthetic",
  state: "observed",
  from: "2026-01-01T00:00:00Z",
  to: "2026-01-02T00:00:00Z",
  checkedAt: "2026-01-02T00:00:00Z",
  target: 0.99,
  attainment: 0.95,
  budgetRemaining: -4,
  budgetBasis: "provider_reported",
  points: [{ time: "2026-01-01T00:01:00Z", attainment: 0.95 }],
  warnings: ["Independent native evidence"],
};
test("native results stay on demand and label provider evidence", async () => {
  mocks.api.mockResolvedValue(result);
  await open();
  expect(mocks.api).not.toHaveBeenCalled();
  expect(host.querySelectorAll("label")).toHaveLength(2);
  await read();
  expect(mocks.api).toHaveBeenCalledOnce();
  expect(mocks.api.mock.calls[0][0]).toMatch(
    /^\/api\/slos\/api\/native-results\?from=/,
  );
  expect(host.textContent).toContain("95.00%");
  expect(host.textContent).toContain("-400.00%");
  expect(host.textContent).toContain("Reported by Datadog");
  const chart = host.querySelector('svg[role="slider"]');
  expect(chart).not.toBeNull();
  expect(chart?.getAttribute("tabindex")).toBe("0");
  expect(chart?.getAttribute("aria-valuetext")).toContain("95.0%");
  expect(host.textContent).toContain("Independent native evidence");
});
test("empty native traffic is never shown as perfect attainment", async () => {
  mocks.api.mockResolvedValue({
    ...result,
    state: "no_data",
    attainment: null,
    budgetRemaining: null,
    points: [],
  });
  await open();
  await read();
  expect(host.textContent).toContain("No native SLI data");
  expect(host.textContent).toContain("No native measurement events");
  expect(host.textContent).not.toContain("100.00%");
});
test("revision conflicts and provider errors do not retain evidence", async () => {
  mocks.api.mockResolvedValue({ ...result, revision: 1 });
  await open();
  await read();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "SLO changed",
  );
  expect(host.textContent).not.toContain("95.00%");
  mocks.api.mockRejectedValue(Error("Provider unavailable"));
  await read();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "Provider unavailable",
  );
});
