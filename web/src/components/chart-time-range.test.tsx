// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ChartTimeRange } from "./chart-time-range";
let root: Root, host: HTMLDivElement;
const changed = vi.fn();
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  changed.mockClear();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
async function open() {
  await act(async () =>
    root.render(
      <ChartTimeRange
        value={{ kind: "relative", seconds: 3600 }}
        maxSeconds={86400}
        stepSeconds={60}
        observedTo="2026-01-15T11:00:00Z"
        onChange={changed}
      />,
    ),
  );
  await act(async () => host.querySelector("button")!.click());
}
async function input(label: string, value: string) {
  const node = document.querySelector<HTMLInputElement>(
    `input[id$="-${label}"]`,
  )!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(node, value);
    node.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function apply() {
  await act(async () =>
    document
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
}
test("custom edits do not query until Apply and publish one aligned UTC range", async () => {
  await open();
  await input("from", "2026-01-15T10:01:01");
  await input("to", "2026-01-15T10:59:59");
  expect(changed).not.toHaveBeenCalled();
  await apply();
  expect(changed).toHaveBeenCalledTimes(1);
  expect(changed).toHaveBeenCalledWith({
    kind: "absolute",
    from: "2026-01-15T10:02:00.000Z",
    to: "2026-01-15T10:59:00.000Z",
  });
});
test("invalid ranges stay open and do not issue a request", async () => {
  await open();
  await input("from", "2026-01-15T11:01:00");
  await apply();
  expect(changed).not.toHaveBeenCalled();
  expect(document.querySelector('[role="alert"]')?.textContent).toContain(
    "End time must be after",
  );
});
