// @vitest-environment jsdom
import { act, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import App from "./App";
import { navigateTo, workspacePath } from "./lib/navigation";

let root: Root, host: HTMLDivElement;
let authenticated = true;
let allowed = ["alpha", "beta"];
let mode = "server";
const requested: {
  path: string;
  tenant: string | null;
  signal?: AbortSignal | null;
}[] = [];
beforeEach(() => {
  authenticated = true;
  allowed = ["alpha", "beta"];
  mode = "server";
  requested.length = 0;
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  window.localStorage.clear();
  window.sessionStorage.clear();
  window.history.replaceState(null, "", "/");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      const tenant = new Headers(init?.headers).get("X-Nines-Tenant");
      requested.push({ path, tenant, signal: init?.signal });
      const result =
        path === "/auth/session"
          ? {
              enabled: true,
              authenticated,
              csrfToken: "fixture-csrf",
              actor: authenticated
                ? { id: "fixture-user", name: "Test user" }
                : undefined,
              providers: [{ id: "local", type: "local", name: "Local" }],
            }
          : path === "/api/tenants"
            ? allowed.map((id) => ({ id, name: id, role: "viewer" }))
            : path === "/api/workspace"
              ? {
                  mode,
                  tenant: { id: tenant, name: tenant },
                  source: { kind: "manual", configured: false },
                  role: "viewer",
                  journeysConfigured: true,
                  workflowsConfigured: false,
                }
              : [];
      return new Response(JSON.stringify(result), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
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
async function until(check: () => boolean) {
  for (let tries = 0; tries < 100; tries++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
    if (check()) return;
  }
  throw Error("UI did not settle: " + host.textContent);
}
async function render() {
  await act(async () =>
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    ),
  );
}
async function travel(direction: "back" | "forward") {
  await act(async () => {
    const moved = new Promise<void>((resolve) =>
      window.addEventListener("popstate", () => resolve(), { once: true }),
    );
    window.history[direction]();
    await moved;
  });
}
test("portal renders actual page headings when native history returns to a page or missing SLO", async () => {
  await render();
  await until(() => !!host.querySelector('a[aria-current="page"]'));
  expect(window.location.pathname).toBe(workspacePath("alpha"));
  const journeys = host.querySelector<HTMLAnchorElement>(
    'a[href="/tenants/alpha/critical-user-journeys"]',
  )!;
  await act(async () => journeys.click());
  expect(host.querySelector("h1")!.textContent).toBe("Critical user journeys");
  await act(async () =>
    navigateTo(workspacePath("alpha", "objectives", "missing-slo")),
  );
  expect(host.querySelector("h1")!.textContent).toBe("Resource not found");
  await travel("back");
  expect(host.querySelector("h1")!.textContent).toBe("Critical user journeys");
  await travel("forward");
  expect(host.querySelector("h1")!.textContent).toBe("Resource not found");
});
test("tenant history closes the old scope and cannot mount a tenant the user cannot access", async () => {
  window.history.replaceState(null, "", workspacePath("alpha", "journeys"));
  await render();
  await until(
    () => host.querySelector("h1")?.textContent === "Critical user journeys",
  );
  await act(async () => navigateTo(workspacePath("beta", "journeys")));
  await until(() =>
    requested.some((r) => r.path === "/api/slos" && r.tenant === "beta"),
  );
  expect(
    requested.find((r) => r.path === "/api/slos" && r.tenant === "alpha")!
      .signal?.aborted,
  ).toBe(true);
  await travel("back");
  await until(
    () => host.querySelector("h1")?.textContent === "Critical user journeys",
  );
  expect(window.location.pathname).toBe(workspacePath("alpha", "journeys"));
  await act(async () => navigateTo(workspacePath("outsider", "objectives")));
  expect(host.textContent).toContain(
    "You do not have access to the tenant in this link",
  );
  expect(requested.some((r) => r.tenant === "outsider")).toBe(false);
  await travel("back");
  await until(
    () => host.querySelector("h1")?.textContent === "Critical user journeys",
  );
  expect(
    requested
      .filter((r) => r.path === "/api/slos" && r.tenant === "alpha")
      .at(-1)?.signal?.aborted,
  ).toBe(false);
});
test("unauthenticated deep links use /login then return to the intended SLO after sign-in", async () => {
  authenticated = false;
  const intended = workspacePath("beta", "objectives", "missing-slo");
  window.history.replaceState(null, "", intended);
  await render();
  await until(() => !!host.querySelector('input[type="password"]'));
  expect(window.location.pathname).toBe("/login");
  expect(new URLSearchParams(window.location.search).get("returnTo")).toBe(
    intended,
  );
  expect(host.querySelector("main")!.hasAttribute("data-replay-block")).toBe(
    true,
  );
  expect(requested.some((r) => r.path === "/api/tenants")).toBe(false);
  authenticated = true;
  await act(async () =>
    window.dispatchEvent(new Event("nines-access-changed")),
  );
  await until(
    () => host.querySelector("h1")?.textContent === "Resource not found",
  );
  expect(window.location.pathname).toBe(intended);
  expect(window.sessionStorage.getItem("nines-login-return")).toBeNull();
  expect(
    [...Array(window.localStorage.length)].map((_, i) =>
      window.localStorage.key(i),
    ),
  ).toEqual(["nines-theme"]);
});
test("demo has a canonical login page without running capture on the login screen", async () => {
  mode = "demo";
  await render();
  await until(() => !!host.querySelector("#demo-password"));
  await until(() => window.location.pathname === "/login");
  expect(host.querySelector("main")!.hasAttribute("data-replay-block")).toBe(
    true,
  );
  expect(host.textContent).toContain("Demo credentials");
  expect(document.title).toBe("Sign in | Nines");
});

test("a revoked browser session returns to login and browser Back cannot reopen tenant data", async () => {
  window.history.replaceState(null, "", workspacePath("alpha", "journeys"));
  await render();
  await until(
    () => host.querySelector("h1")?.textContent === "Critical user journeys",
  );
  await act(async () => navigateTo(workspacePath("alpha", "objectives")));
  authenticated = false;
  await act(async () =>
    window.dispatchEvent(new Event("nines-access-changed")),
  );
  await until(() => !!host.querySelector('input[type="password"]'));
  expect(window.location.pathname).toBe("/login");
  const reads = requested.filter((r) => r.path === "/api/slos").length;
  await travel("back");
  await until(() => window.location.pathname === "/login");
  expect(host.querySelector("h1")!.textContent).toBe("Sign in");
  expect(requested.filter((r) => r.path === "/api/slos").length).toBe(reads);
});
