// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { DataSourceForm } from "./data-source-form";
import { createTenantSession } from "@/lib/api";

let root: Root, host: HTMLDivElement;
let requests: { path: string; init?: RequestInit }[];
const onSaved = vi.fn();
const scope = createTenantSession("synthetic-key", "alpha", "owner");
beforeEach(() => {
  requests = [];
  onSaved.mockClear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      requests.push({ path, init });
      return new Response(JSON.stringify({ id: "trial", version: 1 }));
    }),
  );
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
async function open(
  connection?: Parameters<typeof DataSourceForm>[0]["connection"],
) {
  await act(async () =>
    root.render(
      <DataSourceForm
        scope={scope}
        enabled
        connection={connection}
        allowedRoutes={{
          newrelic: ["123456"],
          "newrelic-entity": ["approved-entity"],
          dash0: ["default"],
          victoriametrics: ["42:7"],
        }}
        onSaved={onSaved}
      />,
    ),
  );
  await act(async () =>
    host.querySelector<HTMLButtonElement>("button")!.click(),
  );
}
async function select(id: string, value: string) {
  await act(async () => {
    const node = document.getElementById(id) as HTMLSelectElement;
    node.value = value;
    node.dispatchEvent(new Event("change", { bubbles: true }));
  });
}
async function input(id: string, value: string) {
  await act(async () => {
    const node = document.getElementById(id) as HTMLInputElement;
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(node, value);
    node.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function save() {
  await act(async () =>
    document
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
}

test.each([
  {
    kind: "datadog",
    credentials: { apiKey: "synthetic-api", applicationKey: "synthetic-app" },
    options: {},
  },
  {
    kind: "newrelic",
    credentials: { userKey: "synthetic-user" },
    options: { accountID: "123456" },
  },
  {
    kind: "dash0",
    credentials: { bearerToken: "synthetic-token" },
    options: { dataset: "default" },
  },
])(
  "$kind form sends provider-specific credentials and routing without rule flags",
  async ({ kind, credentials, options }) => {
    await open();
    expect(
      [
        ...(document.getElementById("source-kind") as HTMLSelectElement)
          .options,
      ].map((o) => o.text),
    ).toEqual([
      "Prometheus",
      "Mimir",
      "Datadog",
      "New Relic",
      "Dash0",
      "VictoriaMetrics",
    ]);
    await select("source-kind", kind);
    await input("source-name", "trial");
    for (const [field, value] of Object.entries(credentials)) {
      const node = document.getElementById(
        `source-credential-${field}`,
      ) as HTMLInputElement;
      expect(node.type).toBe("password");
      expect(node.required).toBe(true);
      await input(node.id, value);
    }
    expect(document.body.textContent).not.toContain(
      "Enable recording and alert rule deployment",
    );
    await save();
    expect(onSaved).toHaveBeenCalledOnce();
    expect(requests).toHaveLength(1);
    const body = JSON.parse(String(requests[0].init?.body));
    expect(body).toMatchObject({
      kind,
      credentials,
      options,
      collectionMode: "manual",
      rulesEnabled: false,
      autoDeploy: false,
      reload: false,
    });
  },
);

test("switching provider clears credentials and recording-rule settings", async () => {
  await open();
  await input("source-credential-bearerToken", "old-private-token");
  const rules = [...document.querySelectorAll("label")]
    .find((l) => l.textContent?.includes("Enable recording and alert"))!
    .querySelector<HTMLInputElement>("input")!;
  await act(async () => rules.click());
  await select("source-kind", "datadog");
  expect(document.getElementById("source-credential-bearerToken")).toBeNull();
  await input("source-name", "trial");
  await input("source-credential-apiKey", "synthetic-api");
  await input("source-credential-applicationKey", "synthetic-app");
  await save();
  const body = JSON.parse(String(requests[0].init?.body));
  expect(body.credentials).not.toHaveProperty("bearerToken");
  expect(body.rulesEnabled).toBe(false);
  expect(JSON.stringify(body)).not.toContain("old-private-token");
});

test("editing a managed account preserves route and does not require re-entering secrets", async () => {
  await open({
    id: "trial",
    kind: "newrelic",
    endpoint: "https://api.eu.newrelic.com/graphql",
    version: 4,
    options: { accountID: "123456" },
    collectionMode: "manual",
  });
  expect(
    (document.getElementById("source-kind") as HTMLSelectElement).disabled,
  ).toBe(true);
  expect(
    (document.getElementById("source-credential-userKey") as HTMLInputElement)
      .required,
  ).toBe(false);
  await save();
  expect(requests[0].init?.method).toBe("PUT");
  expect(JSON.parse(String(requests[0].init?.body))).toMatchObject({
    expectedVersion: 4,
    options: { accountID: "123456" },
    credentials: {},
  });
});

test("Datadog native publication is an explicit opt-in without automatic deployment", async () => {
  await open();
  await select("source-kind", "datadog");
  await input("source-name", "dd-native");
  await input("source-credential-apiKey", "synthetic-api");
  await input("source-credential-applicationKey", "synthetic-app");
  const label = Array.from(document.querySelectorAll("label")).find((l) =>
    l.textContent?.includes("Enable reviewed native SLO publication"),
  );
  const checkbox = label?.querySelector<HTMLInputElement>(
    'input[type="checkbox"]',
  );
  expect(checkbox?.checked).toBe(false);
  await act(async () => checkbox?.click());
  await save();
  expect(JSON.parse(String(requests[0].init?.body))).toMatchObject({
    kind: "datadog",
    rulesEnabled: true,
    autoDeploy: false,
    reload: false,
    rulerURL: "",
  });
});

test("New Relic native publication uses an approved entity without automatic writes", async () => {
  await open();
  await select("source-kind", "newrelic");
  await input("source-name", "nr-native");
  await input("source-credential-userKey", "synthetic-user");
  const label = Array.from(document.querySelectorAll("label")).find((l) =>
    l.textContent?.includes("Enable reviewed native SLO publication"),
  );
  const checkbox = label?.querySelector<HTMLInputElement>(
    'input[type="checkbox"]',
  );
  expect(checkbox?.checked).toBe(false);
  await act(async () => checkbox?.click());
  const entities = document.getElementById(
    "source-native-entity",
  ) as HTMLSelectElement;
  expect([...entities.options].map((o) => o.value)).toEqual([
    "",
    "approved-entity",
  ]);
  expect(entities.value).toBe("approved-entity");
  await save();
  expect(JSON.parse(String(requests[0].init?.body))).toMatchObject({
    kind: "newrelic",
    rulesEnabled: true,
    nativeEntityGUID: "approved-entity",
    autoDeploy: false,
    reload: false,
    options: { accountID: "123456" },
  });
});

test.each(["", "42:7"])(
  "VictoriaMetrics connection form saves the approved route %s without rule flags",
  async (route) => {
    await open();
    await select("source-kind", "victoriametrics");
    await input("source-name", "vm-production");
    await input("source-url", "https://vm.example");
    expect(
      (
        document.getElementById(
          "source-credential-bearerToken",
        ) as HTMLInputElement
      ).required,
    ).toBe(false);
    await input("source-credential-bearerToken", "synthetic-vm-token");
    await select("source-vm-route", route);
    expect(
      [
        ...document
          .getElementById("source-vm-route")!
          .querySelectorAll("option"),
      ].map((o) => o.value),
    ).toEqual(["", "42:7"]);
    expect(document.body.textContent).not.toContain(
      "Enable recording and alert rule deployment",
    );
    await save();
    expect(onSaved).toHaveBeenCalledOnce();
    expect(JSON.parse(String(requests[0].init?.body))).toMatchObject({
      kind: "victoriametrics",
      url: "https://vm.example",
      credentials: { bearerToken: "synthetic-vm-token" },
      options: route ? { accountID: "42", projectID: "7" } : {},
      rulesEnabled: false,
      autoDeploy: false,
      reload: false,
    });
  },
);
