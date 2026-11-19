// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { NinesFeatures } from "./NinesFeatures";

let root: Root, host: HTMLDivElement;
let reduced = false;
const fetcher = vi.fn();
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("fetch", fetcher);
  fetcher.mockClear();
  vi.stubGlobal("matchMedia", () => ({
    matches: reduced,
    addEventListener() {},
    removeEventListener() {},
  }));
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(
        private callback: (entries: { isIntersecting: boolean }[]) => void,
      ) {}
      observe() {
        this.callback([{ isIntersecting: true }]);
      }
      disconnect() {}
    },
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  reduced = false;
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

test("the actual bento prerenders labelled synthetic examples and crawlable guide links", () => {
  const html = renderToString(<NinesFeatures />);
  host.innerHTML = html;
  expect(host.querySelector('[data-slot="feature-bento"]')).not.toBeNull();
  expect(host.textContent).toContain(
    "Interactive illustrations with synthetic data",
  );
  expect(host.textContent).toContain("No SLO or rule is changed");
  expect(host.textContent).not.toContain("$");
  expect(host.querySelector('a[href="./docs/intents/"]')).not.toBeNull();
  expect(host.querySelector('a[href="./docs/cli/"]')).not.toBeNull();
  expect(host.querySelector('a[href="./docs/mcp/"]')).not.toBeNull();
  expect(fetcher).not.toHaveBeenCalled();
});

test("illustrations can be paused and reduced motion stays in the finished state", async () => {
  reduced = true;
  await act(async () => root.render(<NinesFeatures />));
  const section = host.querySelector('[data-slot="feature-bento"]')!;
  expect(section.getAttribute("data-still")).toBe("true");
  const pause = host.querySelector<HTMLButtonElement>(".bento-motion-control")!;
  await act(async () => pause.click());
  expect(pause.getAttribute("aria-pressed")).toBe("true");
  expect(pause.textContent).toBe("Play illustrations");
  await act(async () => pause.click());
  expect(section.getAttribute("data-still")).toBe("true");
  expect(fetcher).not.toHaveBeenCalled();
});

test("keyboard inspection shows burn rates without applying changes", async () => {
  reduced = true;
  await act(async () => root.render(<NinesFeatures />));
  const sample = host.querySelector<HTMLElement>(
    '[role="button"][aria-label="00:10 UTC: 1.20×"]',
  )!;
  await act(async () => sample.focus());
  expect(sample.getAttribute("aria-pressed")).toBe("true");
  expect(host.textContent).toContain("1.20×");
  const input = host.querySelector<HTMLInputElement>('[role="combobox"]')!;
  await act(async () => input.focus());
  expect(input.getAttribute("aria-label")).toBe("Find a workflow guide");
  expect(host.querySelectorAll('[role="option"]').length).toBe(3);
  expect(fetcher).not.toHaveBeenCalled();
  const { default: axe } = await import("axe-core");
  const result = await axe.run(host, {
    rules: { "color-contrast": { enabled: false }, region: { enabled: false } },
  });
  expect(result.violations).toEqual([]);
});

test("pause settles the illustrations and cancels their advancing clock", async () => {
  vi.useFakeTimers();
  await act(async () => root.render(<NinesFeatures />));
  const section = host.querySelector('[data-slot="feature-bento"]')!;
  expect(section.getAttribute("data-still")).toBe("false");
  await act(async () =>
    host.querySelector<HTMLButtonElement>(".bento-motion-control")!.click(),
  );
  expect(section.getAttribute("data-still")).toBe("true");
  const snapshot = host.innerHTML;
  await act(async () => vi.advanceTimersByTime(2000));
  expect(host.innerHTML).toBe(snapshot);
  expect(fetcher).not.toHaveBeenCalled();
});

test("guide navigation stays stable while illustrations advance", async () => {
  vi.useFakeTimers();
  await act(async () => root.render(<NinesFeatures />));
  await act(async () => vi.advanceTimersByTime(12000));
  expect(host.querySelector<HTMLInputElement>('[role="combobox"]')!.value).toBe(
    "",
  );
  expect(host.querySelectorAll('[role="option"]')).toHaveLength(3);
  expect(host.textContent).not.toContain("⌘K");
});
