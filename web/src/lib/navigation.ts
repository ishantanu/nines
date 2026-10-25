import { useMemo, useSyncExternalStore } from "react";
import type { MouseEvent } from "react";

export const pagePaths = {
  home: "home",
  objectives: "slos",
  journeys: "critical-user-journeys",
  investigations: "reviews",
  intents: "intents",
  replays: "replays",
  maps: "service-dependencies",
  bindings: "measurements",
  agents: "agent-evidence",
  alerts: "alerts",
  workflows: "workflows",
  connections: "telemetry-connections",
  audit: "audit",
  access: "access-control",
  clients: "clients",
} as const;
export type Page = keyof typeof pagePaths;
export type WorkspaceRoute = {
  kind: "workspace";
  tenant: string;
  page: Page;
  id: string;
};
export type Route =
  WorkspaceRoute | { kind: "login" } | { kind: "root" } | { kind: "not-found" };
const identifier = /^[a-z0-9][a-z0-9-]{0,62}$/;
const detailPages: Page[] = [
  "objectives",
  "journeys",
  "agents",
  "investigations",
  "intents",
  "replays",
];
const changed = "nines-navigation";
const returnKey = "nines-login-return";

// Parse the original path, before URL normalization can hide traversal segments.
export function parseRoute(path: string): Route {
  if (path === "/") return { kind: "root" };
  if (path === "/login" || path === "/login/") return { kind: "login" };
  const parts = path.split("/");
  if (parts.at(-1) === "") parts.pop();
  if (
    parts.length < 4 ||
    parts.length > 5 ||
    parts[0] !== "" ||
    parts[1] !== "tenants"
  )
    return { kind: "not-found" };
  const [, , tenant, segment, id = ""] = parts;
  const page =
    (Object.keys(pagePaths) as Page[]).find((p) => pagePaths[p] === segment) ??
    (segment === "investigations"
      ? "investigations"
      : segment === "data-sources"
        ? "connections"
        : undefined);
  if (
    !identifier.test(tenant) ||
    !page ||
    (parts.length === 5 &&
      (!identifier.test(id) || !detailPages.includes(page)))
  )
    return { kind: "not-found" };
  return { kind: "workspace", tenant, page, id };
}
export function workspacePath(
  tenant: string,
  page: Page = "home",
  id = "",
): string {
  const path = `/tenants/${encodeURIComponent(tenant)}/${pagePaths[page]}${id ? "/" + encodeURIComponent(id) : ""}`;
  if (parseRoute(path).kind !== "workspace")
    throw Error("Invalid workspace route");
  return path;
}
export function routeFromLocation(value: string): Route {
  const [beforeHash, hash = ""] = value.split("#");
  if (hash.startsWith("/tenants/")) return parseRoute(hash);
  return parseRoute(beforeHash.split("?")[0] || "/");
}
export function safeReturnPath(value: string | null): string | null {
  if (!value || value.length > 512) return null;
  const route = parseRoute(value);
  return route.kind === "workspace"
    ? workspacePath(route.tenant, route.page, route.id)
    : null;
}
function snapshot() {
  if (typeof window === "undefined") return "/";
  return (
    (window.location.pathname || "/") +
    (window.location.search || "") +
    (window.location.hash || "")
  );
}
function subscribe(listener: () => void) {
  window.addEventListener("popstate", listener);
  window.addEventListener("hashchange", listener);
  window.addEventListener(changed, listener);
  return () => {
    window.removeEventListener("popstate", listener);
    window.removeEventListener("hashchange", listener);
    window.removeEventListener(changed, listener);
  };
}
export function useRoute() {
  const location = useSyncExternalStore(subscribe, snapshot, snapshot);
  return useMemo(() => routeFromLocation(location), [location]);
}
function validLoginPath(path: string) {
  if (path === "/login") return true;
  if (!path.startsWith("/login?")) return false;
  const query = new URLSearchParams(path.slice("/login?".length));
  if (
    [...query.keys()].some((key) => key !== "returnTo" && key !== "auth_error")
  )
    return false;
  return (
    query.getAll("returnTo").length <= 1 &&
    query.getAll("auth_error").length <= 1 &&
    (!query.has("returnTo") ||
      safeReturnPath(query.get("returnTo")) === query.get("returnTo")) &&
    (!query.has("auth_error") || query.get("auth_error") === "1")
  );
}
export function navigateTo(path: string, replace = false) {
  if (!validLoginPath(path) && safeReturnPath(path) !== path)
    throw Error("Invalid navigation destination");
  if (snapshot() === path) return;
  window.history[replace ? "replaceState" : "pushState"](null, "", path);
  window.dispatchEvent(new Event(changed));
  if (!replace) window.scrollTo?.({ top: 0, behavior: "instant" });
}
export function followLink(event: MouseEvent<HTMLAnchorElement>, path: string) {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  )
    return;
  event.preventDefault();
  navigateTo(path);
}
export function loginReturnPath(): string | null {
  const query = new URLSearchParams(window.location.search).get("returnTo");
  if (query !== null) return safeReturnPath(query);
  try {
    return safeReturnPath(window.sessionStorage.getItem(returnKey));
  } catch {
    return null;
  }
}
export function clearLoginReturn() {
  try {
    window.sessionStorage.removeItem(returnKey);
  } catch {
    /* Storage is optional. */
  }
}
export function showLogin() {
  const route = routeFromLocation(snapshot());
  const next =
    route.kind === "workspace"
      ? workspacePath(route.tenant, route.page, route.id)
      : loginReturnPath();
  try {
    if (next) window.sessionStorage.setItem(returnKey, next);
    else window.sessionStorage.removeItem(returnKey);
  } catch {
    /* The returnTo query still works without browser storage. */
  }
  const authError = new URLSearchParams(window.location.search).has(
    "auth_error",
  );
  const query = new URLSearchParams();
  if (next) query.set("returnTo", next);
  if (authError) query.set("auth_error", "1");
  navigateTo("/login" + (query.size ? "?" + query.toString() : ""), true);
}
export function useResourceRoute(
  tenant: string,
  page: Page,
): [string, (id: string) => void] {
  const route = useRoute();
  return [
    route.kind === "workspace" && route.tenant === tenant && route.page === page
      ? route.id
      : "",
    (id) => navigateTo(workspacePath(tenant, page, id)),
  ];
}
