// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { SLOTypeGuide } from "./slo-type-guide";
import { SLOTypeBadges } from "./slo-type-badge";

let root: Root, host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

test("every kind has a guided action, with its YAML equivalent still available", async () => {
  const chosen: string[] = [];
  await act(async () =>
    root.render(
      <SLOTypeGuide selected="availability" onChoose={(k) => chosen.push(k)} />,
    ),
  );
  const tile = (id: string) =>
    host.querySelector(`[data-slo-type-tile="${id}"]`)!;
  expect(tile("availability").textContent).toContain("Selected");
  for (const [id, label] of [
    ["latency", "Use latency"],
    ["sessions", "Use sessions"],
    ["good-minute", "Use good-minute"],
    ["customers", "Add per-customer report"],
  ] as const) {
    const button = [...tile(id).querySelectorAll("button")].find((b) =>
      b.textContent?.includes(label),
    )!;
    await act(async () => button.click());
  }
  expect(chosen).toEqual(["latency", "sessions", "good-minute", "customers"]);
  // The journey form lives on the journeys page, so the tile points there.
  expect(tile("journey").textContent).toContain("Critical user journeys");
  // No kind is described as YAML-only any more.
  expect(host.textContent).not.toMatch(/\bYAML\b\s*$/m);
  expect(tile("sessions").textContent).toContain("type: sessions");
  expect(tile("good-minute").textContent).toContain("badInterval:");
  expect(tile("customers").textContent).toContain("label: account");
  expect(
    host.querySelectorAll("a[href^='https://ishantanu.github.io/nines/docs/']")
      .length,
  ).toBeGreaterThan(4);
});

test("badges name what the objective counts", async () => {
  await act(async () =>
    root.render(
      <SLOTypeBadges
        slo={{
          badInterval: {},
          customers: { label: "a", customers: [{ value: "x" }] },
        }}
      />,
    ),
  );
  expect(host.textContent).toContain("Good-minute");
  expect(host.textContent).toContain("Per-customer");
});
