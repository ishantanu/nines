// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { createTenantSession } from "@/lib/api";
import { VisualEvidence } from "./visual-evidence";
const calls = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  review: vi.fn(),
  read: vi.fn(),
  cap: vi.fn(),
}));
vi.mock("@/lib/visual-evaluations", () => ({
  visualNotice: "visual-sharing-v1",
  visualCapabilities: calls.cap,
  listVisual: calls.list,
  createVisual: calls.create,
  reviewVisual: calls.review,
  readVisualFile: calls.read,
}));
let host: HTMLDivElement, root: Root;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  for (const fn of Object.values(calls)) fn.mockReset();
  calls.cap.mockResolvedValue({
    enabled: true,
    routeDigest: "a".repeat(64),
    route: { provider: "typesafe", model: "jev-fixture" },
  });
  calls.list.mockResolvedValue([]);
  calls.read.mockResolvedValue("aW1hZ2U=");
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
function button(label: string) {
  return [...host.querySelectorAll("button")].find(
    (b) => b.textContent === label,
  )!;
}
async function render(role: "owner" | "agent" = "owner") {
  const scope = createTenantSession("", "alpha", role);
  await act(async () =>
    root.render(
      <VisualEvidence scope={scope} journeyID="checkout" revision={2} />,
    ),
  );
  return scope;
}
async function criterion(text: string) {
  await act(async () => {
    const input = host.querySelector<HTMLTextAreaElement>("#visual-criterion")!;
    Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )!.set!.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function image() {
  await act(async () => {
    const input = host.querySelector<HTMLInputElement>("#visual-image")!;
    Object.defineProperty(input, "files", {
      configurable: true,
      value: [new File(["synthetic"], "screen.png", { type: "image/png" })],
    });
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}
test("image sharing requires fresh consent for the exact input and never runs on upload", async () => {
  const scope = await render();
  await criterion("Confirmation is visible");
  await image();
  const submit = button("Share image and assess");
  expect(submit.disabled).toBe(true);
  expect(calls.create).not.toHaveBeenCalled();
  expect(host.querySelector("img")?.getAttribute("src")).toBe(
    "data:image/png;base64,aW1hZ2U=",
  );
  const consent = host.querySelector<HTMLInputElement>(
    'input[type="checkbox"]',
  )!;
  await act(async () => consent.click());
  expect(submit.disabled).toBe(false);
  await criterion("Confirmation plus no payment error");
  expect(consent.checked).toBe(false);
  expect(submit.disabled).toBe(true);
  await act(async () => consent.click());
  calls.create.mockImplementation(async (_scope, _journey, input) => ({
    id: input.id,
    tenant: "alpha",
    journeyId: "checkout",
    journeyRevision: 2,
    imageDigest: "hash",
    criterionDigest: "criterion-hash",
    requestDigest: "request",
    createdAt: new Date().toISOString(),
    expiresAt: new Date().toISOString(),
    route: {
      provider: "typesafe",
      model: "jev-fixture",
      contractDigest: "contract",
    },
    consent: {
      noticeVersion: input.noticeVersion,
      at: new Date().toISOString(),
      actor: { name: "Owner" },
    },
    assessment: {
      status: "assessed",
      judgments: {
        successVisible: 0.9,
        errorVisible: 0.1,
        evidenceSufficient: 0.8,
      },
    },
  }));
  await act(async () => submit.click());
  expect(calls.create).toHaveBeenCalledTimes(1);
  expect(calls.create.mock.calls[0][2]).toMatchObject({
    consent: true,
    noticeVersion: "visual-sharing-v1",
    journeyRevision: 2,
  });
  expect(host.textContent).toContain("Needs human review");
  expect(host.querySelector("img")?.alt).toContain("held locally");
  await act(async () => button("Clear local evidence").click());
  expect(host.querySelector("img")).toBeNull();
  expect(
    host.querySelector<HTMLInputElement>('input[type="checkbox"]')!.checked,
  ).toBe(false);
  expect(calls.review).not.toHaveBeenCalled();
  scope.close();
});
test("agent roles can read receipts but cannot grant sharing consent or review", async () => {
  const scope = await render("agent");
  expect(host.querySelector('input[type="file"]')).toBeNull();
  expect(host.textContent).toContain("An owner or editor");
  expect(calls.create).not.toHaveBeenCalled();
  scope.close();
});
test("foreign tenant receipts are rejected before rendering", async () => {
  calls.list.mockResolvedValue([
    { tenant: "beta", journeyId: "checkout", id: "foreign" },
  ]);
  const scope = await render();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "scope mismatch",
  );
  expect(host.textContent).not.toContain("foreign");
  scope.close();
});
