// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import { createTenantSession, type SLO } from "@/lib/api";
import { RecordingRules } from "./recording-rules";
const mocks = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  api: mocks.api,
}));

test("native review shows dataset and warnings, applies its digest and requires refresh", async () => {
  const plan = {
    digest: "reviewed-native-digest",
    yaml: "apiVersion: openslo/v1\nkind: SLO",
    sloRevision: 3,
    group: { name: "nines-owned-origin", interval: "", rules: [] },
    target: {
      kind: "dash0",
      connection: "dash0-production",
      dataset: "production",
      namespace: "nines-prod",
      queryURL: "https://api.eu-west-1.aws.dash0.com",
    },
    nativeSLO: {
      origin: "nines-owned-origin",
      priorVersion: "8",
      matches: false,
      warnings: [
        "Five-minute settling delay.",
        "Native check rules are not published.",
      ],
    },
  };
  mocks.api.mockReset();
  mocks.api.mockImplementation(async (path: string) =>
    path.endsWith("/plan")
      ? plan
      : {
          state: "stored",
          message: "Native definition read back.",
          evaluation: { state: "unknown", message: "Evaluation unverified." },
        },
  );
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const scope = createTenantSession("synthetic", "production", "owner");
  try {
    await act(async () =>
      root.render(
        <RecordingRules
          slo={{ id: "api-availability", revision: 3 } as SLO}
          scope={scope}
          onConnections={() => {}}
        />,
      ),
    );
    expect(host.textContent).toContain("Dash0 native SLO");
    expect(host.textContent).toContain("production");
    expect(host.textContent).toContain("Five-minute settling delay.");
    expect(host.textContent).not.toContain("Prometheus reload");
    const publish = Array.from(host.querySelectorAll("button")).find(
      (button) => button.textContent === "Publish reviewed SLO",
    );
    expect(publish).toBeDefined();
    await act(async () => publish?.click());
    expect(mocks.api.mock.calls.at(-1)?.[0]).toBe(
      "/api/slos/api-availability/rules/apply",
    );
    expect(JSON.parse(mocks.api.mock.calls.at(-1)?.[2].body)).toEqual({
      expectedDigest: "reviewed-native-digest",
    });
    expect(publish?.disabled).toBe(true);
    expect(host.textContent).toContain("Native SLO stored");
    expect(host.textContent).toContain("Evaluation unverified.");
    expect(host.textContent).toContain(
      "Refresh the plan to review the current upstream version",
    );
  } finally {
    scope.close();
    await act(async () => root.unmount());
    host.remove();
  }
});

test("Google native review names its project and immutable resource without offering deletion", async () => {
  mocks.api.mockReset();
  mocks.api.mockResolvedValue({
    digest: "google-native-digest",
    yaml: "goal: 0.999",
    sloRevision: 2,
    group: {
      name: "projects/nines-test/services/checkout/serviceLevelObjectives/nines-owned",
      interval: "",
      rules: [],
    },
    target: {
      kind: "googlecloud",
      connection: "google-checkout",
      googleProjectID: "nines-test",
      googleServiceID: "checkout",
      namespace: "nines-prod",
    },
    nativeGoogleCloud: {
      origin:
        "projects/nines-test/services/checkout/serviceLevelObjectives/nines-owned",
      matches: false,
      warnings: ["Previous native revisions are retained."],
    },
  });
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const scope = createTenantSession("synthetic", "production", "viewer");
  try {
    await act(async () =>
      root.render(
        <RecordingRules
          slo={{ id: "checkout", revision: 2 } as SLO}
          scope={scope}
          onConnections={() => {}}
        />,
      ),
    );
    expect(host.textContent).toContain("Google Cloud native SLO");
    expect(host.textContent).toContain("nines-test");
    expect(host.textContent).toContain(
      "Previous native revisions are retained.",
    );
    expect(host.textContent).toContain("Google Cloud SLO document");
    expect(host.textContent).not.toContain("OpenSLO document");
    expect(host.textContent).not.toContain("Disable reviewed SLO");
    const verify = Array.from(host.querySelectorAll("button")).find(
      (button) => button.textContent === "Verify provider removal",
    );
    expect(verify?.disabled).toBe(true);
  } finally {
    scope.close();
    await act(async () => root.unmount());
    host.remove();
  }
});

test("Datadog native review publishes a digest and exposes the verified provider mapping", async () => {
  const plan = {
    digest: "dd-reviewed",
    yaml: "type: metric",
    sloRevision: 2,
    group: { name: "nines-owned", rules: [] },
    target: {
      kind: "datadog",
      connection: "dd",
      namespace: "team",
      queryURL: "https://api.uk1.datadoghq.com",
    },
    nativeDatadog: {
      origin: "nines-owned",
      matches: false,
      warnings: ["Native burn monitors are not published."],
    },
  };
  mocks.api.mockReset();
  mocks.api.mockImplementation(async (path: string) =>
    path.endsWith("/plan")
      ? plan
      : {
          state: "stored",
          message: "Read back.",
          nativeID: "0123456789abcdef0123456789abcdef",
          nativeURL:
            "https://app.uk1.datadoghq.com/slo/manage?slo_id=0123456789abcdef0123456789abcdef",
          evaluation: { state: "unknown", message: "Evaluation unverified." },
        },
  );
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const scope = createTenantSession("synthetic", "team", "owner");
  try {
    await act(async () =>
      root.render(
        <RecordingRules
          slo={{ id: "availability", revision: 2 } as SLO}
          scope={scope}
          onConnections={() => {}}
        />,
      ),
    );
    expect(host.textContent).toContain("Datadog native SLO");
    expect(host.textContent).toContain("Datadog SLO document");
    expect(host.textContent).not.toContain("OpenSLO document");
    const publish = Array.from(host.querySelectorAll("button")).find(
      (b) => b.textContent === "Publish reviewed SLO",
    )!;
    await act(async () => publish.click());
    expect(JSON.parse(mocks.api.mock.calls.at(-1)?.[2].body)).toEqual({
      expectedDigest: "dd-reviewed",
    });
    expect(publish.disabled).toBe(true);
    const link = host.querySelector<HTMLAnchorElement>(
      'a[href*="app.uk1.datadoghq.com"]',
    );
    expect(link?.href).toContain("slo_id=0123456789abcdef0123456789abcdef");
    expect(host.textContent).toContain("Evaluation unverified.");
    expect(host.textContent).not.toContain("Disable reviewed SLO");
  } finally {
    scope.close();
    await act(async () => root.unmount());
    host.remove();
  }
});

test("New Relic review shows entity routing and exposes generated SLI metric queries", async () => {
  const queries = {
    goodEvents: {
      nrql: "FROM Metric SELECT sum(newrelic.sli.good)",
      accountId: 123456,
    },
    validEvents: {
      nrql: "FROM Metric SELECT sum(newrelic.sli.valid)",
      accountId: 123456,
    },
    indicator: {
      nrql: "FROM Metric SELECT sum(newrelic.sli.good)/sum(newrelic.sli.valid)",
      accountId: 123456,
    },
  };
  const plan = {
    digest: "nr-reviewed",
    yaml: "events: {}",
    sloRevision: 1,
    group: { name: "nines-owned", rules: [] },
    target: {
      kind: "newrelic",
      connection: "nr",
      namespace: "team",
      newRelicAccountID: "123456",
      newRelicEntityGUID: "approved-entity",
    },
    nativeNewRelic: {
      origin: "nines-owned",
      entityGUID: "approved-entity",
      matches: false,
      warnings: ["Native alerts are not created."],
    },
  };
  mocks.api.mockReset();
  mocks.api.mockImplementation(async (path: string) =>
    path.endsWith("/plan")
      ? plan
      : {
          state: "stored",
          message: "Read back.",
          nativeID: "nr-native",
          nativeURL:
            "https://one.eu.newrelic.com/redirect/entity/synthetic-guid",
          nativeQueries: queries,
          evaluation: { state: "unknown", message: "Evaluation unverified." },
        },
  );
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const scope = createTenantSession("synthetic", "team", "owner");
  try {
    await act(async () =>
      root.render(
        <RecordingRules
          slo={{ id: "availability", revision: 1 } as SLO}
          scope={scope}
          onConnections={() => {}}
        />,
      ),
    );
    expect(host.textContent).toContain("New Relic native SLO");
    expect(host.textContent).toContain("approved-entity");
    expect(host.textContent).toContain("New Relic SLO document");
    expect(host.textContent).not.toContain("OpenSLO document");
    const publish = Array.from(host.querySelectorAll("button")).find(
      (b) => b.textContent === "Publish reviewed SLO",
    )!;
    await act(async () => publish.click());
    expect(JSON.parse(mocks.api.mock.calls.at(-1)?.[2].body)).toEqual({
      expectedDigest: "nr-reviewed",
    });
    expect(publish.disabled).toBe(true);
    expect(host.querySelector('a[href*="one.eu.newrelic.com"]')).not.toBeNull();
    expect(host.textContent).toContain("Inspect native SLI metrics");
    expect(host.textContent).toContain(
      "FROM Metric SELECT sum(newrelic.sli.good)",
    );
    expect(host.textContent).toContain("Evaluation unverified.");
    expect(host.textContent).not.toContain("Disable reviewed SLO");
  } finally {
    scope.close();
    await act(async () => root.unmount());
    host.remove();
  }
});
