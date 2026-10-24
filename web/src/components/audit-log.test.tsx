// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { AuditLog } from "./audit-log";
import { createTenantSession } from "@/lib/api";
const reads = vi.hoisted(() => ({
  list: vi.fn(),
  detail: vi.fn(),
  archive: vi.fn(),
}));
vi.mock("@/lib/audit", () => ({
  readAudit: reads.list,
  readAuditRecord: reads.detail,
  readArchiveStatus: reads.archive,
}));
let host: HTMLDivElement, root: Root;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  reads.list.mockReset();
  reads.detail.mockReset();
  reads.archive.mockResolvedValue({ tenant: "alpha", enabled: false });
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
const record = {
  id: "decision",
  tenant: "alpha",
  conversation: "chat",
  event: {
    at: "2026-10-24T12:00:00Z",
    actor: { subject: "reviewer-id", name: "Reviewer", kind: "user" },
    action: "change_set.approved",
    outcome: "succeeded",
    reason: "Reviewed traffic exclusions.",
    changeSetDigest: "exact-digest",
  },
  resources: [{ name: "api" }],
};
test("owners can filter decision records and open scoped review details", async () => {
  reads.list.mockResolvedValue({
    tenant: "alpha",
    items: [record],
    from: "start",
    to: "end",
  });
  reads.detail.mockResolvedValue(record);
  const scope = createTenantSession("", "alpha", "owner");
  await act(async () => root.render(<AuditLog scope={scope} />));
  expect(host.textContent).toContain("change_set.approved");
  const input = host.querySelector<HTMLInputElement>('input[name="resource"]')!;
  input.value = "api";
  await act(async () =>
    host
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
  expect(reads.list.mock.lastCall![1].get("resource")).toBe("api");
  await act(async () =>
    host
      .querySelector<HTMLButtonElement>(
        'button[aria-label="View decision decision"]',
      )!
      .click(),
  );
  expect(host.textContent).toContain("Reviewed traffic exclusions.");
  expect(host.querySelector("a")?.getAttribute("href")).toBe(
    "/tenants/alpha/intents/chat",
  );
  scope.close();
});
test("viewer access does not fetch the tenant-wide journal", async () => {
  const scope = createTenantSession("", "alpha", "viewer");
  await act(async () => root.render(<AuditLog scope={scope} />));
  expect(reads.list).not.toHaveBeenCalled();
  expect(host.textContent).toContain("requires an owner or editor");
  scope.close();
});
test("late audit responses do not replace a new tenant page", async () => {
  let resolve!: (x: unknown) => void;
  reads.list
    .mockReturnValueOnce(new Promise((r) => (resolve = r)))
    .mockResolvedValueOnce({
      tenant: "beta",
      items: [],
      from: "start",
      to: "end",
    });
  const alpha = createTenantSession("", "alpha", "owner"),
    beta = createTenantSession("", "beta", "owner");
  await act(async () => root.render(<AuditLog scope={alpha} />));
  await act(async () => root.render(<AuditLog scope={beta} />));
  await act(async () => resolve({ tenant: "alpha", items: [record] }));
  expect(host.textContent).not.toContain("change_set.approved");
  expect(host.textContent).toContain("No decisions match");
  alpha.close();
  beta.close();
});
test("pending delivery stays visible and a verified receipt shows its exact object version", async () => {
  reads.list.mockResolvedValue({
    tenant: "alpha",
    items: [record],
    from: "start",
    to: "end",
  });
  reads.detail.mockResolvedValue(record);
  reads.archive
    .mockResolvedValueOnce({
      tenant: "alpha",
      enabled: true,
      state: "retrying",
      verified: 0,
      pending: 3,
      failed: 1,
      retained: 0,
      expired: 0,
      lastError: "retention_not_verified",
    })
    .mockResolvedValue({
      tenant: "alpha",
      enabled: true,
      state: "verified",
      verified: 1,
      pending: 0,
      failed: 0,
      retained: 1,
      expired: 0,
      receipt: {
        bucket: "audit-test",
        key: "record.json",
        versionId: "locked-version",
        sha256: "ab".repeat(32),
        mode: "COMPLIANCE",
        retainUntil: "2027-10-24T00:00:00Z",
        verifiedAt: "2026-10-24T00:00:00Z",
      },
    });
  const scope = createTenantSession("", "alpha", "owner");
  await act(async () => root.render(<AuditLog scope={scope} />));
  expect(host.textContent).toContain("0 verified");
  expect(host.textContent).toContain("3 pending");
  expect(host.textContent).toContain("retention_not_verified");
  await act(async () =>
    host
      .querySelector<HTMLButtonElement>(
        'button[aria-label="View decision decision"]',
      )!
      .click(),
  );
  expect(host.textContent).toContain("locked-version");
  expect(host.textContent).toContain("2027-10-24T00:00:00Z");
  scope.close();
});
