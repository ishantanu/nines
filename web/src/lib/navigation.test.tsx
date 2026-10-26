// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import {
  pagePaths,
  parseRoute,
  routeFromLocation,
  workspacePath,
  safeReturnPath,
  useRoute,
  useResourceRoute,
  navigateTo,
  showLogin,
  loginReturnPath,
  clearLoginReturn,
  followLink,
} from "./navigation";

let root: Root;
let host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  window.history.replaceState(null, "", "/");
  window.sessionStorage.clear();
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
function Probe() {
  const route = useRoute();
  const [selected, select] = useResourceRoute("alpha", "replays");
  return (
    <>
      <output>{JSON.stringify(route)}</output>
      <span>{selected}</span>
      <a
        href={workspacePath("alpha", "replays", "second")}
        onClick={(e) => followLink(e, e.currentTarget.pathname)}
      >
        Open second recording
      </a>
      <button onClick={() => select("third")}>Select third</button>
    </>
  );
}
function current() {
  return JSON.parse(host.querySelector("output")!.textContent!);
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
test("all page URLs round-trip, including UUIDv4 detail identifiers", () => {
  for (const page of Object.keys(pagePaths) as (keyof typeof pagePaths)[]) {
    expect(parseRoute(workspacePath("production", page))).toEqual({
      kind: "workspace",
      tenant: "production",
      page,
      id: "",
    });
  }
  const uuid = crypto.randomUUID();
  expect(uuid).toMatch(
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  );
  expect(parseRoute(workspacePath("alpha", "replays", uuid))).toMatchObject({
    id: uuid,
  });
  expect(parseRoute("/login/")).toEqual({ kind: "login" });
});
test("malformed IDs, traversal, foreign URLs and unsupported detail routes are rejected", () => {
  for (const path of [
    "https://evil.example/",
    "//evil.example",
    "/tenants/alpha/slos/..",
    "/tenants/alpha/slos/%2e%2e",
    "/tenants/alpha/slos/a%2fb",
    "/tenants/alpha/slos/a/b",
    "/tenants/alpha/slos//",
    "/tenants/alpha/home/detail",
    "/tenants/%61lpha/home",
    "/tenants/alpha/nope",
    "/tenants/alpha/replays/id?api_key=secret",
    "/tenants/alpha/replays/id#secret",
    "/tenants/alpha/replays/" + "x".repeat(64),
  ]) {
    expect(parseRoute(path).kind, path).toBe("not-found");
    expect(safeReturnPath(path), path).toBeNull();
  }
  expect(() => navigateTo("https://evil.example")).toThrow();
  expect(() => navigateTo("/login?token=secret")).toThrow();
  expect(() =>
    navigateTo("/login?returnTo=https%3A%2F%2Fevil.example"),
  ).toThrow();
});
test("legacy hashes resolve to the same resource and normalize without adding history", async () => {
  window.history.replaceState(
    null,
    "",
    "/#/tenants/alpha/investigations/incident-1",
  );
  const route = routeFromLocation(
    window.location.href.slice(window.location.origin.length),
  );
  expect(route).toMatchObject({
    tenant: "alpha",
    page: "investigations",
    id: "incident-1",
  });
  const before = window.history.length;
  await act(async () => root.render(<Probe />));
  await act(async () =>
    navigateTo(workspacePath("alpha", "investigations", "incident-1"), true),
  );
  expect(window.history.length).toBe(before);
  expect(window.location.hash).toBe("");
  expect(current()).toEqual(route);
});
test("native Back/Forward restores page, tenant and same-page resource selection", async () => {
  window.history.replaceState(
    null,
    "",
    workspacePath("alpha", "replays", "first"),
  );
  await act(async () => root.render(<Probe />));
  const before = window.history.length;
  await act(async () => host.querySelector("a")!.click());
  expect(current().id).toBe("second");
  expect(host.querySelector("span")!.textContent).toBe("second");
  expect(window.history.length).toBe(before + 1);
  await act(async () =>
    navigateTo(workspacePath("beta", "objectives", "latency")),
  );
  expect(current().tenant).toBe("beta");
  expect(host.querySelector("span")!.textContent).toBe("");
  await travel("back");
  expect(current().id).toBe("second");
  await travel("back");
  expect(current().id).toBe("first");
  await travel("forward");
  expect(current().id).toBe("second");
  await travel("forward");
  expect(current()).toMatchObject({
    tenant: "beta",
    page: "objectives",
    id: "latency",
  });
  // Opening the current page must not create duplicate Back entries.
  const length = window.history.length;
  await act(async () =>
    navigateTo(workspacePath("beta", "objectives", "latency")),
  );
  expect(window.history.length).toBe(length);
});
test("sign-in saves only a validated path and restores it after a same-tab SSO round trip", () => {
  const path = workspacePath("alpha", "objectives", "api-latency");
  window.history.replaceState(
    null,
    "",
    path + "?token=must-not-persist#credential",
  );
  const before = window.history.length;
  showLogin();
  expect(window.location.pathname).toBe("/login");
  expect(loginReturnPath()).toBe(path);
  expect(window.history.length).toBe(before);
  expect(window.location.href).not.toContain("must-not-persist");
  expect(window.sessionStorage.getItem("nines-login-return")).toBe(path);
  expect(window.localStorage.length).toBe(0);
  // OIDC returns to the app root. Browser storage carries only the resource path.
  window.history.replaceState(null, "", "/");
  expect(loginReturnPath()).toBe(path);
  clearLoginReturn();
  expect(loginReturnPath()).toBeNull();
});
test("an untrusted return URL or stored secret cannot become a login redirect", () => {
  window.sessionStorage.setItem(
    "nines-login-return",
    "https://evil.example/?token=secret",
  );
  window.history.replaceState(
    null,
    "",
    "/login?returnTo=" + encodeURIComponent("//evil.example/?token=secret"),
  );
  expect(loginReturnPath()).toBeNull();
  showLogin();
  expect(window.location.pathname + window.location.search).toBe("/login");
  expect(window.sessionStorage.getItem("nines-login-return")).toBeNull();
});
test("denied browser storage does not prevent login or a validated query return", () => {
  const denied = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementation(() => {
      throw Error("denied");
    });
  window.history.replaceState(null, "", workspacePath("alpha", "replays"));
  expect(() => showLogin()).not.toThrow();
  expect(loginReturnPath()).toBe(workspacePath("alpha", "replays"));
  denied.mockRestore();
});
test("modified link clicks preserve native open-in-new-tab behavior", async () => {
  await act(async () => root.render(<Probe />));
  const event = new MouseEvent("click", {
    bubbles: true,
    cancelable: true,
    metaKey: true,
  });
  // Avoid jsdom attempting a real navigation after checking our own handler.
  host.addEventListener(
    "click",
    (e) => {
      expect(e.defaultPrevented).toBe(false);
      e.preventDefault();
    },
    { once: true },
  );
  await act(async () => host.querySelector("a")!.dispatchEvent(event));
  expect(window.location.pathname).toBe("/");
});

test("telemetry connection routes are canonical and old data-source bookmarks remain usable", () => {
  expect(workspacePath("alpha", "connections")).toBe(
    "/tenants/alpha/telemetry-connections",
  );
  expect(parseRoute("/tenants/alpha/data-sources")).toEqual(
    parseRoute("/tenants/alpha/telemetry-connections"),
  );
  expect(safeReturnPath("/tenants/alpha/data-sources")).toBe(
    "/tenants/alpha/telemetry-connections",
  );
});

test("platform paths use a bounded independent route vocabulary", () => {
  for (const page of ["tenants", "users", "audit"] as const) {
    expect(parseRoute("/admin/" + page)).toEqual({ kind: "platform", page });
    expect(safeReturnPath("/admin/" + page)).toBe("/admin/" + page);
  }
  expect(parseRoute("/admin")).toEqual({ kind: "platform", page: "tenants" });
  for (const path of [
    "/admin/unknown",
    "/admin/users/extra",
    "/admin/%2e%2e",
    "//admin/users",
    "https://example.test/admin/users",
  ]) {
    expect(parseRoute(path).kind).toBe("not-found");
    expect(safeReturnPath(path)).toBeNull();
  }
});
