// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { parse, stringify } from "yaml";
import { TopologyEditor } from "./topology-editor";
import { createTenantSession } from "@/lib/api";
import { mapStarter, mapKind } from "@/lib/topology";
let root: Root, host: HTMLDivElement;
let calls: { path: string; init?: RequestInit }[];
const saved = vi.fn();
beforeEach(() => {
  calls = [];
  saved.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      return new Response(
        JSON.stringify(
          path.endsWith("/plan")
            ? {
                action: "update",
                expectedRevision: 3,
                digest: "reviewed",
                manifest: parse(String(init?.body)),
              }
            : { revision: 4 },
        ),
      );
    }),
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function button(name: string) {
  return [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => b.textContent?.trim() === name,
  )!;
}
async function click(name: string) {
  await act(async () => button(name).click());
}
async function open(role: "owner" | "viewer" = "owner") {
  await act(async () =>
    root.render(
      <TopologyEditor
        scope={createTenantSession("fixture", "alpha", role)}
        initial={stringify(mapStarter)}
        kind={mapKind}
        label="Edit map"
        onApplied={saved}
      />,
    ),
  );
  await click("Edit map");
}
test("visual edits invalidate a plan and saving uses the freshly reviewed revision", async () => {
  await open();
  await click("Validate and plan");
  expect(button("Apply reviewed definition").disabled).toBe(false);
  const move = document.querySelector<HTMLButtonElement>(
    '[aria-label="Move service api"]',
  )!;
  await act(async () =>
    move.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "ArrowDown",
        bubbles: true,
        cancelable: true,
      }),
    ),
  );
  expect(button("Apply reviewed definition").disabled).toBe(true);
  expect(calls.filter((c) => c.path.endsWith("/apply"))).toHaveLength(0);
  await click("Validate and plan");
  await click("Apply reviewed definition");
  const apply = calls.find((c) => c.path.endsWith("/apply"))!;
  expect(new Headers(apply.init?.headers).get("If-Match")).toBe('"3"');
  expect(new Headers(apply.init?.headers).get("X-Nines-Tenant")).toBe("alpha");
  expect(parse(String(apply.init?.body)).spec.services[0].position.y).toBe(64);
  expect(saved).toHaveBeenCalledOnce();
});
test("a viewer can plan the same draft but cannot save it", async () => {
  await open("viewer");
  await click("Validate and plan");
  expect(button("Owner or editor required").disabled).toBe(true);
  expect(calls.filter((c) => c.path.endsWith("/apply"))).toHaveLength(0);
});
