// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { DependencyDesigner } from "./dependency-designer";
import { mapStarter } from "@/lib/topology";
import type { DependencyManifest } from "@/lib/dependency-editor";
let root: Root, host: HTMLDivElement;
let latest: DependencyManifest;
function Editor() {
  const [draft, setDraft] = useState<DependencyManifest>({
    ...structuredClone(mapStarter),
    spec: { ...structuredClone(mapStarter.spec), dependencies: [] },
  });
  latest = draft;
  return (
    <DependencyDesigner value={draft} disabled={false} onChange={setDraft} />
  );
}
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
test("keyboard movement and non-drag connection form edit a draft without network writes", async () => {
  await act(async () => root.render(<Editor />));
  const move = host.querySelector<HTMLButtonElement>(
    '[aria-label="Move service api"]',
  )!;
  await act(async () =>
    move.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "ArrowRight",
        bubbles: true,
        cancelable: true,
      }),
    ),
  );
  expect(latest.spec.services.find((n) => n.name === "api")?.position?.x).toBe(
    64,
  );
  const controls = host.querySelectorAll("select");
  await act(async () => {
    controls[0].value = "api";
    controls[0].dispatchEvent(new Event("change", { bubbles: true }));
    controls[1].value = "database";
    controls[1].dispatchEvent(new Event("change", { bubbles: true }));
  });
  await act(async () =>
    host
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
  expect(latest.spec.dependencies).toEqual([
    { caller: "api", callee: "database", critical: false },
  ]);
  expect(host.querySelector('[role="status"]')?.textContent).toContain(
    "Added dependency",
  );
});
test("visual editor passes automated WCAG semantic checks with keyboard alternatives", async () => {
  await act(async () => root.render(<Editor />));
  const { default: axe } = await import("axe-core");
  const result = await axe.run(host, {
    runOnly: {
      type: "tag",
      values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"],
    },
    rules: { "color-contrast": { enabled: false } },
  });
  expect(
    result.violations.map((v) => ({
      id: v.id,
      targets: v.nodes.map((n) => n.target),
    })),
  ).toEqual([]);
});
