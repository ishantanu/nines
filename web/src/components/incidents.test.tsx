// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Incidents } from "./incidents";
import { createTenantSession } from "@/lib/api";
import type { Incident } from "@/lib/incidents";

const calls = vi.hoisted(() => ({
  list: vi.fn(),
  declare: vi.fn(),
  transition: vi.fn(),
  review: vi.fn(),
  navigate: vi.fn(),
}));
vi.mock("@/lib/incidents", async (original) => ({
  ...(await original<typeof import("@/lib/incidents")>()),
  listIncidents: calls.list,
  declareIncident: calls.declare,
  transitionIncident: calls.transition,
}));
vi.mock("@/lib/investigations", () => ({ createInvestigation: calls.review }));
vi.mock("@/lib/navigation", async (original) => ({
  ...(await original<typeof import("@/lib/navigation")>()),
  navigateTo: calls.navigate,
}));

const incident: Incident = {
  id: "6292a3c0-6b63-4891-add3-367b74e49e9a",
  revision: 2,
  title: "Checkout success is burning its error budget",
  source: "alert",
  state: "monitoring",
  openedAt: "2026-11-22T13:41:00Z",
  slos: [
    {
      id: "checkout-success",
      configRevision: 1,
      policy: "fast-burn",
      firingSince: "2026-11-22T13:41:00Z",
      clearedAt: "2026-11-22T13:45:24Z",
      firing: false,
    },
  ],
  journeys: [],
  timeline: [
    {
      time: "2026-11-22T13:41:00Z",
      kind: "opened",
      message: "Opened by the critical burn alert on checkout-success.",
    },
  ],
  impact: [
    {
      sloId: "checkout-success",
      impactStartedAt: "2026-11-22T13:39:20Z",
      from: "2026-11-22T13:39:20Z",
      to: "2026-11-22T13:45:20Z",
      badEvents: 412,
      badEventsAtLeast: true,
      budgetConsumedFraction: null,
      coverage: 0.96,
      reason: "1 of 26 intervals during the incident have no data",
    },
  ],
};

let host: HTMLDivElement, root: Root;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  Object.values(calls).forEach((f) => f.mockReset());
  calls.list.mockResolvedValue([incident]);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

async function show(role: "editor" | "viewer", reviewsOn = true) {
  const scope = createTenantSession("", "alpha", role, "csrf");
  await act(async () =>
    root.render(
      <Incidents
        scope={scope}
        objectives={[]}
        reviewsOn={reviewsOn}
        onSLO={() => {}}
        onJourney={() => {}}
      />,
    ),
  );
  return scope;
}
const button = (text: string) =>
  [...document.querySelectorAll("button")].find((b) =>
    b.textContent?.includes(text),
  );

test("the detail shows when impact started, the alert lag and why budget is unknown", async () => {
  await show("editor");
  const text = host.textContent ?? "";
  expect(text).toContain("Checkout success is burning its error budget");
  expect(text).toContain("1 min 40 s before the alert");
  expect(text).toContain("At least 412");
  expect(text).toContain("Unknown");
  expect(text).toContain("1 of 26 intervals during the incident have no data");
  expect(text).toContain("Definition revision 1");
  expect(text).not.toMatch(/\b0(\.0)?% of the error budget/);
});

test("responders move an incident with a note at the revision they read", async () => {
  await show("editor");
  calls.transition.mockResolvedValue({
    ...incident,
    revision: 3,
    state: "resolved",
    timeline: [],
  });
  await act(async () => button("Mark resolved")!.click());
  const note = document.querySelector("textarea")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )!.set!;
    setter.call(note, "Stable for 30 minutes.");
    note.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("Save")!.click());
  expect(calls.transition).toHaveBeenCalledWith(
    expect.anything(),
    incident,
    "resolved",
    "Stable for 30 minutes.",
  );
});

test("viewers cannot change or declare incidents", async () => {
  await show("viewer");
  expect(button("Mark resolved")?.getAttribute("aria-disabled")).toBe("true");
  expect(button("Declare incident")?.getAttribute("aria-disabled")).toBe(
    "true",
  );
});

test("Start review opens a review scoped to the incident", async () => {
  await show("editor");
  calls.review.mockResolvedValue({ id: "review-1" });
  await act(async () => button("Start review")!.click());
  const input = calls.review.mock.calls[0][1];
  expect(input.scope).toMatchObject({
    kind: "Incident",
    resourceId: incident.id,
    expectedRevision: 2,
  });
  expect(calls.navigate).toHaveBeenCalledWith(
    "/tenants/alpha/reviews/review-1",
  );
});

test("Start review is hidden when reviews are turned off", async () => {
  await show("editor", false);
  expect(button("Start review")).toBeUndefined();
});
