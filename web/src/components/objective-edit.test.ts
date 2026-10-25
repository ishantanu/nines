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

test("lifecycle edits preserve identity, optional defaults and intent provenance", () => {
  const before = definitionFromSLO({
    id: "api",
    project: "p",
    service: "api",
    name: "API",
    target: 0.99,
    windowSeconds: 3600,
    stepSeconds: 60,
  });
  expect(objectiveDraft(before).lifecycle).toBe("committed");
  expect(
    objectiveDefinition(before, objectiveDraft(before)).spec.lifecycle,
  ).toBeUndefined();
  before.spec.lifecycle = "aspirational";
  before.spec.intent = {
    lifecycle: "aspirational",
    description: "API requests succeed",
    service: "api",
    source: "primary",
    outcome: "availability",
    population: { service: "api" },
    target: 0.99,
    windowSeconds: 3600,
    noDataPolicy: "unknown",
  };
  const draft = objectiveDraft(before);
  expect(draft.lifecycle).toBe("aspirational");
  draft.lifecycle = "committed";
  const edited = objectiveDefinition(before, draft);
  expect(edited.metadata.name).toBe("api");
  expect(edited.spec.lifecycle).toBe("committed");
  expect(edited.spec.intent?.lifecycle).toBe("committed");
  expect(before.spec.lifecycle).toBe("aspirational");
});

test("quality and budget edits preserve measurement and intent agreement", () => {
  const before = definitionFromSLO({
    id: "api",
    project: "p",
    service: "api",
    name: "API",
    target: 0.99,
    windowSeconds: 3600,
    stepSeconds: 60,
    quality: { minEvents: 100, cohortLabels: ["region"] },
    errorBudgetPolicy: {
      owner: "oncall",
      runbookURL: "https://example.test/runbook",
      reviewBelow: 0.25,
    },
    intent: {
      description: "API success",
      service: "api",
      source: "primary",
      outcome: "availability",
      population: { service: "api" },
      target: 0.99,
      windowSeconds: 3600,
      noDataPolicy: "unknown",
    },
  });
  const draft = objectiveDraft(before);
  draft.quality = { minEvents: 200, cohortLabels: ["region", "tier"] };
  draft.errorBudgetPolicy = { ...draft.errorBudgetPolicy!, owner: "new-owner" };
  const after = objectiveDefinition(before, draft);
  expect(after.metadata).toEqual(before.metadata);
  expect(after.spec.quality?.minEvents).toBe(200);
  expect(after.spec.intent?.quality).toEqual(after.spec.quality);
  expect(after.spec.intent?.errorBudgetPolicy).toEqual(
    after.spec.errorBudgetPolicy,
  );
  expect(before.spec.quality?.minEvents).toBe(100);
});
