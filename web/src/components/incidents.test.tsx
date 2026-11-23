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
  existing: vi.fn(),
  postmortems: vi.fn(),
  navigate: vi.fn(),
}));
vi.mock("@/lib/incidents", async (original) => ({
  ...(await original<typeof import("@/lib/incidents")>()),
  listIncidents: calls.list,
  declareIncident: calls.declare,
  transitionIncident: calls.transition,
}));
vi.mock("@/lib/investigations", () => ({
  createInvestigation: calls.review,
  findIncidentReview: calls.existing,
}));
vi.mock("@/lib/postmortems", async (original) => ({
  ...(await original<typeof import("@/lib/postmortems")>()),
  listIncidentPostmortems: calls.postmortems,
}));
vi.mock("@/lib/navigation", async (original) => ({
  ...(await original<typeof import("@/lib/navigation")>()),
  navigateTo: calls.navigate,
}));

const incident: Incident = {
  id: "6292a3c0-6b63-4891-add3-367b74e49e9a",
  revision: 2,
  title: "Checkout success is burning its error budget",
  source: "alert",
  detectedBySLO: true,
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
  calls.existing.mockResolvedValue(null);
  calls.postmortems.mockResolvedValue([]);
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

test("an incident no SLO detected says so and offers to define one", async () => {
  const external: Incident = {
    ...incident,
    source: "external",
    detectedBySLO: false,
    slos: [],
    impact: [],
    signals: [
      {
        source: "alertmanager",
        key: "g",
        title: "Postgres primary is down",
        url: "https://alertmanager.example",
        firing: true,
        firingSince: "2026-11-22T12:00:00Z",
      },
    ],
  };
  calls.list.mockResolvedValue([external]);
  const define = vi.fn();
  const scope = createTenantSession("", "alpha", "editor", "csrf");
  await act(async () =>
    root.render(
      <Incidents
        scope={scope}
        objectives={[]}
        reviewsOn
        onDefineSLO={define}
        onSLO={() => {}}
        onJourney={() => {}}
      />,
    ),
  );
  const text = host.textContent ?? "";
  expect(text).toContain("Not detected by any SLO");
  expect(text).toContain("Opened by an alert from Alertmanager");
  expect(text).toContain("Postgres primary is down");
  expect(
    host.querySelector('a[href="https://alertmanager.example"]'),
  ).not.toBeNull();
  await act(async () => button("Define an SLO")!.click());
  expect(define).toHaveBeenCalled();
});

test("Start review opens the incident's existing review instead of a second one", async () => {
  await show("editor");
  calls.existing.mockResolvedValue("review-0");
  await act(async () => button("Start review")!.click());
  expect(calls.review).not.toHaveBeenCalled();
  expect(calls.navigate).toHaveBeenCalledWith(
    "/tenants/alpha/reviews/review-0",
  );
});

test("the incident lists its postmortems: published ones link, drafts say so", async () => {
  calls.postmortems.mockResolvedValue([
    {
      reviewId: "review-1",
      reviewTitle: "Review: checkout burn",
      published: true,
      savedAt: "2026-11-23T09:00:00Z",
      savedBy: { subject: "u1", name: "Ana", kind: "user" },
      openActions: 2,
    },
    {
      reviewId: "review-2",
      reviewTitle: "Second look",
      published: false,
      savedAt: "2026-11-23T10:00:00Z",
      savedBy: { subject: "u2", name: "Ben", kind: "user" },
      openActions: 0,
    },
  ]);
  await show("editor");
  const text = host.textContent ?? "";
  expect(text).toContain("2 open action items");
  expect(text).toContain("Draft in progress by Ben");
  await act(async () => button("Review: checkout burn")!.click());
  expect(calls.navigate).toHaveBeenCalledWith(
    "/tenants/alpha/reviews/review-1",
  );
});
