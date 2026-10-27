// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { readFileSync } from "node:fs";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { DependencyDiscovery } from "./dependency-discovery";
import { createTenantSession } from "@/lib/api";
import { mapStarter } from "@/lib/topology";
import type { DependencyManifest } from "@/lib/dependency-editor";
let root: Root, host: HTMLDivElement;
let calls: { path: string; init?: RequestInit }[];
let release: ((value: Response) => void) | undefined;
const changed = vi.fn(),
  adopted = vi.fn();
const receipt = {
  tenant: "alpha",
  manifest: mapStarter,
  inventoryDigest: "fixture-digest",
  evidence: [],
  warnings: ["Configuration candidates"],
};
beforeEach(() => {
  calls = [];
  release = undefined;
  changed.mockClear();
  adopted.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      if (init?.method === "POST")
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      return new Response(
        JSON.stringify({
          tenant: "alpha",
          clusterEnabled: false,
          namespaces: [],
        }),
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
const button = (name: string) =>
  [...host.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => b.textContent?.trim() === name,
  )!;
async function setup() {
  const scope = createTenantSession("fixture", "alpha", "owner");
  await act(async () =>
    root.render(
      <DependencyDiscovery
        scope={scope}
        value={structuredClone(mapStarter) as DependencyManifest}
        disabled={false}
        onChange={changed}
        onAdopt={adopted}
      />,
    ),
  );
  const raw = readFileSync(
    "../internal/kubediscovery/testdata/resources.yaml",
    "utf8",
  ).replace(
    "postgres://database",
    "postgres://fixture-user:credential-sentinel@database",
  );
  const textarea = host.querySelector("textarea")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )!.set!.call(textarea, raw);
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("Preview discovery").click());
  return scope;
}
test("only projected inventory leaves the browser and adoption never applies a map", async () => {
  await setup();
  const post = calls.find((c) => c.init?.method === "POST")!;
  const body = String(post.init?.body);
  for (const secret of [
    "credential-sentinel",
    "fixture-user",
    "postgres://",
    "DEBUG_TOKEN",
    "private-cache-reference",
    "example.invalid",
  ])
    expect(body).not.toContain(secret);
  expect(JSON.parse(body).inventory.resources).toHaveLength(4);
  await act(async () => release!(new Response(JSON.stringify(receipt))));
  expect(changed).not.toHaveBeenCalled();
  await act(async () => button("Add discovery to draft").click());
  expect(changed).toHaveBeenCalledOnce();
  expect(adopted).toHaveBeenCalledOnce();
  expect(
    calls.every((c) => c.path === "/api/dependency-discovery/kubernetes"),
  ).toBe(true);
});
test("a late preview from a closed tenant scope cannot be adopted", async () => {
  const scope = await setup();
  scope.close();
  await act(async () => release!(new Response(JSON.stringify(receipt))));
  expect(button("Add discovery to draft")).toBeUndefined();
  expect(changed).not.toHaveBeenCalled();
});
