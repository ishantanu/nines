// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { replayFixture } from "./lib/replay-test-fixtures";
const mock = vi.hoisted(() => ({ instances: [] as any[], broken: false }));
vi.mock("@rrweb/replay", () => ({
  Replayer: class {
    current = 0;
    callbacks = new Map<string, () => void>();
    destroy = vi.fn();
    getMetaData = () => ({ totalTime: 1000 });
    getCurrentTime = () => this.current;
    play = vi.fn((offset: number) => {
      this.current = offset;
    });
    pause = vi.fn((offset?: number) => {
      if (offset !== undefined) this.current = offset;
    });
    on = (name: string, fn: () => void) => this.callbacks.set(name, fn);
    constructor(_events: unknown, options: { root: HTMLElement }) {
      if (mock.broken) throw Error("synthetic parser failure");
      const wrapper = document.createElement("div");
      wrapper.className = "replayer-wrapper";
      const frame = document.createElement("iframe");
      frame.style.width = "800px";
      frame.style.height = "600px";
      wrapper.append(frame);
      options.root.append(wrapper);
      mock.instances.push(this);
    }
  },
}));
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});
test("player controls, frame scaling, trusted messages and malformed-stream recovery", async () => {
  vi.useFakeTimers();
  const parentWindow = { postMessage: vi.fn() };
  vi.stubGlobal("parent", parentWindow);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
    },
  );
  document.body.innerHTML =
    '<button id="play" disabled>Play</button><button id="pause" disabled>Pause</button><input id="seek" type="range" min="0" max="1" disabled><span id="status"></span><div id="player"></div><script type="application/json" id="replay-events"></script>';
  document.getElementById("replay-events")!.textContent =
    JSON.stringify(replayFixture());
  const root = document.getElementById("player")!;
  Object.defineProperty(root, "clientWidth", {
    value: 400,
    configurable: true,
  });
  Object.defineProperty(root, "clientHeight", {
    value: 300,
    configurable: true,
  });
  await import("./replay-player");
  const play = document.getElementById("play") as HTMLButtonElement;
  const pause = document.getElementById("pause") as HTMLButtonElement;
  const seek = document.getElementById("seek") as HTMLInputElement;
  const status = document.getElementById("status")!;
  expect(status.textContent).toBe("Ready");
  expect((root.firstChild as HTMLElement).style.transform).toBe("scale(0.5)");
  const first = mock.instances[0];
  play.click();
  first.current = 400;
  vi.advanceTimersByTime(250);
  expect(seek.value).toBe("400");
  pause.click();
  expect(status.textContent).toBe("Paused");
  seek.value = "600";
  seek.dispatchEvent(new Event("input"));
  play.click();
  expect(first.play).toHaveBeenLastCalledWith(600);
  first.current = 1000;
  vi.advanceTimersByTime(250);
  expect(status.textContent).toBe("Finished");
  play.click();
  expect(first.play).toHaveBeenLastCalledWith(0);
  function receive(events: unknown, origin = location.origin) {
    window.dispatchEvent(
      new MessageEvent("message", {
        source: parentWindow as unknown as Window,
        origin,
        data: { type: "nines-replay-load", events },
      }),
    );
  }
  receive([], "https://untrusted.example");
  expect(status.textContent).toBe("Playing");
  receive([]);
  expect(status.textContent).toBe("This recording could not be played.");
  expect(first.destroy).toHaveBeenCalledOnce();
  expect(root.children).toHaveLength(0);
  vi.advanceTimersByTime(1000);
  expect(status.textContent).toBe("This recording could not be played.");
  expect(play.disabled).toBe(true);
  expect(pause.disabled).toBe(true);
  expect(seek.disabled).toBe(true);
  receive(replayFixture());
  expect(status.textContent).toBe("Ready");
  expect(play.disabled).toBe(false);
  mock.broken = true;
  receive(replayFixture());
  expect(status.textContent).toBe("This recording could not be played.");
  expect(parentWindow.postMessage).toHaveBeenCalledWith(
    { type: "nines-replay-state", state: "error" },
    location.origin,
  );
});
