// @vitest-environment jsdom
import { Blob as NodeBlob } from "node:buffer";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { createTenantSession } from "@/lib/api";
import type { IntentConversation } from "@/lib/intent-conversations";
import { IntentGovernanceHistory } from "./intent-governance";
const requests = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: requests,
}));
let host: HTMLDivElement, root: Root, exported: NodeBlob | undefined;
const createURL = vi.fn((blob: NodeBlob) => {
  exported = blob;
  return "blob:audit-test";
});
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("Blob", NodeBlob);
  vi.stubGlobal("URL", {
    createObjectURL: createURL,
    revokeObjectURL: vi.fn(),
  });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  requests.mockReset();
  createURL.mockClear();
  exported = undefined;
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
async function startExport() {
  const scope = createTenantSession("", "alpha", "owner");
  const conversation = {
    id: "chat",
    tenant: "alpha",
    changeSetHistory: [],
    audit: [],
  } as unknown as IntentConversation;
  let pending: Promise<unknown> | undefined;
  await act(async () =>
    root.render(
      <IntentGovernanceHistory
        scope={scope}
        conversation={conversation}
        busy={false}
        running={false}
        onAction={async () => {}}
        onRead={(fn) => {
          pending = fn();
          void pending.catch(() => {});
        }}
      />,
    ),
  );
  vi.useFakeTimers();
  await act(async () => {
    Array.from(host.querySelectorAll("button"))
      .find((button) => button.textContent === "Download audit JSON")!
      .click();
  });
  return { pending: pending!, scope };
}
test("audit download follows the frozen sequence range and includes every event", async () => {
  requests
    .mockResolvedValueOnce({
      tenant: "alpha",
      conversation: "chat",
      integrity: "verified",
      events: [{ sequence: 1 }],
      nextAfter: 1,
      through: 2,
      changeSets: [],
    })
    .mockResolvedValueOnce({
      tenant: "alpha",
      conversation: "chat",
      integrity: "verified",
      events: [{ sequence: 2 }],
      through: 2,
    });
  const { pending, scope } = await startExport();
  await pending;
  expect(requests.mock.calls[1][0]).toBe(
    "/api/intent-conversations/chat/audit?after=1&through=2",
  );
  const result = JSON.parse(await exported!.text());
  expect(
    result.events.map((event: { sequence: number }) => event.sequence),
  ).toEqual([1, 2]);
  expect(result.nextAfter).toBeUndefined();
  scope.close();
});
test("a later page with a different scope never produces a partial audit download", async () => {
  requests
    .mockResolvedValueOnce({
      tenant: "alpha",
      conversation: "chat",
      integrity: "verified",
      events: [],
      nextAfter: 1,
      through: 2,
    })
    .mockResolvedValueOnce({
      tenant: "other",
      conversation: "chat",
      integrity: "verified",
      events: [],
      through: 2,
    });
  const { pending, scope } = await startExport();
  await expect(pending).rejects.toThrow("scope mismatch");
  expect(createURL).not.toHaveBeenCalled();
  scope.close();
});
