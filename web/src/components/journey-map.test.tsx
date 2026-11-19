// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import type { Journey } from "@/lib/api";
import { JourneyMap } from "./journey-map";
let root: Root, host: HTMLDivElement;
let resize: (
  entries: { contentRect: { width: number; height: number } }[],
) => void;
const onSLO = vi.fn();
const journey: Journey = {
  resource: {
    revision: 1,
    digest: "synthetic",
    appliedAt: "2026-11-03T00:00:00Z",
    manifest: {
      apiVersion: "nines.dev/v1alpha1",
      kind: "CriticalUserJourney",
      metadata: { name: "checkout", project: "test" },
      spec: {
        displayName: "Checkout",
        description: "Synthetic journey",
        owner: "test",
        criticality: "high",
        slos: [],
        steps: [
          {
            id: "start",
            name: "Start checkout",
            actor: "user",
            sloRefs: ["api"],
          },
          {
            id: "pay",
            name: "Authorize payment",
            actor: "service",
            dependsOn: ["start"],
            sloRefs: [],
          },
          {
            id: "stock",
            name: "Reserve stock",
            actor: "service",
            dependsOn: ["start"],
            sloRefs: [],
          },
          {
            id: "finish",
            name: "Confirm order",
            actor: "service",
            dependsOn: ["pay", "stock"],
            sloRefs: [],
          },
        ],
      },
    },
  },
  health: "unknown",
  links: [],
  reasons: [],
};
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: typeof resize) {
        resize = callback;
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  onSLO.mockClear();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
async function render() {
  await act(async () =>
    root.render(<JourneyMap journey={journey} onSLO={onSLO} />),
  );
}
async function click(text: string) {
  const button = [...host.querySelectorAll("button")].find(
    (b) => b.textContent === text || b.getAttribute("aria-label") === text,
  )!;
  expect(button).toBeDefined();
  await act(async () => button.click());
}
test("map draws only explicit dependencies, fits changes in available space, and restores inspection focus", async () => {
  await render();
  expect(
    host.querySelectorAll("[data-journey-world] > svg > path"),
  ).toHaveLength(4);
  await act(async () => resize([{ contentRect: { width: 420, height: 400 } }]));
  const fitted = host.querySelector<HTMLElement>("[data-journey-world]")!.style
    .transform;
  await click("Reset journey zoom to 100 percent");
  expect(
    host.querySelector<HTMLElement>("[data-journey-world]")!.style.transform,
  ).toBe("scale(1)");
  await click("Fit journey");
  expect(
    host.querySelector<HTMLElement>("[data-journey-world]")!.style.transform,
  ).toBe(fitted);
  await click("Inspect Start checkout");
  await act(async () => resize([{ contentRect: { width: 300, height: 400 } }]));
  expect(
    host.querySelector<HTMLElement>("[data-journey-world]")!.style.transform,
  ).toBe(fitted);
  expect(
    host.querySelector('[aria-label="Selected journey step"]')?.textContent,
  ).toContain("Measurement unavailable");
  await click("api");
  expect(onSLO).toHaveBeenCalledWith("api");
  await click("Close step details");
  expect(host.querySelector('[aria-label="Selected journey step"]')).toBeNull();
  expect(document.activeElement?.getAttribute("aria-label")).toBe(
    "Inspect Start checkout",
  );
});
test("ordered view exposes complete dependency names and expandable stages with keyboard-accessible controls", async () => {
  await render();
  await click("List");
  expect(host.querySelectorAll("ol li")).toHaveLength(4);
  expect(host.textContent).toContain("After Authorize payment, Reserve stock");
  expect(host.querySelectorAll("details")).toHaveLength(3);
  await click("Inspect Confirm order");
  expect(
    host.querySelector('[aria-label="Selected journey step"]')?.textContent,
  ).toContain("No objectives linked");
  const { default: axe } = await import("axe-core");
  const result = await axe.run(host, {
    runOnly: {
      type: "tag",
      values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"],
    },
    rules: { "color-contrast": { enabled: false } },
  });
  expect(result.violations).toEqual([]);
});
test("expanded journey is an accessible dialog that closes with Escape", async () => {
  await render();
  await click("Expand journey");
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
    "Checkout",
  );
  expect(document.querySelectorAll(".journey-canvas-expanded")).toHaveLength(1);
  await act(async () =>
    document
      .querySelector('[role="dialog"]')!
      // A native <dialog> reports Escape as `cancel`, which Astryx listens for.
      .dispatchEvent(new Event("cancel", { cancelable: true })),
  );
  expect(document.querySelector('[role="dialog"]')).toBeNull();
});
