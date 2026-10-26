// @vitest-environment jsdom
import { act, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { IntentWorkspace } from "./intent-workspace";
import { createTenantSession } from "@/lib/api";
import { navigateTo, useRoute, workspacePath } from "@/lib/navigation";

let root: Root, host: HTMLDivElement;
let complete: (response: Response) => void;
const createdID = "d14d9627-5966-44e6-bbd5-e7f6b872a907";
const scope = createTenantSession("credential", "alpha", "owner");
function Workspace() {
  const route = useRoute();
  return route.kind === "workspace" && route.page === "intents" ? (
    <IntentWorkspace scope={scope} journeys={[]} onApplied={() => {}} />
  ) : (
    <h1>Home</h1>
  );
}
function response(id = createdID) {
  return new Response(
    JSON.stringify({
      tenant: "alpha",
      apiVersion: "nines.dev/v1alpha1",
      id,
      title: "API availability",
      version: 1,
      state: "draft",
      scope: {
        metadata: { name: "api", project: "production" },
        service: "api",
        source: "primary",
        population: {},
        noDataPolicy: "unknown",
      },
      createdAt: "2026-10-23T00:00:00Z",
      updatedAt: "2026-10-23T00:00:00Z",
      messages: [],
      runs: [],
    }),
    { status: 200 },
  );
}
beforeEach(async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  window.history.replaceState(null, "", workspacePath("alpha", "intents"));
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      if (init?.method === "POST")
        return new Promise<Response>((resolve) => {
          complete = resolve;
        });
      if (path.endsWith("/capabilities"))
        return new Response(
          JSON.stringify({
            tenant: "alpha",
            modelEnabled: false,
            systemOneEnabled: true,
            systemOneRoute: {
              provider: "typesafe",
              model: "jev-1.14.0",
              contractDigest: "contract",
            },
            draftEnabled: false,
            skill: { id: "intent", version: "1", digest: "test" },
          }),
        );
      if (path === "/api/intent-conversations")
        return new Response(JSON.stringify({ tenant: "alpha", items: [] }));
      return response(path.split("/").at(-1));
    }),
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  await act(async () =>
    root.render(
      <StrictMode>
        <Workspace />
      </StrictMode>,
    ),
  );
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function button(text: string) {
  return [...host.querySelectorAll("button")].find(
    (item) => item.textContent === text,
  )!;
}
async function start() {
  await act(async () => button("New conversation").click());
  for (const [id, value] of Object.entries({
    name: "api",
    project: "production",
    service: "api",
    source: "primary",
    population: 'service: "api"',
  })) {
    const field = host.querySelector<HTMLInputElement | HTMLTextAreaElement>(
      "#flow-" + id,
    )!;
    await act(async () => {
      const prototype =
        field instanceof HTMLTextAreaElement
          ? HTMLTextAreaElement.prototype
          : HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(
        field,
        value,
      );
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }
  await act(async () => button("Save conversation").click());
  expect(complete).toBeTypeOf("function");
}
test("a completed create request opens its conversation while the author stays on the page", async () => {
  await start();
  await act(async () => complete(response()));
  expect(window.location.pathname).toBe(
    workspacePath("alpha", "intents", createdID),
  );
  expect(host.textContent).toContain("API availability");
});
test("a late create response cannot navigate back after leaving the intent workspace", async () => {
  await start();
  await act(async () => navigateTo(workspacePath("alpha", "home")));
  await act(async () => complete(response()));
  expect(window.location.pathname).toBe(workspacePath("alpha", "home"));
  expect(host.textContent).toBe("Home");
});
test("a late create response cannot replace a conversation selected through history navigation", async () => {
  await start();
  await act(async () =>
    navigateTo(workspacePath("alpha", "intents", "previous")),
  );
  await act(async () => complete(response()));
  expect(window.location.pathname).toBe(
    workspacePath("alpha", "intents", "previous"),
  );
});

test("System One preparation needs its own opt-in and resets after submission", async () => {
  await act(async () =>
    navigateTo(workspacePath("alpha", "intents", "existing")),
  );
  const label = [...host.querySelectorAll("label")].find((item) =>
    item.textContent?.includes("optional category and definition advice"),
  )!;
  expect(label).toBeTruthy();
  const checkbox = label.querySelector("input")!;
  expect(checkbox.checked).toBe(false);
  expect(
    vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === "POST"),
  ).toHaveLength(0);
  await act(async () => checkbox.click());
  await act(async () => button("Prepare proposal").click());
  const request = vi
    .mocked(fetch)
    .mock.calls.find(
      ([url, init]) => String(url).endsWith("/runs") && init?.method === "POST",
    )!;
  expect(JSON.parse(String(request[1]?.body))).toMatchObject({
    systemOneConsent: true,
    modelConsent: false,
    mode: "guided",
  });
  await act(async () => complete(response("existing")));
  expect(checkbox.checked).toBe(false);
});
