import { expect, test } from "vitest";
import {
  draftFromSLOTemplate,
  extensionRequestInputs,
  type SLOTemplate,
} from "./slo-templates";
import { sloFromMeasurementDraft } from "./measurement";
const template: SLOTemplate = {
  id: "agent-evaluation-pass",
  revision: 1,
  digest: "abc",
  category: "agent",
  sli: "evaluation-quality",
  title: "Agent evaluation",
  description: "Example",
  eligibleEvents: "Evaluated tasks",
  goodEvents: "Pass verdicts",
  requirements: ["Version the rubric"],
  manifest: {
    apiVersion: "nines.dev/v1alpha1",
    kind: "ServiceLevelObjective",
    metadata: { name: "agent-evaluation-pass", project: "production" },
    spec: {
      displayName: "Agent evaluation",
      service: "agent",
      source: "primary",
      target: 0.99,
      lifecycle: "experimental",
      windowSeconds: 2419200,
      stepSeconds: 60,
      measurement: {
        type: "availability",
        metric: "agent_evaluations_total",
        labels: { service: "agent", rubric: "v1" },
        successLabels: { verdict: "pass" },
        countFrom: "counter",
        scrapeIntervalSeconds: 15,
        observationPoint: "Terminal evaluation verdicts",
      },
    },
  },
};
test("templates become editable SLO-owned measurements with the complete denominator", () => {
  const draft = draftFromSLOTemplate(template);
  const slo = sloFromMeasurementDraft(draft, "99", "28d", []);
  expect(slo.spec.measurement).toEqual(template.manifest.spec.measurement);
  expect(slo.spec.measurement?.labels).not.toHaveProperty("verdict");
  expect(slo.spec.measurementBinding).toBeUndefined();
  draft.population[0].value = "edited";
  expect(template.manifest.spec.measurement?.labels.service).toBe("agent");
});
test("latency template uses the threshold and classic histogram family without success labels", () => {
  const latency = structuredClone(template);
  latency.manifest.spec.measurement = {
    type: "latency",
    metric: "inference_time_to_first_token_seconds",
    labels: { service: "inference" },
    thresholdSeconds: 0.5,
    scrapeIntervalSeconds: 15,
    observationPoint: "Streams reaching a first token",
  };
  const draft = draftFromSLOTemplate(latency);
  expect(draft.measurement).toBe("latency");
  expect(draft.threshold).toBe("0.5");
  expect(draft.success).toEqual([]);
  expect(
    sloFromMeasurementDraft(draft, "99", "28d", []).spec.measurement,
  ).toEqual(latency.manifest.spec.measurement);
});

test("extension parameters preserve typed numbers and equality labels without inventing defaults", () => {
  const extension = {
    ...template,
    inputs: {
      threshold: {
        field: "thresholdSeconds",
        type: "number" as const,
        required: true,
        description: "Duration",
      },
      population: {
        field: "labels",
        type: "labels" as const,
        required: true,
        description: "Cohort",
      },
      target: {
        field: "target",
        type: "number" as const,
        required: false,
        description: "Target",
        default: 0.99,
      },
    },
  };
  expect(
    extensionRequestInputs(extension, {
      threshold: "0.5",
      population: [{ label: "rubric", value: "v1" }],
    }),
  ).toEqual({ threshold: 0.5, population: { rubric: "v1" } });
  expect(() =>
    extensionRequestInputs(extension, { threshold: "not a number" }),
  ).toThrow();
  expect(() =>
    extensionRequestInputs(extension, {
      population: [{ label: "le", value: "1" }],
    }),
  ).toThrow();
});
