import { expect, it } from "vitest";
import { parse, stringify } from "yaml";
import { definitionFromSLO, RESOURCE_API_VERSION } from "./definitions";
it("exports a versioned SLO YAML definition without runtime fields", () => {
  const definition = definitionFromSLO({
    id: "checkout",
    project: "commerce",
    service: "payments",
    name: "Checkout success",
    target: 0.999,
    windowSeconds: 86400,
    stepSeconds: 60,
    goodQuery: "good",
    totalQuery: "total",
    policy: {
      shortWindowSeconds: 300,
      longWindowSeconds: 3600,
      burnRate: 14.4,
      forSeconds: 120,
      minCoverage: 1,
    },
  });
  const document = parse(stringify(definition));
  expect(document).toEqual({
    apiVersion: RESOURCE_API_VERSION,
    kind: "ServiceLevelObjective",
    metadata: { name: "checkout", project: "commerce" },
    spec: {
      displayName: "Checkout success",
      service: "payments",
      target: 0.999,
      windowSeconds: 86400,
      stepSeconds: 60,
      goodQuery: "good",
      totalQuery: "total",
      policy: {
        shortWindowSeconds: 300,
        longWindowSeconds: 3600,
        burnRate: 14.4,
        forSeconds: 120,
        minCoverage: 1,
      },
    },
  });
  expect(document).not.toHaveProperty("id");
  expect(document).not.toHaveProperty("createdAt");
});

it("preserves provider windows and binding lineage when exporting/editing YAML", () => {
  const providerWindow = {
    goodSelector: 'duration_seconds_bucket{le="0.25"}',
    totalSelector: "duration_seconds_count",
    scrapeIntervalSeconds: 2,
  };
  const measurementBinding = {
    name: "latency",
    revision: 2,
    digest: "a".repeat(64),
  };
  const exported = definitionFromSLO({
    id: "latency-window",
    name: "Latency",
    project: "test",
    service: "api",
    target: 0.99,
    stepSeconds: 10,
    windowSeconds: 120,
    providerWindow,
    measurementBinding,
  });
  const roundtrip = parse(stringify(exported));
  expect(roundtrip.spec.providerWindow).toEqual(providerWindow);
  expect(roundtrip.spec.measurementBinding).toEqual(measurementBinding);
  expect(roundtrip.spec).not.toHaveProperty("goodQuery");
  expect(roundtrip.spec).not.toHaveProperty("totalQuery");
});
