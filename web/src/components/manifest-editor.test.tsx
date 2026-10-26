// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { parse, stringify } from "yaml";
import { ManifestEditor } from "./manifest-editor";
import { createTenantSession, type Manifest, type Revision } from "@/lib/api";
import { newJourney } from "@/lib/journeys";
let host: HTMLDivElement, root: Root;
const saved = vi.fn(),
  proposed = vi.fn();
let calls: { path: string; init?: RequestInit }[] = [];
let failPlan = false,
  failSave = false;
let sloRead: Promise<Response> | undefined;
function manifest(): Manifest {
  const m = newJourney();
  m.metadata.name = "checkout";
  m.spec.displayName = "Complete checkout";
  m.spec.owner = "commerce";
  m.spec.slos = [{ id: "api", dimension: "availability", required: true }];
  m.spec.steps = [
    { id: "request", name: "Confirm order", actor: "user", sloRefs: ["api"] },
  ];
  return m;
}
const revision: Revision = {
  manifest: manifest(),
  revision: 4,
  digest: "old",
  appliedAt: "2026-10-26T00:00:00Z",
};
beforeEach(() => {
  saved.mockClear();
  proposed.mockClear();
  calls = [];
  failPlan = false;
  failSave = false;
  sloRead = undefined;
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      expect(new Headers(init?.headers).get("X-Nines-Tenant")).toBe("alpha");
      if (path === "/api/slos")
        return (
          sloRead ??
          new Response(
            JSON.stringify([{ slo: { id: "api", name: "API availability" } }]),
          )
        );
      if (path.endsWith("/plan"))
        return failPlan
          ? new Response(JSON.stringify({ error: "cycle detected" }), {
              status: 400,
            })
          : new Response(
              JSON.stringify({
                manifest: parse(String(init?.body)),
                action: "update",
                expectedRevision: 4,
                digest: "reviewed",
                previous: revision,
              }),
            );
      if (failSave)
        return new Response(JSON.stringify({ error: "revision conflict" }), {
          status: 409,
        });
      return new Response(JSON.stringify({ ok: true }));
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
function button(text: string) {
  return [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => b.textContent?.trim() === text,
  )!;
}
async function click(text: string) {
  await act(async () => button(text).click());
}
async function render(
  role: "owner" | "agent" | "viewer" = "owner",
  resource: Revision | undefined = revision,
) {
  await act(async () =>
    root.render(
      <ManifestEditor
        scope={createTenantSession("fixture", "alpha", role)}
        resource={resource}
        onApplied={saved}
        onProposed={proposed}
      />,
    ),
  );
}
async function tab(name: string) {
  const b = [
    ...document.querySelectorAll<HTMLButtonElement>('[role="tab"]'),
  ].find((b) => b.textContent === name)!;
  await act(async () =>
    b.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 })),
  );
}
async function field(label: string, value: string) {
  const l = [...document.querySelectorAll("label")].find(
    (l) => l.textContent?.trim() === label,
  )!;
  const e = document.getElementById(l.htmlFor) as
    HTMLInputElement | HTMLTextAreaElement;
  await act(async () => {
    const proto =
      e.tagName === "TEXTAREA"
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(e, value);
    e.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
test("opens the guided definition and saves exactly the reviewed manifest with a revision precondition", async () => {
  await render();
  await click("Edit definition");
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
    "Linked SLOs",
  );
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
    "API availability",
  );
  expect(button("Save reviewed journey").disabled).toBe(true);
  await field("Display name", "Purchase confirmed");
  await click("Validate and plan");
  const planned = parse(
    String(calls.find((c) => c.path.endsWith("/plan"))!.init?.body),
  );
  expect(planned.spec.displayName).toBe("Purchase confirmed");
  await click("Save reviewed journey");
  const applied = calls.find((c) => c.path.endsWith("/apply"))!;
  expect(new Headers(applied.init?.headers).get("If-Match")).toBe('"4"');
  expect(parse(String(applied.init?.body))).toEqual(planned);
  expect(saved).toHaveBeenCalledOnce();
});
test("changing a form field invalidates review and server validation/conflicts never leave an actionable plan", async () => {
  await render();
  await click("Edit definition");
  await click("Validate and plan");
  await field("Owner", "team-api");
  expect(button("Save reviewed journey").disabled).toBe(true);
  failPlan = true;
  await click("Validate and plan");
  expect(button("Save reviewed journey").disabled).toBe(true);
  failPlan = false;
  await click("Validate and plan");
  failSave = true;
  await click("Save reviewed journey");
  expect(button("Save reviewed journey").disabled).toBe(true);
  expect(saved).not.toHaveBeenCalled();
});
test("YAML changes survive switching to the form; invalid shapes stay editable in YAML", async () => {
  await render();
  await click("Edit definition");
  await tab("YAML");
  const next = manifest();
  next.spec.owner = "team-browser";
  next.spec.agentic = {
    successDefinition: "Confirmed",
    maxCostUSD: 0,
    approvalRequired: true,
  };
  await field("CriticalUserJourney manifest (YAML)", stringify(next));
  await tab("Definition");
  const label = [...document.querySelectorAll("label")].find(
    (l) => l.textContent?.trim() === "Owner",
  )!;
  expect(
    (document.getElementById(label.htmlFor) as HTMLInputElement).value,
  ).toBe("team-browser");
  await tab("YAML");
  await field("CriticalUserJourney manifest (YAML)", "null");
  await tab("Definition");
  expect(
    document.querySelector('[role="tab"][data-state="active"]')?.textContent,
  ).toBe("YAML");
  expect(document.querySelector('[role="alert"]')).not.toBeNull();
});
test("agent submits a UUID proposal and never applies directly; viewer cannot open the editor", async () => {
  await render("agent");
  await click("Edit definition");
  await click("Validate and plan");
  await click("Submit proposal for review");
  const call = calls.find((c) => c.path === "/api/workflows")!;
  expect(JSON.parse(String(call.init?.body)).id).toMatch(
    /^proposal-[0-9a-f-]{36}$/,
  );
  expect(calls.some((c) => c.path.endsWith("/apply"))).toBe(false);
  expect(proposed).toHaveBeenCalledOnce();
  await render("viewer");
  expect(button("Edit definition").disabled).toBe(true);
});
test("closing the editor cancels tenant-scoped SLO discovery", async () => {
  let aborted = false;
  sloRead = new Promise(() => {});
  await render();
  await click("Edit definition");
  const call = calls.find((c) => c.path === "/api/slos")!;
  call.init?.signal?.addEventListener("abort", () => {
    aborted = true;
  });
  await click("Cancel");
  expect(aborted).toBe(true);
});
