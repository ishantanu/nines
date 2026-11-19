import { expect, test } from "vitest";
import { goalWithMeasurement, goalWithExclusions } from "./intent-measurement";
import { measurementDraft } from "@/lib/measurement";
import type { IntentDocument } from "@/lib/intent";
const goal: IntentDocument = {
  apiVersion: "nines.dev/v1alpha1",
  kind: "ServiceLevelObjectiveIntent",
  metadata: { name: "api", project: "p" },
  spec: {
    description: "API should succeed",
    service: "api",
    source: "mimir",
    outcome: "availability",
    population: { environment: "production" },
    target: 0.999,
    windowSeconds: 2419200,
    noDataPolicy: "unknown",
  },
};
test("intent measurement preserves explicit scope and adds no binding lifecycle", () => {
  const d = measurementDraft();
  d.scrape = "15";
  const result = goalWithMeasurement(goal, d);
  expect(result.spec.measurement?.labels).toEqual(goal.spec.population);
  expect(result.spec.service).toBe("api");
  expect(result.spec.source).toBe("mimir");
  expect(result.spec.stepSeconds).toBe(60);
  expect(result.metadata).toEqual(goal.metadata);
  expect(result.spec.measurement?.type).toBe("availability");
  expect(goal.spec.measurement).toBeUndefined();
});
test("latency goal supplies the threshold and discards counter-only success criteria", () => {
  const latency = {
    ...goal,
    spec: { ...goal.spec, outcome: "latency", thresholdSeconds: 0.3 },
  };
  const d = measurementDraft();
  d.scrape = "15";
  d.threshold = "999";
  const result = goalWithMeasurement(latency, d);
  expect(result.spec.measurement?.thresholdSeconds).toBe(0.3);
  expect(result.spec.measurement?.successLabels).toBeUndefined();
});

test("goals of every ratio and threshold type compile to the matching measurement", () => {
  const freshness = {
    ...goal,
    spec: { ...goal.spec, outcome: "freshness", thresholdSeconds: 300 },
  };
  const d = measurementDraft();
  d.scrape = "15";
  d.success = [{ label: "verdict", value: "pass" }];
  const fresh = goalWithMeasurement(freshness, d);
  expect(fresh.spec.measurement?.type).toBe("freshness");
  expect(fresh.spec.measurement?.thresholdSeconds).toBe(300);
  expect(fresh.spec.measurement?.successLabels).toBeUndefined();
  for (const outcome of ["correctness", "durability", "coverage"]) {
    const result = goalWithMeasurement(
      { ...goal, spec: { ...goal.spec, outcome } },
      d,
    );
    expect(result.spec.measurement?.type).toBe(outcome);
    expect(result.spec.measurement?.successLabels).toEqual({
      verdict: "pass",
    });
    expect(result.spec.measurement?.thresholdSeconds).toBeUndefined();
  }
});

test("observation exclusions distinguish absent, none and boundaries without changing the measurement", () => {
  const original = goalWithMeasurement(goal, measurementDraft());
  const none = goalWithExclusions(original, "none", "");
  expect(none.spec.exclusions).toEqual({ none: true });
  expect(none.spec.measurement).toBe(original.spec.measurement);
  const notes = goalWithExclusions(
    none,
    "boundaries",
    "Requests never reaching the observer.\nAbandoned requests before completion.",
  );
  expect(notes.spec.exclusions?.observationBoundaries).toHaveLength(2);
  expect(notes.spec.target).toBe(goal.spec.target);
  expect(
    goalWithExclusions(notes, "not_recorded", "").spec.exclusions,
  ).toBeUndefined();
  for (const value of [" ", "same\n SAME ", "x".repeat(501)])
    expect(() => goalWithExclusions(original, "boundaries", value)).toThrow();
});
