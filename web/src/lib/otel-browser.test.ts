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
