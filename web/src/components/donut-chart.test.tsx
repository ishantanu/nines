// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import { DonutChart } from "./ui/notchset/donut-chart";

test("coverage selection keeps the ring and total stable with readable details outside the hole", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  try {
    await act(async () =>
      root.render(
        <DonutChart
          data={[
            { label: "Complete windows", value: 10 },
            { label: "Partial windows with incomplete evidence", value: 1 },
            { label: "No measurements", value: 0 },
          ]}
          digits={0}
          label="Objectives"
          caption="objectives"
        />,
      ),
    );
    const ring = () => [
      ...host.querySelectorAll<SVGCircleElement>('circle[pathLength="100"]'),
    ];
    const shape = () =>
      ring().map((c) => [
        c.getAttribute("r"),
        c.style.strokeWidth,
        c.style.strokeDasharray,
        c.style.strokeDashoffset,
      ]);
    const before = shape();
    const total = host.querySelector('[data-slot="donut-total"]')!.textContent;
    const rows = host.querySelectorAll<HTMLElement>('[role="button"]');
    await act(async () =>
      ring()[0].dispatchEvent(new MouseEvent("mouseover", { bubbles: true })),
    );
    expect(shape()).toEqual(before);
    expect(
      host.querySelector('[data-slot="donut-selection"]')!.textContent,
    ).toContain("Complete windows: 10 · 91%");
    await act(async () => rows[1].focus());
    expect(shape()).toEqual(before);
    expect(
      ring().every(
        (c) => c.style.strokeWidth === "18" && c.style.opacity === "1",
      ),
    ).toBe(true);
    expect(host.querySelector('[data-slot="donut-total"]')!.textContent).toBe(
      total,
    );
    expect(
      host.querySelector('[data-slot="donut-selection"]')!.textContent,
    ).toContain(
      "Partial windows with incomplete evidence: 1 · 9% of objectives",
    );
    expect(rows[1].getAttribute("aria-pressed")).toBe("true");
    await act(async () => rows[2].click());
    expect(
      host.querySelector('[data-slot="donut-selection"]')!.textContent,
    ).toContain("No measurements: 0 · 0%");
    expect(shape()).toEqual(before);
    await act(async () =>
      rows[2].dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
      ),
    );
    expect(
      host.querySelector('[data-slot="donut-selection"]')!.textContent,
    ).toContain("Hover or focus");
    const { default: axe } = await import("axe-core");
    expect(
      (
        await axe.run(host, {
          rules: {
            "color-contrast": { enabled: false },
            region: { enabled: false },
          },
        })
      ).violations,
    ).toEqual([]);
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});
