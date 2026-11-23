// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { OpenActionItems } from "./open-action-items";
import { createTenantSession } from "@/lib/api";
import { FeaturesProvider } from "@/lib/features";

const calls = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/postmortems", async (original) => ({
  ...(await original<typeof import("@/lib/postmortems")>()),
  listOpenActionItems: calls.list,
}));

let host: HTMLDivElement, root: Root;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  calls.list.mockReset();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
const scope = createTenantSession("", "alpha", "viewer", "csrf");
async function show(features: string[]) {
  await act(async () =>
    root.render(
      <FeaturesProvider enabled={features}>
        <OpenActionItems scope={scope} slo="checkout" />
      </FeaturesProvider>,
    ),
  );
}

test("open action items show on the SLO they change", async () => {
  calls.list.mockResolvedValue([
    {
      reviewId: "review-1",
      reviewTitle: "Review",
      incidentId: "i1",
      item: {
        id: "a1",
        title: "Page on replication lag",
        owner: "db-team",
        due: "2026-12-15",
        kind: "detect",
        done: false,
      },
    },
  ]);
  await show(["reliability-reviews"]);
  expect(calls.list).toHaveBeenCalledWith(
    scope,
    { slo: "checkout", journey: undefined },
    expect.anything(),
  );
  expect(host.textContent).toContain("Page on replication lag");
  expect(host.textContent).toContain("Detect · db-team · due 2026-12-15");
});

test("nothing shows without items, or with reviews turned off", async () => {
  calls.list.mockResolvedValue([]);
  await show(["reliability-reviews"]);
  expect(host.textContent).toBe("");
  await show([]);
  expect(host.textContent).toBe("");
});
