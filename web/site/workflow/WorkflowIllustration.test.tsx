// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { WorkflowIllustration } from "./WorkflowIllustration";

let root: Root, host: HTMLDivElement;
let reduced = false;
const fetcher = vi.fn();
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("matchMedia", () => ({
    matches: reduced,
    addEventListener() {},
    removeEventListener() {},
  }));
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  fetcher.mockClear();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  reduced = false;
  vi.unstubAllGlobals();
});
async function select(name: string) {
  const tab = [
    ...host.querySelectorAll<HTMLButtonElement>('[role="tab"]'),
  ].find((node) => node.textContent?.includes(name))!;
  await act(async () => {
    tab.click();
  });
}

test("the server-rendered poster shows review and never pretends to apply a change", () => {
  const html = renderToString(<WorkflowIllustration />);
  expect(html).toContain("Awaiting engineer review");
  expect(html).toContain("Example only; no commands run here");
  expect(html).not.toContain("nines apply");
  expect(html).toContain("animation-name:none");
});

test("all four steps expose their commands without making network calls", async () => {
  await act(async () => root.render(<WorkflowIllustration />));
  for (const label of ["Inspect", "Validate", "Plan", "Review"]) {
    await select(label);
    const tab = host.querySelector('[role="tab"][aria-selected="true"]');
    expect(tab?.textContent).toContain(label);
    expect(
      host.querySelector('[role="tabpanel"][data-state="active"] pre')
        ?.textContent,
    ).toContain("nines ");
    expect(host.querySelector('[role="status"]')?.textContent).not.toBe("");
    for (const outgoing of host.querySelectorAll(".ss-swap__out"))
      expect(outgoing.getAttribute("aria-hidden")).toBe("true");
  }
  expect(fetcher).not.toHaveBeenCalled();
});

test("reduced motion replaces text without keeping an animated outgoing copy", async () => {
  reduced = true;
  await act(async () => root.render(<WorkflowIllustration />));
  await select("Inspect");
  expect(host.querySelector(".ss-swap__out")).toBeNull();
  expect(
    host.querySelector<HTMLElement>(".ss-swap__in")?.style.animationName,
  ).toBe("none");
  expect(host.querySelector("[role=status]")?.textContent).toBe(
    "Contracts available",
  );
});
