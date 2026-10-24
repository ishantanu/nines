// @vitest-environment jsdom
import { act, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { createTenantSession } from "@/lib/api";
import { MaskedReplayCapture } from "@/lib/replay-capture";
import { replayFixture } from "@/lib/replay-test-fixtures";
import { navigateTo, workspacePath } from "@/lib/navigation";
import type { SessionReplay } from "@/lib/replays";
import { SessionReplays } from "./session-replays";
const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  events: vi.fn(),
  stop: vi.fn(),
  record: vi.fn(),
}));
vi.mock("@rrweb/record", () => ({ record: mocks.record }));
vi.mock("@/lib/replays", async (original) => ({
  ...(await original<typeof import("@/lib/replays")>()),
  listReplays: mocks.list,
  createReplay: mocks.create,
  replayEvents: mocks.events,
}));
let host: HTMLDivElement, root: Root;
function metadata(): SessionReplay {
  return {
    apiVersion: "nines.dev/v1alpha1",
    tenant: "alpha",
    id: "saved",
    title: "Synthetic",
    version: 1,
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(),
    startedAt: new Date().toISOString(),
    finishedAt: new Date().toISOString(),
    durationMillis: 1,
    eventCount: 2,
    sizeBytes: 300,
    digest: "synthetic-digest",
    policy: "masked-v1",
    recordedBy: { name: "Synthetic" },
    references: [],
    archived: false,
  };
}
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  for (const mock of Object.values(mocks)) mock.mockReset();
  mocks.record.mockImplementation((options) => {
    replayFixture().forEach(options.emit);
    return mocks.stop;
  });
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  mocks.list.mockResolvedValue({ tenant: "alpha", items: [] });
  mocks.events.mockResolvedValue({
    tenant: "alpha",
    id: "saved",
    digest: "synthetic-digest",
    events: replayFixture(),
  });
  window.history.replaceState(null, "", workspacePath("alpha", "replays"));
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
async function settle() {
  await act(async () => {
    await Promise.resolve();
  });
}
function button(name: string) {
  return [...host.querySelectorAll("button")].find(
    (button) => button.textContent === name,
  )!;
}
async function render(capture = new MaskedReplayCapture()) {
  const scope = createTenantSession("", "alpha", "owner");
  await act(async () =>
    root.render(
      <StrictMode>
        <SessionReplays
          scope={scope}
          capture={capture}
          options={[]}
          onResource={() => {}}
        />
      </StrictMode>,
    ),
  );
  await settle();
  return { capture, scope };
}
async function pendingSave() {
  const { capture, scope } = await render();
  await act(async () => {
    await capture.start(true);
    capture.stop();
  });
  await act(async () => {
    const title = host.querySelector<HTMLInputElement>("#replay-title")!;
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(title, "Synthetic");
    title.dispatchEvent(new Event("input", { bubbles: true }));
  });
  let resolve!: (value: SessionReplay) => void;
  mocks.create.mockImplementation(
    () =>
      new Promise<SessionReplay>((r) => {
        resolve = r;
      }),
  );
  expect(button("Save recording").disabled).toBe(false);
  await act(async () => button("Save recording").click());
  return { capture, scope, resolve };
}
test("a late save response cannot navigate away from the user's next page", async () => {
  const { capture, scope, resolve } = await pendingSave();
  await act(async () => root.render(<h1>Next page</h1>));
  navigateTo(workspacePath("alpha", "home"));
  await act(async () => resolve(metadata()));
  expect(window.location.pathname).toBe(workspacePath("alpha", "home"));
  expect(capture.getSnapshot().status).toBe("idle");
  scope.close();
});
test("an older save completion cannot discard a newer recording", async () => {
  const { capture, scope, resolve } = await pendingSave();
  await act(async () => {
    capture.discard();
    await capture.start(true);
    capture.stop();
  });
  const version = capture.getRecordingVersion();
  await act(async () => resolve(metadata()));
  expect(capture.getRecordingVersion()).toBe(version);
  expect(capture.getSnapshot().status).toBe("ready");
  expect(capture.export()).toHaveLength(2);
  await act(async () => {
    scope.close();
    capture.discard();
  });
});
test("empty or mismatched event responses produce a visible error instead of a blank player", async () => {
  mocks.list.mockResolvedValue({ tenant: "alpha", items: [metadata()] });
  mocks.events.mockResolvedValue({
    tenant: "alpha",
    id: "saved",
    digest: "synthetic-digest",
    events: [],
  });
  const { scope } = await render();
  await settle();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "2–2000 events",
  );
  expect(host.querySelector("iframe")).toBeNull();
  scope.close();
});
test("expired or missing replay deep links explain what happened", async () => {
  window.history.replaceState(
    null,
    "",
    workspacePath("alpha", "replays", "expired"),
  );
  const { scope } = await render();
  expect(host.textContent).toContain(
    "This recording is unavailable or has expired.",
  );
  scope.close();
});
test("missing player initialization becomes an explicit retryable error", async () => {
  vi.useFakeTimers();
  mocks.list.mockResolvedValue({ tenant: "alpha", items: [metadata()] });
  mocks.events.mockResolvedValue({
    tenant: "alpha",
    id: "saved",
    digest: "synthetic-digest",
    events: replayFixture(),
  });
  const { scope } = await render();
  expect(host.querySelector("iframe")).not.toBeNull();
  await act(async () => vi.advanceTimersByTime(10001));
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "could not initialize",
  );
  expect(host.querySelector("iframe")).toBeNull();
  scope.close();
});

test("foreign-tenant list metadata is rejected before it is displayed", async () => {
  mocks.list.mockResolvedValue({
    tenant: "alpha",
    items: [{ ...metadata(), tenant: "beta", title: "Foreign title" }],
  });
  const { scope } = await render();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "Replay tenant mismatch",
  );
  expect(host.textContent).not.toContain("Foreign title");
  scope.close();
});
test("foreign-tenant save responses cannot discard the unsaved recording", async () => {
  const { capture, scope, resolve } = await pendingSave();
  const version = capture.getRecordingVersion();
  await act(async () => resolve({ ...metadata(), tenant: "beta" }));
  expect(capture.getRecordingVersion()).toBe(version);
  expect(capture.getSnapshot().status).toBe("ready");
  expect(window.location.pathname).toBe(workspacePath("alpha", "replays"));
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "Replay tenant mismatch",
  );
  await act(async () => {
    scope.close();
    capture.discard();
  });
});
