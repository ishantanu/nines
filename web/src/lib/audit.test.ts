import { afterEach, expect, test, vi } from "vitest";
import { createTenantSession } from "./api";
import { readAudit, readAuditRecord } from "./audit";
afterEach(() => vi.unstubAllGlobals());
test("audit reads keep immutable tenant scope and reject cross-tenant items", async () => {
  const scope = createTenantSession("secret", "alpha", "editor");
  let headers = new Headers();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, options: RequestInit) => {
      headers = new Headers(options.headers);
      return new Response(
        JSON.stringify({
          tenant: "alpha",
          items: [{ id: "one", tenant: "other" }],
        }),
      );
    }),
  );
  await expect(
    readAudit(
      scope,
      new URLSearchParams({ actor: "reviewer" }),
      new AbortController().signal,
    ),
  ).rejects.toThrow("tenant mismatch");
  expect(headers.get("X-Nines-Tenant")).toBe("alpha");
  expect(headers.get("Authorization")).toBe("Bearer secret");
  scope.close();
});
test("frozen decision readback rejects a different record identity", async () => {
  const scope = createTenantSession("", "alpha", "owner");
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ tenant: "alpha", id: "another" })),
    ),
  );
  await expect(
    readAuditRecord(scope, "one", new AbortController().signal),
  ).rejects.toThrow("scope mismatch");
  scope.close();
});
test("archive receipt reads reject another record even within the same tenant", async () => {
  const scope = createTenantSession("", "alpha", "owner");
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({ tenant: "alpha", recordId: "other", enabled: true }),
        ),
    ),
  );
  const { readArchiveStatus } = await import("./audit");
  await expect(
    readArchiveStatus(scope, new AbortController().signal, "one"),
  ).rejects.toThrow("scope mismatch");
  scope.close();
});
