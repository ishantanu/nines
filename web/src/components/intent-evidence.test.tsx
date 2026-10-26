import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  IntentTemplateEvidence,
  IntentEvidence,
  IntentProgress,
} from "./intent-evidence";
import type { IntentPlan } from "@/lib/intent";

const compiled: NonNullable<IntentPlan["compiled"]> = {
  manifest: {
    apiVersion: "nines.dev/v1alpha1",
    kind: "ServiceLevelObjective",
    metadata: { name: "api", project: "commerce" },
    spec: {
      displayName: "API",
      service: "api",
      source: "production",
      target: 0.999,
      windowSeconds: 3600,
      stepSeconds: 60,
    },
  },
  binding: { name: "requests", revision: 3, digest: "binding-digest" },
  definition: {
    apiVersion: "nines.dev/v1alpha1",
    kind: "MeasurementBinding",
    metadata: { name: "requests", project: "commerce" },
    spec: {
      displayName: "Request success",
      service: "api",
      source: "production",
      measurement: "availability",
      metric: "requests_total",
      labels: { service: "api" },
      successLabels: { status: "200" },
      stepSeconds: 60,
      observationPoint: "API server completion",
    },
  },
  yaml: "",
  digest: "manifest-digest",
  warnings: ["Review retry semantics."],
  evidence: {
    from: "2026-10-22T12:00:00Z",
    to: "2026-10-22T12:15:00Z",
    fingerprint: "discovery-digest",
    warnings: ["Review retry semantics."],
  },
};
test("proposal shows observation point, exact population, evidence and unresolved assumptions", () => {
  const html = renderToStaticMarkup(
    <IntentEvidence compiled={compiled} sourceDigest="source-digest" />,
  );
  for (const value of [
    "production",
    "requests_total",
    "revision 3",
    "API server completion",
    "Measurement population",
    "2026-10-22T12:15:00Z",
    "discovery-digest",
    "source-digest",
    "binding-digest",
    "Review assumptions",
    "end-to-end user success",
  ])
    expect(html).toContain(value);
  expect(html.match(/Review retry semantics/g)).toHaveLength(1);
});
test("stream progress shows aggregate bytes and never model response content", () => {
  const html = renderToStaticMarkup(
    <IntentProgress
      progress={{ modelCall: 1, state: "receiving", bytes: 128, at: "now" }}
    />,
  );
  expect(html).toContain("Receiving model response");
  expect(html).toContain("128 bytes");
  const waiting = renderToStaticMarkup(
    <IntentProgress
      progress={{ modelCall: 2, state: "waiting", bytes: 0, at: "now" }}
    />,
  );
  expect(waiting).toContain("Waiting for model response");
  expect(waiting).not.toContain("0 bytes");
});

test("inline evidence describes the SLO measurement without a shared binding", () => {
  const inline = {
    ...compiled,
    definition: undefined,
    binding: undefined,
    measurementDigest: "slo-contract",
    manifest: {
      ...compiled.manifest,
      spec: {
        ...compiled.manifest.spec,
        measurement: {
          type: "availability" as const,
          metric: "requests_total",
          labels: { service: "api" },
          successLabels: { status: "200" },
          scrapeIntervalSeconds: 15,
          observationPoint: "API completion",
        },
      },
    },
  };
  const html = renderToStaticMarkup(<IntentEvidence compiled={inline} />);
  expect(html).toContain("Owned by this SLO");
  expect(html).toContain("slo-contract");
  expect(html).toContain("API completion");
  expect(html).not.toContain("Shared measurement");
  expect(html).not.toContain("binding-digest");
});

test("template provenance distinguishes a recipe from telemetry evidence and escapes publisher text", () => {
  const html = renderToStaticMarkup(
    <IntentTemplateEvidence
      selection={{
        templateId: "example--recipe",
        templateRevision: 1,
        templateDigest: "a".repeat(64),
        catalogDigest: "b".repeat(64),
        extension: {
          name: "<script>publisher</script>",
          version: "1.0.0",
          sha256: "c".repeat(64),
        },
        valuesDigest: "d".repeat(64),
        intentDigest: "e".repeat(64),
        at: "2026-10-25T00:00:00Z",
      }}
    />,
  );
  expect(html).toContain("Measurement template");
  expect(html).toContain("Extension artifact SHA-256");
  expect(html).toContain("Telemetry evidence and human confirmation");
  expect(html).toContain("&lt;script&gt;");
  expect(html).not.toContain("<script>");
});
