// @vitest-environment jsdom
import { act, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { replayFixture } from "./lib/replay-test-fixtures";
import { navigateTo, workspacePath } from "./lib/navigation";

const recorder = vi.hoisted(() => ({ stop: vi.fn(), record: vi.fn() }));
vi.mock("@rrweb/record", () => ({ record: recorder.record }));
vi.mock("./components/administration", () => ({
  AccessControl: () => <h1 data-private>Access control</h1>,
  Connections: () => <h1 data-private>Data sources</h1>,
  Clients: () => <h1 data-private>API and clients</h1>,
}));
vi.mock("./components/audit-log", () => ({
  AuditLog: () => <h1 data-private>Audit log</h1>,
}));
vi.mock("./components/login", () => ({ Login: () => <h1>Sign in</h1> }));
let allowed: { id: string; name: string; role: "owner" | "viewer" }[];
let authState: Record<string, unknown>;
let deniedReplayStatus = 0;
vi.mock("./components/objectives", () => ({
  ObjectiveDetail: () => <h1>API availability</h1>,
  Objectives: () => <h1>Objectives</h1>,
}));
let root: Root, host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  recorder.stop.mockClear();
  recorder.record.mockReset();
  recorder.record.mockImplementation((options) => {
    replayFixture().forEach(options.emit);
    return recorder.stop;
  });
  allowed = [{ id: "alpha", name: "Alpha", role: "owner" }];
  authState = { enabled: false, authenticated: false };
  deniedReplayStatus = 0;
  window.localStorage.clear();
  window.sessionStorage.clear();
  window.history.replaceState(null, "", workspacePath("alpha", "replays"));
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, options?: RequestInit) => {
      if (path === "/api/session-replays" && deniedReplayStatus) {
        const status = deniedReplayStatus;
        deniedReplayStatus = 0;
        return new Response(JSON.stringify({ error: "Access changed" }), {
          status,
        });
      }
      const tenant =
        new Headers(options?.headers).get("X-Nines-Tenant") ?? "alpha";
      const value =
        path === "/auth/session"
          ? authState
          : path === "/api/tenants"
            ? allowed
            : path === "/api/workspace"
              ? {
                  mode: "server",
                  tenant: { id: tenant, name: tenant },
                  role:
                    allowed.find((item) => item.id === tenant)?.role ??
                    "viewer",
                  source: { kind: "manual", configured: false },
                }
              : path === "/api/slos"
                ? [
                    {
                      slo: {
                        id: "api",
                        name: "API availability",
                        project: "production",
                        service: "api",
                        target: 0.999,
                        windowSeconds: 86400,
                        stepSeconds: 60,
                      },
                      status: {
                        reliability: null,
                        coverage: 0,
                        alert: { phase: "inactive" },
                      },
                    },
                  ]
                : path === "/api/session-replays"
                  ? { tenant, items: [] }
                  : [];
      return new Response(JSON.stringify(value), { status: 200 });
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
  for (let i = 0; i < 100; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (check()) return;
  }
  throw Error("UI did not settle: " + host.textContent);
}
function button(text: string) {
  return [...host.querySelectorAll("button")].find(
    (node) => node.textContent === text,
  )!;
}
test("the recording indicator and stop action remain available on SLO details and missing-resource views", async () => {
  await act(async () =>
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    ),
  );
  await until(() => !!host.querySelector("summary"));
  await act(async () => host.querySelector("summary")!.click());
  await act(async () =>
    host
      .querySelector<HTMLInputElement>('label input[type="checkbox"]')!
      .click(),
  );
  await act(async () => button("Start recording").click());
  await until(() =>
    host.textContent!.includes(
      "Recording this tab. Private data is masked.",
    ),
  );
  await act(async () =>
    navigateTo(workspacePath("alpha", "objectives", "api")),
  );
  expect(host.querySelector("h1")?.textContent).toBe("API availability");
  expect(host.textContent).toContain(
    "Recording this tab. Private data is masked.",
  );
  expect(button("Stop recording")).toBeDefined();
  await act(async () =>
    navigateTo(workspacePath("alpha", "objectives", "missing")),
  );
  expect(host.querySelector("h1")?.textContent).toBe("Resource not found");
  expect(button("Stop recording")).toBeDefined();
  await act(async () => button("Stop recording").click());
  expect(window.location.pathname).toBe(workspacePath("alpha", "replays"));
  expect(host.textContent).toContain("A masked recording is ready to save.");
  expect(recorder.stop).toHaveBeenCalledOnce();
});

async function recordTab() {
  await act(async () =>
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    ),
  );
  await until(() => !!host.querySelector("summary"));
  await act(async () => host.querySelector("summary")!.click());
  await act(async () =>
    host
      .querySelector<HTMLInputElement>('label input[type="checkbox"]')!
      .click(),
  );
  await act(async () => button("Start recording").click());
  await until(() =>
    host.textContent!.includes(
      "Recording this tab. Private data is masked.",
    ),
  );
}
test("sensitive pages pause capture and an eligible page resumes a fresh segment", async () => {
  await recordTab();
  for (const page of ["access", "connections", "clients", "audit"] as const) {
    await act(async () => navigateTo(workspacePath("alpha", page)));
    await until(() =>
      host.textContent!.includes("Session recording paused on this screen."),
    );
    expect(recorder.stop).toHaveBeenCalledTimes(1);
  }
  await act(async () =>
    navigateTo(workspacePath("alpha", "objectives", "api")),
  );
  await until(() =>
    host.textContent!.includes(
      "Recording this tab. Private data is masked.",
    ),
  );
  expect(recorder.record).toHaveBeenCalledTimes(2);
});
test("switching tenants discards local capture and does not carry consent back", async () => {
  allowed.push({ id: "beta", name: "Beta", role: "owner" });
  await recordTab();
  await act(async () => navigateTo(workspacePath("beta", "replays")));
  await until(
    () =>
      window.location.pathname === workspacePath("beta", "replays") &&
      !!button("Start recording"),
  );
  expect(recorder.stop).toHaveBeenCalledOnce();
  expect(host.textContent).not.toContain(
    "A masked recording is ready to save.",
  );
  expect(button("Start recording").disabled).toBe(true);
  await act(async () => navigateTo(workspacePath("alpha", "replays")));
  await until(() => !!button("Start recording"));
  expect(button("Start recording").disabled).toBe(true);
});
test("a viewer cannot consent to or start capture", async () => {
  allowed[0].role = "viewer";
  await act(async () =>
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    ),
  );
  await until(() => !!host.querySelector("summary"));
  await act(async () => host.querySelector("summary")!.click());
  expect(
    host.querySelector<HTMLInputElement>('label input[type="checkbox"]')!
      .disabled,
  ).toBe(true);
  expect(button("Start recording").disabled).toBe(true);
  expect(recorder.record).not.toHaveBeenCalled();
});
test("session expiry discards capture after the API rejects access", async () => {
  authState = {
    enabled: true,
    authenticated: true,
    actor: { id: "user-1", name: "Synthetic" },
    csrfToken: "synthetic-csrf",
  };
  await recordTab();
  authState = { enabled: true, authenticated: false };
  deniedReplayStatus = 401;
  await act(async () =>
    host
      .querySelector<HTMLButtonElement>('[aria-label="Refresh recordings"]')!
      .click(),
  );
  await until(() => host.querySelector("h1")?.textContent === "Sign in");
  expect(recorder.stop).toHaveBeenCalledOnce();
  expect(host.textContent).not.toContain("Recording this tab");
});
test("a role downgrade discards capture and refreshes the read-only workspace", async () => {
  await recordTab();
  allowed[0].role = "viewer";
  deniedReplayStatus = 403;
  await act(async () =>
    host
      .querySelector<HTMLButtonElement>('[aria-label="Refresh recordings"]')!
      .click(),
  );
  await until(() => !!button("Start recording")?.disabled);
  expect(recorder.stop).toHaveBeenCalledOnce();
  expect(host.textContent).not.toContain(
    "A masked recording is ready to save.",
  );
});

test("sign-out discards capture immediately even while logout is pending", async () => {
  authState = {
    enabled: true,
    authenticated: true,
    actor: { id: "user-1", name: "Synthetic" },
    csrfToken: "synthetic-csrf",
  };
  const baseFetch = fetch;
  let resolveLogout!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn((path: RequestInfo | URL, options?: RequestInit) =>
      path === "/auth/logout"
        ? new Promise<Response>((resolve) => {
            resolveLogout = resolve;
          })
        : baseFetch(path, options),
    ),
  );
  await recordTab();
  await act(async () => button("Sign out").click());
  expect(recorder.stop).toHaveBeenCalledOnce();
  expect(host.textContent).not.toContain("Recording this tab. Private data");
  expect(host.textContent).not.toContain(
    "A masked recording is ready to save.",
  );
  authState = { enabled: true, authenticated: false };
  await act(async () => resolveLogout(new Response("{}", { status: 200 })));
  await until(() => host.querySelector("h1")?.textContent === "Sign in");
});
