// @vitest-environment jsdom
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
    },
  ],
  providers: [{ id: "local", name: "Local", type: "local" }],
  administrators: [{ provider: "local", subject: "admin" }],
  auditRetentionDays: 90,
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
  await act(async () => {
    const proto =
      input instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
test("tenant inventory has independent administration navigation and does not send tenant credentials", async () => {
  await render("tenants");
  expect(host.textContent).toContain("Configured tenants");
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
test("account creation requires a reason, transmits CSRF and clears the password", async () => {
  await render("users");
  await act(async () => button("Create user").click());
  expect(button("Save change").disabled).toBe(true);
  await field("platform-username", "new-user");
  await field("platform-name", "New user");
  await field("platform-password", "test-private-password");
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
  expect(document.getElementById("platform-password")).toBeNull();
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
