// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, test, vi } from "vitest";
import { SLOTemplatePicker } from "./slo-template-picker";
import { createTenantSession } from "@/lib/api";
import type { SLOTemplateCatalog } from "@/lib/slo-templates";
let host: HTMLDivElement, root: Root;
const select = vi.fn();
const catalog: SLOTemplateCatalog = {
  tenant: "alpha",
  apiVersion: "nines.dev/v1alpha1",
  kind: "SLOTemplateCatalog",
  digest: "digest",
  categories: [
    { id: "request-serving", title: "Request serving" },
    { id: "agent", title: "Agents" },
  ],
  templates: [
    {
      id: "agent-completion",
      revision: 1,
      digest: "template-digest",
      category: "agent",
      sli: "task-success",
      title: "Agent task completion",
      description: "Example metric names",
      eligibleEvents: "All terminal tasks",
      goodEvents: "Successful tasks",
      requirements: ["Count timeouts and cancellations"],
      manifest: {
        apiVersion: "nines.dev/v1alpha1",
        kind: "ServiceLevelObjective",
        metadata: { name: "agent-completion", project: "p" },
        spec: {
          displayName: "Agent completion",
          service: "agent",
          lifecycle: "experimental",
          target: 0.99,
          windowSeconds: 86400,
          stepSeconds: 60,
        },
      },
    },
  ],
};
beforeEach(() => {
  select.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(catalog), { status: 200 })),
  );
  Element.prototype.scrollIntoView = vi.fn();
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
async function pick(index: number, text: string) {
  const combo = host.querySelectorAll('[role="combobox"]')[index];
  await act(async () => {
    combo.dispatchEvent(
      new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }),
    );
  });
  const option = [
    ...document.querySelectorAll<HTMLElement>('[role="option"]'),
  ].find((e) => e.textContent === text)!;
  expect(option).toBeTruthy();
  await act(async () => option.click());
}
test("category and SLI selection require an explicit use action and never render or apply remotely", async () => {
  const scope = createTenantSession("token", "alpha", "editor");
  await act(async () =>
    root.render(
      <SLOTemplatePicker scope={scope} disabled={false} onSelect={select} />,
    ),
  );
  await pick(0, "Agents");
  await pick(1, "Agent task completion");
  expect(host.textContent).toContain("All terminal tasks");
  expect(host.textContent).toContain("Count timeouts and cancellations");
  expect(select).not.toHaveBeenCalled();
  const use = [...host.querySelectorAll("button")].find(
    (b) => b.textContent === "Use template",
  )!;
  await act(async () => use.click());
  expect(select).toHaveBeenCalledWith(catalog.templates[0]);
  expect(vi.mocked(fetch).mock.calls).toHaveLength(1);
  expect(vi.mocked(fetch).mock.calls[0][0]).toBe("/api/slo-templates");
});
test("catalog failure leaves manual measurement authoring available", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 503 })),
  );
  await act(async () =>
    root.render(
      <SLOTemplatePicker
        scope={createTenantSession("token", "alpha", "editor")}
        disabled={false}
        onSelect={select}
      />,
    ),
  );
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "You can still define a measurement below",
  );
  expect(select).not.toHaveBeenCalled();
});

test("rejects a catalog returned for a different tenant", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ ...catalog, tenant: "other" }), {
          status: 200,
        }),
    ),
  );
  await act(async () =>
    root.render(
      <SLOTemplatePicker
        scope={createTenantSession("token", "alpha", "editor")}
        disabled={false}
        onSelect={select}
      />,
    ),
  );
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(
    "scope mismatch",
  );
  expect(select).not.toHaveBeenCalled();
});

test.each([false, true])(
  "extension use submits typed inputs with its catalog pin; stale=%s",
  async (stale) => {
    const extensionCatalog = structuredClone(catalog);
    const template = extensionCatalog.templates[0];
    template.extension = {
      name: "example-agent",
      version: "1.0.0",
      sha256: "artifact-digest",
    };
    template.inputs = {
      metric: {
        field: "metric",
        type: "string",
        description: "Observed counter",
        required: true,
      },
    };
    template.evaluation = {
      name: "outcome",
      version: "1.0.0",
      rubric: "Reviewed outcome rubric",
      sampling: "Selected tasks",
      unknownVerdict: "not_good",
      missingVerdict: "unknown",
    };
    const busy = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (path === "/api/slo-templates")
          return new Response(JSON.stringify(extensionCatalog), {
            status: 200,
          });
        expect(path).toBe(`/api/slo-templates/${template.id}/render`);
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toEqual({
          expectedTemplateDigest: template.digest,
          inputs: { metric: "observed_verdicts_total" },
        });
        if (stale)
          return new Response(JSON.stringify({ error: "stale revision" }), {
            status: 409,
          });
        return new Response(
          JSON.stringify({
            apiVersion: "nines.dev/v1alpha1",
            tenant: "alpha",
            templateId: template.id,
            templateDigest: template.digest,
            manifest: template.manifest,
          }),
          { status: 200 },
        );
      }),
    );
    await act(async () =>
      root.render(
        <SLOTemplatePicker
          scope={createTenantSession("token", "alpha", "editor")}
          disabled={false}
          onSelect={select}
          onBusyChange={busy}
        />,
      ),
    );
    await pick(0, "Agents");
    await pick(1, "Agent task completion");
    expect(host.textContent).toContain("Extension example-agent");
    expect(host.textContent).toContain("Reviewed outcome rubric");
    const input = host.querySelector<HTMLInputElement>('input[type="text"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )!.set!.call(input, "observed_verdicts_total");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    const use = [...host.querySelectorAll("button")].find(
      (b) => b.textContent === "Use template",
    )!;
    await act(async () => use.click());
    if (stale) {
      expect(select).not.toHaveBeenCalled();
      expect(host.querySelector('[role="alert"]')).toBeTruthy();
    } else expect(select).toHaveBeenCalledWith(template);
    expect(busy.mock.calls).toEqual([[true], [false]]);
    expect(vi.mocked(fetch).mock.calls).toHaveLength(2);
  },
);
