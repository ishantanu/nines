import { expect, test } from "vitest";
import { objectiveDraft, objectiveDefinition } from "./objective-create";
import { definitionFromSLO } from "@/lib/definitions";

test("editing custom measurement fields preserves the SLO identity", () => {
  const before = definitionFromSLO({
    id: "same-objective",
    project: "p",
    service: "api",
    name: "API",
    target: 0.999,
    windowSeconds: 86400,
    stepSeconds: 60,
    providerWindow: {
      goodSelector: 'requests_total{code="200"}',
      totalSelector: "requests_total",
      scrapeIntervalSeconds: 15,
    },
  });
  const draft = objectiveDraft(before);
  draft.goodQuery = 'playground_requests_total{code="200"}';
  draft.totalQuery = "playground_requests_total";
  draft.scrape = "5";
  draft.step = "30s";
  draft.source = "other";
  draft.service = "checkout";
  const edited = objectiveDefinition(before, draft);
  expect(edited.metadata.name).toBe(before.metadata.name);
  expect(edited.spec).toMatchObject({
    service: "checkout",
    source: "other",
    stepSeconds: 30,
    providerWindow: {
      goodSelector: draft.goodQuery,
      totalSelector: draft.totalQuery,
      scrapeIntervalSeconds: 5,
    },
  });
  expect(before.spec.providerWindow?.totalSelector).toBe("requests_total");
});
test("switching a saved measurement to custom queries explicitly detaches provenance", () => {
  const before = definitionFromSLO({
    id: "bound",
    project: "p",
    service: "api",
    name: "API",
    target: 0.99,
    windowSeconds: 3600,
    stepSeconds: 60,
    measurementBinding: {
      name: "requests",
      revision: 1,
      digest: "a".repeat(64),
    },
  });
  const draft = objectiveDraft(before);
  expect(objectiveDefinition(before, draft).spec.measurementBinding).toEqual(
    before.spec.measurementBinding,
  );
  draft.customMeasurement = true;
  draft.goodQuery = "vector(90)";
  draft.totalQuery = "vector(100)";
  expect(
    objectiveDefinition(before, draft).spec.measurementBinding,
  ).toBeUndefined();
  expect(objectiveDefinition(before, draft).spec.intent).toBeUndefined();
  expect(before.spec.measurementBinding).toBeDefined();
});
