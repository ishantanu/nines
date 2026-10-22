import { afterEach, expect, test, vi } from "vitest";
import { replayController } from "./session-replay";
import { initialDarkMode, readPreference, savePreference } from "./preferences";
afterEach(() => vi.unstubAllGlobals());
test("unavailable storage never blocks startup or the theme control", () => {
  vi.stubGlobal("window", {
    get localStorage() {
      throw new Error("denied");
    },
    matchMedia: () => ({ matches: true }),
  });
  expect(readPreference("nines-theme")).toBeNull();
  expect(() => savePreference("nines-theme", "dark")).not.toThrow();
  expect(initialDarkMode()).toBe(true);
});
test("a cancelled lazy recorder does not initialize or capture a credential page", async () => {
  let resolve!: (v: any) => void;
  const client = {
    init: vi.fn(),
    startSessionRecording: vi.fn(),
    stopSessionRecording: vi.fn(),
  };
  const initialize = vi.fn();
  const start = replayController(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
    initialize,
  );
  const controller = new AbortController();
  const result = start(controller.signal);
  controller.abort();
  resolve(client);
  await result;
  expect(initialize).not.toHaveBeenCalled();
  expect(client.startSessionRecording).not.toHaveBeenCalled();
});
test("stale cleanup cannot stop a newer recording owner", async () => {
  const client = {
    init: vi.fn(),
    startSessionRecording: vi.fn(),
    stopSessionRecording: vi.fn(),
  };
  const start = replayController(async () => client as any, vi.fn());
  const a = new AbortController();
  const stopA = await start(a.signal);
  const b = new AbortController();
  const stopB = await start(b.signal);
  const stops = client.stopSessionRecording.mock.calls.length;
  stopA();
  a.abort();
  expect(client.stopSessionRecording).toHaveBeenCalledTimes(stops);
  stopB();
  expect(client.stopSessionRecording).toHaveBeenCalledTimes(stops + 1);
});
