import { expect, test } from "vitest";
import { goalWithMeasurement } from "./intent-measurement";
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
