import { expect, test } from "vitest";
import { stringify } from "yaml";
import {
  inlineObjectiveDraft,
  inlineObjectiveDefinition,
  parseObjectiveDefinition,
  newObjectiveDefinition,
} from "./objective-editor";
function definition() {
  const d = newObjectiveDefinition();
  d.metadata.name = "api";
  d.spec.displayName = "API availability";
  d.spec.service = "api";
  d.spec.measurement!.metric = "requests_total";
  d.spec.measurement!.observationPoint =
    "Every terminal request observed at the server.";
  return d;
}
test("guided round trip preserves omitted defaults, exact targets and legacy alert policy", () => {
  const d = definition();
  delete d.spec.source;
  delete d.spec.measurement!.countFrom;
  d.spec.policy = {
    shortWindowSeconds: 60,
    longWindowSeconds: 3600,
    burnRate: 3,
    forSeconds: 0,
    minCoverage: 1,
  };
  const result = inlineObjectiveDefinition(inlineObjectiveDraft(d));
  expect(JSON.parse(JSON.stringify(result))).toEqual(
    JSON.parse(JSON.stringify(d)),
  );
  expect(result.spec.target).toBe(0.999);
});
test("measurement changes retain and synchronize intent provenance and policy context", () => {
  const d = definition();
  d.spec.intent = {
    description: "API attempts succeed",
    service: "api",
    source: "primary",
    outcome: "availability",
    population: {},
    measurement: d.spec.measurement,
    stepSeconds: 60,
    target: 0.999,
    windowSeconds: d.spec.windowSeconds,
    noDataPolicy: "unknown",
    journey: {
      name: "checkout",
      step: "pay",
      expectedRevision: 2,
      required: true,
    },
  };
  const draft = inlineObjectiveDraft(d);
  draft.target = "99.5";
  draft.window = "7d";
  draft.measurement.metric = "checkout_requests_total";
  draft.measurement.population = [{ label: "region", value: "eu" }];
  draft.quality = { minEvents: 100 };
  draft.lifecycle = "experimental";
  const result = inlineObjectiveDefinition(draft);
  expect(result.spec.intent).toMatchObject({
    description: "API attempts succeed",
    target: 0.995,
    windowSeconds: 604800,
    lifecycle: "experimental",
    quality: { minEvents: 100 },
    population: { region: "eu" },
    measurement: { metric: "checkout_requests_total" },
    journey: d.spec.intent.journey,
  });
  expect(result.spec.intent!.measurement).toEqual(result.spec.measurement);
  expect(d.spec.measurement!.metric).toBe("requests_total");
});
test("custom queries never silently turn into a guessed inline measurement", () => {
  const d = definition();
  delete d.spec.measurement;
  d.spec.goodQuery = "vector(99)";
  d.spec.totalQuery = "vector(100)";
  expect(() => inlineObjectiveDraft(d)).toThrow("custom queries");
  d.spec.measurement = definition().spec.measurement;
  expect(() => inlineObjectiveDraft(d)).toThrow("conflicts");
});
test("incomplete drafts can be viewed in YAML; ambiguous filters cannot lose data through conversion", () => {
  const draft = inlineObjectiveDraft(newObjectiveDefinition());
  expect(inlineObjectiveDefinition(draft, false).metadata.name).toBe("");
  expect(() => inlineObjectiveDefinition(draft)).toThrow("required");
  draft.measurement.population = [
    { label: "region", value: "eu" },
    { label: "region", value: "us" },
  ];
  expect(() => inlineObjectiveDefinition(draft, false)).toThrow("repeated");
});
test("invalid YAML shapes remain editable instead of crashing or resetting form fields", () => {
  const d = definition();
  for (const text of [
    "null",
    stringify({ ...d, apiVersion: "wrong" }),
    stringify({ ...d, spec: { ...d.spec, policies: { name: "oops" } } }),
    stringify({
      ...d,
      spec: {
        ...d.spec,
        measurement: { ...d.spec.measurement, labels: ["region"] },
      },
    }),
    stringify({ ...d, spec: { ...d.spec, quality: { cohortLabels: 12 } } }),
    stringify({ ...d, spec: { ...d.spec, target: Infinity } }),
  ])
    expect(() => parseObjectiveDefinition(text)).toThrow();
  expect(parseObjectiveDefinition(stringify(d))).toEqual(d);
});
