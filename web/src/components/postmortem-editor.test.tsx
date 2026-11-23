// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { PostmortemEditor } from "./postmortem-editor";
import { createTenantSession } from "@/lib/api";
import type { Incident } from "@/lib/incidents";
import type { Investigation } from "@/lib/investigations";

const calls = vi.hoisted(() => ({ read: vi.fn(), save: vi.fn() }));
vi.mock("@/lib/postmortems", async (original) => ({
  ...(await original<typeof import("@/lib/postmortems")>()),
  readPostmortem: calls.read,
  savePostmortem: calls.save,
}));

const review: Investigation = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  id: "review-1",
  title: "Review: replica lag",
  version: 3,
  scope: {
    kind: "Incident",
    resourceId: "incident-1",
    expectedRevision: 1,
    at: "2026-11-22T13:00:00Z",
  },
  createdAt: "2026-11-22T13:00:00Z",
  updatedAt: "2026-11-22T13:00:00Z",
  createdBy: { subject: "u", name: "Ana", kind: "user" },
  entries: [],
};
const incident = {
  id: "incident-1",
  revision: 1,
  title: "Replica lag",
  source: "external",
  state: "monitoring",
  detectedBySLO: false,
  openedAt: "2026-11-22T12:00:00Z",
  slos: [],
  journeys: [],
  timeline: [
    {
      time: "2026-11-22T12:00:00Z",
      kind: "opened",
      message: "Opened by the Alertmanager alert.",
    },
  ],
  impact: [],
} as Incident;
const template = {
  postmortem: {
    summary: "",
    addedEvents: [],
    contributingCauses: [],
    lessons: { wentWell: [], wentPoorly: [], gotLucky: [] },
    actionItems: [
      {
        id: "define-slo",
        title: "Define an SLO that would have caught this incident",
        owner: "",
        kind: "detect",
        done: false,
      },
    ],
    published: false,
  },
  findings: [],
};

let host: HTMLDivElement, root: Root;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  calls.read.mockReset().mockResolvedValue(structuredClone(template));
  calls.save.mockReset();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
async function show(role: "editor" | "viewer") {
  await act(async () =>
    root.render(
      <PostmortemEditor
        scope={createTenantSession("", "alpha", role, "csrf")}
        review={review}
        incident={incident}
        objectives={[]}
        journeys={[]}
        onSaved={() => {}}
      />,
    ),
  );
}
const button = (text: string) =>
  [...document.querySelectorAll("button")].find(
    (b) =>
      b.textContent?.includes(text) || b.getAttribute("aria-label") === text,
  );
async function type(el: HTMLTextAreaElement | HTMLInputElement, value: string) {
  await act(async () => {
    const proto =
      el instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

test("timeline and impact come from the captured incident; the standing item stays", async () => {
  await show("editor");
  const text = host.textContent ?? "";
  expect(text).toContain("Opened by the Alertmanager alert.");
  expect(text).toContain("No SLO measured this incident");
  const titles = [...document.querySelectorAll("input")].map((i) => i.value);
  expect(titles).toContain(
    "Define an SLO that would have caught this incident",
  );
  expect(text).toContain("this item stays until the SLO exists");
  expect(button("Remove action 1")).toBeUndefined();
  // Contributing causes, and no field for a single root cause.
  expect(text).toContain("Contributing causes");
  expect(
    [...document.querySelectorAll("[aria-label],label")].some((el) =>
      /root cause/i.test(el.getAttribute("aria-label") ?? el.textContent ?? ""),
    ),
  ).toBe(false);
});

test("hindsight wording is flagged as people type, and saving sends the review version", async () => {
  await show("editor");
  await type(
    document.querySelector("textarea")!,
    "The on-call engineer should have failed over sooner.",
  );
  expect(host.textContent).toContain("“should have”");
  calls.save.mockResolvedValue({
    investigation: { ...review, version: 4 },
    findings: [{ field: "summary", phrase: "should have", suggestion: "x" }],
  });
  await act(async () => button("Save revision")!.click());
  const [, sentReview, sent] = calls.save.mock.calls[0];
  expect(sentReview.version).toBe(3);
  expect(sent.summary).toContain("should have");
  expect(sent.actionItems[0].id).toBe("define-slo");
  expect(host.textContent).toContain("1 wording note to consider");
});

test("viewers read the postmortem but cannot edit it", async () => {
  await show("viewer");
  expect(button("Save revision")?.getAttribute("aria-disabled")).toBe("true");
  expect(host.textContent).toContain("Only owners and editors can edit.");
});
