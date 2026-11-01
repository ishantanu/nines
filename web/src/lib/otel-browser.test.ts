import { createServer } from "node:http";
import { afterEach, expect, test, vi } from "vitest";
import { setupBrowserTelemetry } from "./otel-browser";
afterEach(() => vi.unstubAllGlobals());
test("browser telemetry initializes without preventing API requests", async () => {
  const on = vi.fn();
  vi.stubGlobal("window", { addEventListener: on });
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response("{}", { status: 200 })),
  );
  const fetcher = setupBrowserTelemetry("http://127.0.0.1:54318");
  const r = await fetcher("/api/slos", {});
  expect(r.status).toBe(200);
  expect(on).toHaveBeenCalledWith("pagehide", expect.any(Function));
  const init = vi
    .mocked(fetch)
    .mock.calls.find(([path]) => path === "/api/slos")![1]!;
  expect(new Headers(init.headers).get("traceparent")).toMatch(
    /^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/,
  );
});

test("browser telemetry exports traces, metrics and logs without request secrets", async () => {
  const received = new Map<string, string>();
  const server = createServer((request, response) => {
    let body = "";
    request.setEncoding("utf8");
    request.on("data", (chunk) => {
      body += chunk;
    });
    request.on("end", () => {
      received.set(request.url!, body);
      response.setHeader("Content-Type", "application/json");
      response.end("{}");
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    if (!address || typeof address === "string")
      throw new Error("Missing fixture address");
    let flush: (() => void) | undefined;
    vi.stubGlobal("window", {
      addEventListener: (_event: string, callback: () => void) => {
        flush = callback;
      },
    });
    const realFetch = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
        String(input).startsWith("/api/")
          ? Promise.resolve(new Response("{}", { status: 200 }))
          : realFetch(input, init),
      ),
    );
    const fetcher = setupBrowserTelemetry(`http://127.0.0.1:${address.port}`);
    await fetcher("/api/slos/private-objective/context?token=fixture-secret", {
      headers: { Authorization: "Bearer private-api-credential" },
    });
    expect(flush).toBeTypeOf("function");
    flush!();
    await vi.waitFor(
      () => {
        for (const signal of ["traces", "metrics", "logs"]) {
          expect(received.has(`/v1/${signal}`)).toBe(true);
        }
      },
      { timeout: 5000 },
    );
    for (const body of received.values()) {
      expect(() => JSON.parse(body)).not.toThrow();
      expect(body).toContain("nines-web");
      expect(body).not.toMatch(
        /private-objective|fixture-secret|private-api-credential/,
      );
    }
    expect(received.get("/v1/traces")).toContain("/api/slos/{id}/context");
  } finally {
    server.closeAllConnections();
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
});
