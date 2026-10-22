import { expect, test, vi, afterEach } from "vitest";
import { decodeRunEvent, streamReviewRun } from "./review-stream";
import { createTenantSession } from "./api";
afterEach(() => vi.unstubAllGlobals());
const run = {
  apiVersion: "nines.dev/v1alpha1",
  tenant: "alpha",
  id: "run-1",
  investigationId: "review-1",
  version: 2,
  status: "completed",
};
test("stream events cannot change tenant or review scope", () => {
  const scope = createTenantSession("token", "alpha", "viewer");
  expect(() =>
    decodeRunEvent(
      JSON.stringify({ ...run, tenant: "beta" }),
      scope,
      "review-1",
      "run-1",
    ),
  ).toThrow();
  expect(() =>
    decodeRunEvent(
      JSON.stringify({ ...run, investigationId: "other" }),
      scope,
      "review-1",
      "run-1",
    ),
  ).toThrow();
});
test("authenticated progress uses headers and follows only scoped committed versions", async () => {
  const scope = createTenantSession("token", "alpha", "viewer");
  const frames = `id: 2\nevent: run\ndata: ${JSON.stringify(run)}\n\nevent: done\ndata: {"reason":"terminal"}\n\n`;
  const fetch = vi.fn(async (_path: string, init: RequestInit) => {
    const headers = new Headers(init.headers);
    expect(headers.get("Authorization")).toBe("Bearer token");
    expect(headers.get("X-Nines-Tenant")).toBe("alpha");
    expect(headers.get("Last-Event-ID")).toBe("1");
    return new Response(frames, {
      headers: { "Content-Type": "text/event-stream" },
    });
  });
  vi.stubGlobal("fetch", fetch);
  const received = vi.fn();
  expect(
    await streamReviewRun(
      scope,
      "review-1",
      "run-1",
      1,
      new AbortController().signal,
      received,
    ),
  ).toBe(true);
  expect(received).toHaveBeenCalledWith(run);
  expect(fetch.mock.calls[0][0]).not.toContain("token");
});

test("CRLF and UTF-8 fragments can split at any network byte", async () => {
  const scope = createTenantSession("token", "alpha", "viewer");
  const value = { ...run, question: "café" };
  const frames = `event: run\r\ndata: ${JSON.stringify(value)}\r\n\r\nevent: done\r\ndata: {"reason":"terminal"}\r\n\r\n`;
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const byte of new TextEncoder().encode(frames))
        controller.enqueue(new Uint8Array([byte]));
      controller.close();
    },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(stream, {
          headers: { "Content-Type": "text/event-stream" },
        }),
    ),
  );
  const receive = vi.fn();
  expect(
    await streamReviewRun(
      scope,
      "review-1",
      "run-1",
      1,
      new AbortController().signal,
      receive,
    ),
  ).toBe(true);
  expect(receive).toHaveBeenCalledWith(value);
});
