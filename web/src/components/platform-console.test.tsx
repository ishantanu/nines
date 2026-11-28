// @vitest-environment jsdom
import { typeInto } from "@/test-support/fields";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { PlatformConsole } from "./platform-console";
let host: HTMLDivElement, root: Root;
const refresh = vi.fn();
const overview = {
  tenants: [
    {
      id: "alpha",
      name: "Alpha",
      sourceKind: "prometheus",
      connections: 1,
      systemOne: {
        provider: "typesafe",
        model: "jev-1.14.0",
        contractDigest: "contract",
      },
      managedBy: "configuration",
    },
    {
      id: "beta",
      name: "Beta",
      sourceKind: "manual",
      connections: 1,
      managedBy: "console",
      state: "retired",
      version: 3,
    },
  ],
  providers: [{ id: "local", name: "Local", type: "local" }],
  administrators: [{ provider: "local", subject: "admin" }],
  auditRetentionDays: 90,
  tenantProvisioning: "platform console",
};
const auth = {
  enabled: true,
  authenticated: true,
  csrfToken: "platform-csrf",
  actor: {
    id: "u-admin",
    name: "Administrator",
    kind: "user",
    platformAdmin: true,
  },
  providers: overview.providers.map((p) => ({ ...p, type: "local" as const })),
};
const calls: { url: string; init?: RequestInit }[] = [];
beforeEach(() => {
  calls.length = 0;
  refresh.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      let result: unknown = { ok: true };
      if (url.endsWith("/users/create"))
        result = {
          id: "u-new",
          provider: "local",
          subject: "new-user",
          name: "New user",
          version: 1,
          setupLink: {
            url: "http://127.0.0.1/setup#user-token",
            expiresAt: "2030-01-01T00:00:00Z",
          },
        };
      if (url.endsWith("/tenants/create"))
        result = {
          id: "payments",
          name: "Payments",
          state: "active",
          version: 1,
          setupLink: {
            url: "http://127.0.0.1/setup#synthetic-token",
            expiresAt: "2030-01-01T00:00:00Z",
          },
        };
      if (url.endsWith("/overview")) result = overview;
      else if (url.endsWith("/users"))
        result = {
          items: [
            {
              user: {
                id: "u-alice",
                provider: "local",
                subject: "alice",
                name: "Alice",
                version: 1,
              },
              local: true,
              platformAdmin: false,
              memberships: [
                {
                  tenant: "alpha",
                  member: { role: "owner", effectiveRole: "owner", version: 2 },
                },
              ],
              effectiveGrants: { alpha: "owner" },
            },
          ],
          nextCursor: "",
        };
      else if (url.endsWith("/audit"))
        result = {
          items: [
            {
              id: "audit-id",
              at: "2026-10-26T00:00:00Z",
              actorId: "u-admin",
              actorName: "Administrator",
              actorProvider: "local",
              action: "systemone.policy",
              resource: "alpha",
              reason: "Bound provider usage",
              outcome: "succeeded",
              details: { enabled: false },
            },
          ],
          nextCursor: "",
        };
      else if (url.includes("/system-one"))
        result = {
          tenant: "alpha",
          configured: true,
          route: { provider: "typesafe", model: "jev-1.14.0" },
          policy: { version: 3, enabled: true, dailyRequestLimit: 100 },
          reservedRequests: 4,
          date: "2026-10-26",
        };
      return new Response(JSON.stringify(result), { status: 200 });
    }),
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
async function render(page: "tenants" | "users" | "audit") {
  await act(async () =>
    root.render(
      <PlatformConsole
        page={page}
        auth={auth}
        tenants={[]}
        onSignOut={() => {}}
        onRefreshAccess={refresh}
      />,
    ),
  );
}
function button(text: string) {
  return [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => b.textContent === text,
  )!;
}
async function field(id: string, value: string) {
  const input = document.getElementById(id) as
    HTMLInputElement | HTMLTextAreaElement;
  await typeInto(input, value);
}
test("tenant inventory has independent administration navigation and does not send tenant credentials", async () => {
  await render("tenants");
  expect(host.querySelector("h2")?.textContent).toBe("All tenants");
  expect(host.textContent).toContain("Configured");
  expect(document.title).toBe("Tenants | Platform administration | Nines");
  expect(host.textContent).toContain("does not grant access");
  expect(host.querySelector('a[href="/admin/users"]')).toBeTruthy();
  expect(
    calls.every(
      (c) =>
        !new Headers(c.init?.headers).has("X-Nines-Tenant") &&
        !new Headers(c.init?.headers).has("Authorization"),
    ),
  ).toBe(true);
  expect(calls.some((c) => c.init?.method === "POST")).toBe(false);
});
test("account creation sends no password and shows the setup link", async () => {
  await render("users");
  await act(async () => button("Create user").click());
  // The reason is optional; nothing blocks saving on it.
  expect(button("Save change").getAttribute("aria-disabled")).not.toBe("true");
  await field("platform-username", "new-user");
  await field("platform-name", "New user");
  expect(document.getElementById("platform-password")).toBeNull();
  await field("platform-reason", "Provision the tenant team");
  await act(async () => button("Save change").click());
  const request = calls.find((c) => c.url.endsWith("/users/create"))!;
  expect(request).toBeTruthy();
  expect(new Headers(request.init?.headers).get("X-CSRF-Token")).toBe(
    "platform-csrf",
  );
  expect(JSON.parse(String(request.init?.body))).toMatchObject({
    reason: "Provision the tenant team",
    tenant: "alpha",
    user: { provider: "local", username: "new-user", role: "viewer" },
  });
  expect(JSON.parse(String(request.init?.body)).user.password).toBeUndefined();
  expect(
    (document.getElementById("setup-link") as HTMLInputElement).value,
  ).toBe("http://127.0.0.1/setup#user-token");
  expect(refresh).toHaveBeenCalledOnce();
});
test("editing an existing membership preserves its current role until explicitly changed", async () => {
  await render("users");
  await act(async () => button("Tenant access").click());
  expect(document.getElementById("platform-role")?.textContent).toBe("owner");
});
test("Jev policy edits preserve the exact revision and never accept a secret value", async () => {
  await render("tenants");
  await act(async () => button("Jev policy").click());
  expect(document.body.textContent).toContain("4 requests reserved");
  expect(document.body.textContent).toContain(
    "Credentials and model routes remain server-owned",
  );
  await field("platform-daily-limit", "50");
  await field("platform-reason", "Bound synthetic model usage");
  await act(async () => button("Save change").click());
  const request = calls.find((c) => c.url.endsWith("/system-one/save"))!;
  expect(JSON.parse(String(request.init?.body))).toEqual({
    tenant: "alpha",
    reason: "Bound synthetic model usage",
    policy: { version: 3, enabled: true, dailyRequestLimit: 50 },
  });
});

test("platform audit shows a readable actor and retains its stable identifier in details", async () => {
  await render("audit");
  const article = host.querySelector("article")!;
  expect(article).toBeTruthy();
  expect(article.textContent).toContain("Administrator");
  expect(article.textContent).toContain("(local)");
  expect(article.querySelector("pre")?.textContent).toContain("u-admin");
});

function submit() {
  return document.querySelector<HTMLButtonElement>(
    '[role="dialog"] button[type="submit"]',
  )!;
}
test("configured tenants cannot be changed; console tenants show their state", async () => {
  await render("tenants");
  const rows = [...host.querySelectorAll("tbody tr")];
  expect(rows[0].textContent).toContain("Configured");
  const labels = (row: Element) =>
    [...row.querySelectorAll("button")].map((b) => b.textContent);
  expect(labels(rows[0])).toEqual(["Jev policy"]);
  expect(rows[1].textContent).toContain("Retired");
  expect(labels(rows[1])).toEqual(["Jev policy", "Delete"]);
});
test("creating a tenant gives its new owner a setup link, never a password", async () => {
  await render("tenants");
  await act(async () => button("Create tenant").click());
  expect(submit().getAttribute("aria-disabled")).toBe("true");
  await field("tenant-id", "Payments");
  expect((document.getElementById("tenant-id") as HTMLInputElement).value).toBe(
    "payments",
  );
  await field("tenant-name", "Payments");
  await field("tenant-username", "pat");
  await field("tenant-owner-name", "Pat");
  expect(document.getElementById("tenant-password")).toBeNull();
  await field("tenant-reason", "Onboard the payments team");
  expect(submit().getAttribute("aria-disabled")).not.toBe("true");
  await act(async () => submit().click());
  const request = calls.find((c) => c.url.endsWith("/tenants/create"))!;
  expect(new Headers(request.init?.headers).get("X-CSRF-Token")).toBe(
    "platform-csrf",
  );
  expect(JSON.parse(String(request.init?.body))).toEqual({
    reason: "Onboard the payments team",
    newTenant: {
      id: "payments",
      name: "Payments",
      ownerUser: {
        provider: "local",
        username: "pat",
        name: "Pat",
      },
    },
  });
  // The link is shown once, before the dialog closes.
  const link = document.getElementById("setup-link") as HTMLInputElement;
  expect(link.value).toBe("http://127.0.0.1/setup#synthetic-token");
  expect(document.body.textContent).toContain("not shown again");
  expect(refresh).not.toHaveBeenCalled();
  await act(async () => button("Done").click());
  expect(document.getElementById("setup-link")).toBeNull();
  expect(refresh).toHaveBeenCalledOnce();
});
test("deleting a tenant needs its ID typed and sends the reviewed version", async () => {
  await render("tenants");
  await act(async () => button("Delete").click());
  expect(document.body.textContent).toContain("cannot be undone");
  await field("tenant-reason", "Team merged");
  await field("tenant-confirm", "Beta");
  expect(submit().getAttribute("aria-disabled")).toBe("true");
  await field("tenant-confirm", "beta");
  await act(async () => submit().click());
  const request = calls.find((c) => c.url.endsWith("/tenants/delete"))!;
  expect(JSON.parse(String(request.init?.body))).toEqual({
    reason: "Team merged",
    expectedVersion: 3,
    tenant: "beta",
    confirm: "beta",
  });
});

test("a new setup link for a local user replaces the old one", async () => {
  await render("users");
  await act(async () => button("Setup link").click());
  expect(document.body.textContent).toContain("Any earlier link stops working");
  await field("platform-reason", "Lost their link");
  await act(async () => button("Save change").click());
  const request = calls.find((c) => c.url.endsWith("/users/setup-link"))!;
  expect(JSON.parse(String(request.init?.body))).toEqual({
    reason: "Lost their link",
    userId: "u-alice",
  });
});
