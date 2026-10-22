import { afterEach, describe, it, expect, vi } from "vitest";
import {
  api,
  createTenantSession,
  discoverTenants,
  loadData,
  healthOf,
  stepLayers,
  type Snapshot,
  type Step,
} from "./api";
afterEach(() => vi.unstubAllGlobals());
describe("tenant request boundary", () => {
  it("scopes reads and writes and prevents per-request tenant or token overrides", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetcher);
    const scope = createTenantSession("trusted-token", "alpha", "owner");
    await api("/api/journeys/apply", scope, {
      method: "POST",
      body: "{}",
      headers: new Headers({
        "X-Nines-Tenant": "other",
        Authorization: "Bearer other",
        "X-Trace": "test",
      }),
    });
    const options = fetcher.mock.calls[0][1] as RequestInit;
    const headers = new Headers(options.headers);
    expect(headers.get("X-Nines-Tenant")).toBe("alpha");
    expect(headers.get("Authorization")).toBe("Bearer trusted-token");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Trace")).toBe("test");
  });
  it("preserves raw YAML and revision headers while binding the tenant", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetcher);
    const body =
      "apiVersion: nines.dev/v1alpha1\nkind: ServiceLevelObjective\n";
    await api(
      "/api/resources/apply",
      createTenantSession("token", "alpha", "owner"),
      {
        method: "POST",
        headers: { "Content-Type": "application/yaml", "If-Match": '"0"' },
        body,
      },
    );
    const options = fetcher.mock.calls[0][1] as RequestInit;
    const headers = new Headers(options.headers);
    expect(headers.get("Content-Type")).toBe("application/yaml");
    expect(headers.get("If-Match")).toBe('"0"');
    expect(headers.get("X-Nines-Tenant")).toBe("alpha");
    expect(options.body).toBe(body);
  });
  it("discovers tenants using authentication without an existing tenant selector", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        new Response('[{"id":"alpha","name":"Alpha","role":"viewer"}]'),
      );
    vi.stubGlobal("fetch", fetcher);
    const tenants = await discoverTenants(
      "read-token",
      new AbortController().signal,
    );
    expect(tenants[0].role).toBe("viewer");
    const headers = new Headers(fetcher.mock.calls[0][1].headers);
    expect(headers.get("Authorization")).toBe("Bearer read-token");
    expect(headers.has("X-Nines-Tenant")).toBe(false);
  });
  it("passes the selected tenant to every parallel workspace request", async () => {
    const fetcher = vi
      .fn()
      .mockImplementation(() => Promise.resolve(new Response("[]")));
    vi.stubGlobal("fetch", fetcher);
    await loadData(
      createTenantSession("", "beta", "agent"),
      new AbortController().signal,
    );
    expect(fetcher).toHaveBeenCalledTimes(4);
    expect(
      fetcher.mock.calls.every(
        (call) => new Headers(call[1].headers).get("X-Nines-Tenant") === "beta",
      ),
    ).toBe(true);
  });
  it("cancels old reads and mutations on switch even when a fetch implementation resolves after abort", async () => {
    let resolveOld!: (value: Response) => void;
    const fetcher = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveOld = resolve;
          }),
      )
      .mockResolvedValueOnce(new Response('{"tenant":"beta"}'));
    vi.stubGlobal("fetch", fetcher);
    const oldScope = createTenantSession("token", "alpha", "owner");
    const oldRequest = api("/api/workflows", oldScope, {
      method: "POST",
      body: "{}",
    });
    const rejected = expect(oldRequest).rejects.toMatchObject({
      name: "AbortError",
    });
    oldScope.close();
    const next = createTenantSession("token", "beta", "owner");
    expect(await api("/api/workflows", next)).toEqual({ tenant: "beta" });
    resolveOld(new Response('{"tenant":"alpha"}'));
    await rejected;
    expect(fetcher.mock.calls[0][1].signal.aborted).toBe(true);
    await expect(api("/api/slos", oldScope)).rejects.toMatchObject({
      name: "AbortError",
    });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it("rejects late response bodies and propagates per-view cancellation", async () => {
    let resolveBody!: (value: unknown) => void;
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        new Promise((resolve) => {
          resolveBody = resolve;
        }),
    });
    vi.stubGlobal("fetch", fetcher);
    const scope = createTenantSession("", "alpha", "owner");
    const view = new AbortController();
    const pending = api("/api/journeys/example/runs", scope, {
      signal: view.signal,
    });
    await Promise.resolve();
    const rejected = expect(pending).rejects.toMatchObject({
      name: "AbortError",
    });
    view.abort();
    resolveBody([]);
    await rejected;
    expect(scope.signal.aborted).toBe(false);
  });
});
describe("objective health", () => {
  const healthy: Snapshot = {
    sloId: "s",
    at: "",
    from: "",
    good: 100,
    total: 100,
    bad: 0,
    coverage: 1,
    reliability: 1,
    budgetRemainingFraction: 1,
    burnRate: 0,
    dataStatus: "complete",
    alert: { phase: "inactive" },
  };
  it("keeps missing traffic and partial coverage unknown", () => {
    expect(healthOf({ ...healthy, reliability: null })).toBe("unknown");
    expect(healthOf({ ...healthy, coverage: 0.99 })).toBe("unknown");
  });
  it("surfaces budget exhaustion and active alerts even with missing coverage", () => {
    expect(healthOf({ ...healthy, budgetRemainingFraction: 0 })).toBe(
      "degraded",
    );
    expect(
      healthOf({ ...healthy, coverage: 0, alert: { phase: "firing" } }),
    ).toBe("degraded");
  });
  it("marks excessive burn as at risk", () =>
    expect(healthOf({ ...healthy, burnRate: 2 })).toBe("at_risk"));
});
it("orders a branched journey by dependencies rather than YAML position", () => {
  const steps: Step[] = [
    {
      id: "finish",
      name: "Finish",
      actor: "service",
      dependsOn: ["left", "right"],
      sloRefs: [],
    },
    {
      id: "right",
      name: "Right",
      actor: "agent",
      dependsOn: ["start"],
      sloRefs: [],
    },
    { id: "start", name: "Start", actor: "user", sloRefs: [] },
    {
      id: "left",
      name: "Left",
      actor: "human",
      dependsOn: ["start"],
      sloRefs: [],
    },
  ];
  expect(stepLayers(steps).map((layer) => layer.map((s) => s.id))).toEqual([
    ["start"],
    ["right", "left"],
    ["finish"],
  ]);
});

describe("browser session scope", () => {
  it("sends same-origin cookies and the immutable session CSRF token", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('{"ok":true}'));
    vi.stubGlobal("fetch", fetcher);
    const scope = createTenantSession("", "alpha", "editor", "trusted-csrf");
    await api("/api/resources/apply", scope, {
      method: "POST",
      headers: { "X-CSRF-Token": "forged-csrf", "X-Nines-Tenant": "beta" },
      body: "{}",
    });
    const options = fetcher.mock.calls[0][1];
    const headers = new Headers(options.headers);
    expect(options.credentials).toBe("same-origin");
    expect(headers.get("X-CSRF-Token")).toBe("trusted-csrf");
    expect(headers.get("X-Nines-Tenant")).toBe("alpha");
    expect(headers.has("Authorization")).toBe(false);
  });
  it("keeps a machine bearer identity separate from browser CSRF", async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('{"ok":true}'));
    vi.stubGlobal("fetch", fetcher);
    await api(
      "/api/resources/plan",
      createTenantSession("machine-token", "beta", "agent", "browser-csrf"),
      { method: "POST", body: "{}" },
    );
    const headers = new Headers(fetcher.mock.calls[0][1].headers);
    expect(headers.get("Authorization")).toBe("Bearer machine-token");
    expect(headers.has("X-CSRF-Token")).toBe(false);
  });
});

it("does not trigger a recursive authentication refresh when tenant discovery rejects an expired credential", async () => {
  const dispatchEvent = vi.fn();
  vi.stubGlobal("window", { dispatchEvent });
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response('{"error":"authentication required"}', { status: 401 }),
      ),
  );
  await expect(
    discoverTenants("expired-token", new AbortController().signal),
  ).rejects.toMatchObject({ status: 401 });
  expect(dispatchEvent).not.toHaveBeenCalled();
});
