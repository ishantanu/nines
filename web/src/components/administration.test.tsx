// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Connections } from "./administration";
import { createTenantSession } from "@/lib/api";

let root: Root, host: HTMLDivElement;
let scope = createTenantSession("synthetic-key", "alpha", "owner");
let pending: Map<
  string,
  { resolve: (response: Response) => void; signal: AbortSignal }
>;
beforeEach(() => {
  scope = createTenantSession("synthetic-key", "alpha", "owner");
  pending = new Map();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn((path: string, init?: RequestInit) => {
      if (path === "/api/connections")
        return Promise.resolve(
          new Response(
            JSON.stringify({
              management: "server",
              connections: [
                { id: "newrelic-trial", kind: "newrelic", version: 1 },
                { id: "primary", kind: "mimir", version: 1 },
              ],
            }),
          ),
        );
      return new Promise<Response>((resolve) =>
        pending.set(path, { resolve, signal: init?.signal as AbortSignal }),
      );
    }),
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  scope.close();
  host.remove();
  vi.unstubAllGlobals();
});
async function render() {
  await act(async () => root.render(<Connections scope={scope} />));
}
function card(id: string) {
  return [...host.querySelectorAll("h2")].find(
    (h) => h.textContent === `Connection: ${id}`,
  )!.parentElement!.parentElement!;
}
function button(id: string) {
  return card(id).querySelector<HTMLButtonElement>("button")!;
}
async function resolve(id: string, ok = true) {
  await act(async () =>
    pending
      .get(`/api/connections/${id}/test`)!
      .resolve(
        new Response(
          JSON.stringify(
            ok
              ? { status: "reachable" }
              : { error: "Connection credentials rejected" },
          ),
          { status: ok ? 200 : 502 },
        ),
      ),
  );
}

test("only the tested card shows pending state and its result", async () => {
  await render();
  await act(async () => button("newrelic-trial").click());
  expect(button("newrelic-trial").disabled).toBe(true);
  expect(button("newrelic-trial").textContent).toContain("Testing");
  expect(button("primary").disabled).toBe(false);
  expect(button("primary").textContent).toBe("Test connection");
  await resolve("newrelic-trial");
  expect(
    card("newrelic-trial").querySelector('[role="status"]')?.textContent,
  ).toContain("newrelic-trial: configured endpoint responded");
  expect(card("primary").querySelector('[role="status"]')).toBeNull();
});

test("concurrent tests resolving out of order keep success and error on their own cards", async () => {
  await render();
  await act(async () => {
    button("newrelic-trial").click();
    button("primary").click();
  });
  await resolve("primary", false);
  expect(card("primary").textContent).toContain(
    "Connection credentials rejected",
  );
  expect(card("newrelic-trial").textContent).not.toContain(
    "Connection credentials rejected",
  );
  expect(button("newrelic-trial").disabled).toBe(true);
  await resolve("newrelic-trial");
  expect(card("newrelic-trial").textContent).toContain(
    "No samples were stored",
  );
  expect(card("primary").textContent).not.toContain("No samples were stored");
});

test("a tenant change aborts the old test and ignores a late response", async () => {
  await render();
  await act(async () => button("newrelic-trial").click());
  const request = pending.get("/api/connections/newrelic-trial/test")!;
  scope = createTenantSession("synthetic-other-key", "beta", "owner");
  await render();
  expect(request.signal.aborted).toBe(true);
  await resolve("newrelic-trial");
  expect(host.textContent).not.toContain("No samples were stored");
  expect(button("newrelic-trial").disabled).toBe(false);
});
