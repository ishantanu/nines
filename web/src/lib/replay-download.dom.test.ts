// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { replayFixture } from "./replay-test-fixtures";
import { downloadReplay } from "./replay-download";
afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
test("portable replay download clicks a connected anchor and removes it afterward", async () => {
  vi.useFakeTimers();
  let clicked = false;
  let blob: Blob | undefined;
  vi.stubGlobal("URL", {
    createObjectURL: vi.fn((value: Blob) => {
      blob = value;
      return "blob:test";
    }),
    revokeObjectURL: vi.fn(),
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response('console.log("bundled player")')),
  );
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    clicked = true;
    expect(this.isConnected).toBe(true);
    expect(this.download).toBe("nines-replay-recording.html");
  });
  await downloadReplay(
    "recording",
    replayFixture(),
    new AbortController().signal,
  );
  expect(clicked).toBe(true);
  expect(blob?.type).toBe("text/html;charset=utf-8");
  expect(document.querySelector("a[download]")).toBeNull();
});
test("cancelled portable download does not create or click a download link", async () => {
  vi.useFakeTimers();
  const create = vi.fn();
  vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: vi.fn() });
  const controller = new AbortController();
  controller.abort();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("player")),
  );
  await downloadReplay(
    "recording",
    [{ type: 2, timestamp: 1, data: {} }],
    controller.signal,
  );
  expect(create).not.toHaveBeenCalled();
});
