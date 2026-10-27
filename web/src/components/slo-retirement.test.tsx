// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { SLORetirement } from "./slo-retirement";
import { createTenantSession, type SLO } from "@/lib/api";
let root: Root, host: HTMLDivElement;
const changed = vi.fn(),
  deleted = vi.fn();
const slo: SLO = {
  id: "api",
  name: "API",
  project: "p",
  service: "api",
  target: 0.99,
  revision: 3,
  stepSeconds: 60,
  windowSeconds: 3600,
};
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  changed.mockClear();
  deleted.mockClear();
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
const button = (text: string) =>
  [...host.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => b.textContent === text,
  )!;
test("archive applies only the displayed review and a viewer has no mutation control", async () => {
  const calls: { path: string; body?: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, body: String(init?.body ?? "") });
      return new Response(
        JSON.stringify({
          tenant: "alpha",
          slo: "api",
          action: "archive",
          expectedRevision: 3,
          digest: "reviewed",
          references: [],
          blockers: [],
        }),
      );
    }),
  );
  const owner = createTenantSession("token", "alpha", "owner");
  await act(async () =>
    root.render(
      <SLORetirement
        slo={slo}
        scope={owner}
        onChanged={changed}
        onDeleted={deleted}
      />,
    ),
  );
  expect(calls).toHaveLength(0);
  await act(async () => button("Review archival").click());
  expect(calls).toHaveLength(1);
  await act(async () => button("Archive reviewed objective").click());
  expect(JSON.parse(calls[1].body!)).toEqual({
    action: "archive",
    expectedRevision: 3,
    digest: "reviewed",
    confirmName: "",
  });
  expect(changed).toHaveBeenCalledOnce();
  expect(deleted).not.toHaveBeenCalled();
  await act(async () =>
    root.render(
      <SLORetirement
        slo={slo}
        scope={createTenantSession("viewer", "alpha", "viewer")}
        onChanged={changed}
        onDeleted={deleted}
      />,
    ),
  );
  expect(button("Review archival")).toBeUndefined();
  expect(host.textContent).toContain("tenant owner");
});
test("current references block archival and expose required links accessibly", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            tenant: "alpha",
            slo: "api",
            action: "archive",
            expectedRevision: 3,
            digest: "reviewed",
            references: [
              {
                kind: "CriticalUserJourney",
                name: "checkout",
                revision: 2,
                required: true,
              },
            ],
            blockers: ["Remove current references first."],
          }),
        ),
    ),
  );
  await act(async () =>
    root.render(
      <SLORetirement
        slo={slo}
        scope={createTenantSession("owner", "alpha", "owner")}
        onChanged={changed}
        onDeleted={deleted}
      />,
    ),
  );
  await act(async () => button("Review archival").click());
  expect(button("Archive reviewed objective").disabled).toBe(true);
  expect(host.textContent).toContain("required link");
  const { default: axe } = await import("axe-core");
  const results = await axe.run(host, {
    runOnly: {
      type: "tag",
      values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"],
    },
    rules: { "color-contrast": { enabled: false } },
  });
  expect(results.violations.map((v) => v.id)).toEqual([]);
});
