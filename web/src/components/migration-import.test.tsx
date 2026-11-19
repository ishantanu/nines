// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { MigrationImport } from "./migration-import";
import { createTenantSession } from "@/lib/api";
import { noteCounts } from "@/lib/migration";

const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));
let root: Root, host: HTMLDivElement;
const editor = createTenantSession("synthetic", "alpha", "editor");
const viewer = createTenantSession("synthetic", "alpha", "viewer");
const result = {
  tool: "sloth",
  definitions: [
    {
      name: "svc-a",
      origin: "svc/a",
      yaml: "kind: ServiceLevelObjective\nmetadata:\n  name: svc-a\n",
      notes: [
        {
          level: "lossy",
          field: "description",
          message: "Nines SLOs have no description field.",
        },
        {
          level: "review",
          field: "sli.events",
          message: "rate() became increase().",
        },
      ],
    },
    {
      name: "svc-b",
      origin: "svc/b",
      yaml: "kind: ServiceLevelObjective\n",
      notes: [],
    },
  ],
  skipped: [
    {
      origin: "svc/raw",
      reason: "a raw error-ratio query has no event counts",
    },
  ],
  notes: [{ level: "info", message: "Imported SLOs start experimental." }],
  limitations: ["Conversion is offline."],
};
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  mocks.api.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
});
const disabled = (b: HTMLButtonElement) =>
  b.disabled || b.getAttribute("aria-disabled") === "true";
const flush = () =>
  act(async () => {
    await Promise.resolve();
  });
const click = (text: string) =>
  act(async () => {
    [...document.querySelectorAll("button")]
      .find((b) => b.textContent?.includes(text))!
      .click();
  });
const typeInto = (selector: string, value: string) =>
  act(async () => {
    const el = document.querySelector(selector) as HTMLTextAreaElement;
    Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });

async function openAndConvert(scope = editor) {
  await act(async () =>
    root.render(<MigrationImport scope={scope} onApplied={() => {}} />),
  );
  await click("Import from Sloth or Pyrra");
  await typeInto("#migration-content", "version: prometheus/v1");
  await click("Convert");
  await flush();
}

test("conversion shows skipped items, every note and imports start experimental", async () => {
  mocks.api.mockResolvedValue(result);
  await openAndConvert();
  const body = JSON.parse(mocks.api.mock.calls[0][2].body);
  expect(mocks.api.mock.calls[0][0]).toBe("/api/migrations/convert");
  expect(body).toMatchObject({
    tool: "sloth",
    lifecycle: "experimental",
    project: "production",
    windowSeconds: 30 * 86400,
  });
  const text = document.body.textContent!;
  expect(text).toContain("2 converted, 1 skipped");
  expect(text).toContain("a raw error-ratio query has no event counts");
  expect(text).toContain("Imported SLOs start experimental");
  expect(text).toContain("1 not carried over");
  expect(text).toContain("1 to review");
  // Nothing was planned or applied by converting.
  expect(mocks.api.mock.calls.map((c) => c[0])).toEqual([
    "/api/migrations/convert",
  ]);
});

test("apply needs a plan, an acknowledgement and uses each plan's reviewed revision", async () => {
  mocks.api.mockImplementation(
    async (path: string, _s: unknown, init?: RequestInit) => {
      if (path === "/api/migrations/convert") return result;
      if (path === "/api/resources/plan")
        return {
          action: String(init?.body).includes("svc-a") ? "update" : "create",
          expectedRevision: String(init?.body).includes("svc-a") ? 3 : 0,
          warnings: ["plan warning"],
        };
      return {};
    },
  );
  await openAndConvert();
  // No apply button until something is planned.
  expect(
    [...document.querySelectorAll("button")].some((b) =>
      b.textContent?.includes("Apply"),
    ),
  ).toBe(false);
  await click("Plan 2 selected");
  await flush();
  expect(document.body.textContent).toContain("plan warning");
  const apply = () =>
    [...document.querySelectorAll("button")].find((b) =>
      b.textContent?.startsWith("Apply 2"),
    )!;
  expect(disabled(apply())).toBe(true);
  await act(
    async () =>
      document.querySelector(
        "input[type=checkbox]:not([aria-label]):last-of-type",
      ) as HTMLInputElement | null,
  );
  const ack = [...document.querySelectorAll("label")]
    .find((l) => l.textContent?.includes("I reviewed"))!
    .querySelector("input")!;
  await act(async () => ack.click());
  expect(disabled(apply())).toBe(false);
  await click("Apply 2 planned");
  await flush();
  const applies = mocks.api.mock.calls.filter(
    (c) => c[0] === "/api/resources/apply",
  );
  expect(applies.map((c) => c[2].headers["If-Match"])).toEqual(['"3"', '"0"']);
  expect(document.body.textContent).toContain("Applied");
});

test("viewers can convert and plan but never apply, and committed warns about double paging", async () => {
  mocks.api.mockImplementation(async (path: string) =>
    path === "/api/migrations/convert"
      ? result
      : { action: "create", expectedRevision: 0 },
  );
  await openAndConvert(viewer);
  await click("Plan 2 selected");
  await flush();
  const ack = [...document.querySelectorAll("label")]
    .find((l) => l.textContent?.includes("I reviewed"))!
    .querySelector("input")!;
  await act(async () => ack.click());
  expect(
    disabled(
      [...document.querySelectorAll("button")].find((b) =>
        b.textContent?.startsWith("Apply 2"),
      )!,
    ),
  ).toBe(true);
  expect(document.body.textContent).toContain("convert and plan but not apply");
  await act(async () =>
    (document.querySelector("#migration-lifecycle") as HTMLElement).click(),
  );
  await act(async () => {
    [...document.querySelectorAll('[role="option"]')]
      .find((o) => o.textContent?.includes("Committed"))!
      .dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  expect(document.body.textContent).toContain("you will be paged twice");
});

test("note counting separates what is lost from what to review", () => {
  expect(
    noteCounts([
      { level: "lossy", message: "a" },
      { level: "review", message: "b" },
      { level: "info", message: "c" },
    ]),
  ).toEqual({ lossy: 1, review: 1 });
});
