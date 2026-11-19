// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import {
  DependencyCompositionPanel,
  type DependencyComposition,
} from "./dependency-composition";
import { createTenantSession } from "@/lib/api";

const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));
let root: Root, host: HTMLDivElement;
const scope = createTenantSession("synthetic", "alpha", "viewer");
const data: DependencyComposition = {
  tenant: "alpha",
  id: "services",
  limitations: ["Assumes independent failures."],
  checks: [
    {
      service: "web",
      sloId: "web-availability",
      target: 0.9995,
      verdict: "exceeds_ceiling",
      ceiling: 0.998001,
      headroom: -0.001499,
      dependencies: [{ service: "api", sloId: "api-a", target: 0.999 }],
      unmeasuredDependencies: ["queue"],
    },
    {
      service: "api",
      sloId: "api-a",
      target: 0.999,
      verdict: "no_ceiling",
      dependencies: [],
      unmeasuredDependencies: [],
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
    root.render(
      <DependencyCompositionPanel scope={scope} id="services" revision={1} />,
    ),
  );

test("an objective above its ceiling is flagged with exact numbers and the unmeasured dependency", async () => {
  mocks.api.mockResolvedValue(data);
  await open();
  expect(mocks.api.mock.calls[0][0]).toBe(
    "/api/dependency-maps/services/composition",
  );
  const text = host.textContent!;
  expect(text).toContain("Above ceiling");
  expect(text).toContain("99.950%");
  expect(text).toContain("99.800%");
  expect(text).toContain("−0.150 pts");
  expect(text).toContain("no committed availability SLO for queue");
  expect(text).toContain("Assumes independent failures.");
  // Objectives with no ceiling are not listed as if they were checked.
  expect(host.querySelectorAll("tbody tr")).toHaveLength(1);
});

test("with no ceiling it says so rather than showing an empty table", async () => {
  mocks.api.mockResolvedValue({ ...data, checks: [data.checks[1]] });
  await open();
  expect(host.textContent).toContain("no ceiling can be computed");
  expect(host.querySelector("table")).toBeNull();
});

test("errors stay visible", async () => {
  mocks.api.mockRejectedValue(new Error("boom"));
  await open();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain("boom");
});
