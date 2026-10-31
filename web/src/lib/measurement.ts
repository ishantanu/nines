import {
  bindingStarter,
  type BindingManifest,
  type Discovery,
} from "./topology";
import { api, type RequestScope, type BurnRatePolicy } from "./api";
import type { SLODefinition } from "./definitions";
import { durationSeconds } from "./history";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";

export type LabelFilter = { label: string; value: string };
export type MeasurementDraft = {
  name: string;
  project: string;
  displayName: string;
  service: string;
  source: string;
  measurement: "availability" | "latency";
  metric: string;
  population: LabelFilter[];
  success: LabelFilter[];
  countFrom: "counter" | "histogram";
  threshold: string;
  step: string;
  scrape: string;
  observationPoint: string;
};
export function measurementDraft(
  binding: BindingManifest = bindingStarter(),
): MeasurementDraft {
  const s = binding.spec;
  return {
    name: binding.metadata.name,
    project: binding.metadata.project,
    displayName: s.displayName,
    service: s.service,
    source: s.source,
    measurement: s.measurement,
    metric: s.metric,
    population: Object.entries(s.labels).map(([label, value]) => ({
      label,
      value,
    })),
    success: Object.entries(s.successLabels ?? {}).map(([label, value]) => ({
      label,
      value,
    })),
    countFrom: s.countFrom ?? "counter",
    threshold: String(s.thresholdSeconds ?? ""),
    step: String(s.stepSeconds),
    scrape: String(s.scrapeIntervalSeconds ?? ""),
    observationPoint: s.observationPoint,
  };
}
export function filterLabels(rows: LabelFilter[]): Record<string, string> {
  const result: Record<string, string> = Object.create(null);
  for (const { label, value } of rows) {
    if (!label && !value) continue;
    if (
      !/^[a-zA-Z_][a-zA-Z0-9_]{0,99}$/.test(label) ||
      label.startsWith("__") ||
      label === "le"
    )
      throw new Error("Use valid, non-reserved metric label names.");
    if (Object.hasOwn(result, label))
      throw new Error(`Label ${label} is repeated.`);
    result[label] = value;
  }
  return result;
}
export function bindingFromDraft(
  d: MeasurementDraft,
  validate = true,
): BindingManifest {
  const b = bindingStarter();
  b.metadata = { name: d.name, project: d.project };
  const labels = filterLabels(d.population);
  const success =
    d.measurement === "availability" ? filterLabels(d.success) : {};
  const step = Number(d.step),
    scrape = Number(d.scrape);
  if (
    validate &&
    (!Number.isInteger(step) ||
      step < 1 ||
      step > 3600 ||
      !Number.isInteger(scrape) ||
      scrape < 1 ||
      step % scrape !== 0 ||
      step < scrape * 2)
  )
    throw new Error(
      "The measurement interval must be 1–3600 seconds and contain at least two evenly spaced scrapes.",
    );
  b.spec = {
    displayName: d.displayName,
    service: d.service,
    source: d.source,
    measurement: d.measurement,
    metric: d.metric,
    labels,
    stepSeconds: step,
    scrapeIntervalSeconds: scrape,
    observationPoint: d.observationPoint,
  };
  if (d.measurement === "availability") {
    if (validate && !Object.keys(success).length)
      throw new Error(
        "Specify at least one label that identifies successful events.",
      );
    if (Object.keys(success).some((k) => Object.hasOwn(labels, k)))
      throw new Error(
        "Success labels must add to the population filters, without repeating or overriding them.",
      );
    b.spec.countFrom = d.countFrom;
    b.spec.successLabels = success;
  } else {
    const threshold = Number(d.threshold);
    if (
      validate &&
      (!Number.isFinite(threshold) || threshold <= 0 || threshold > 86400)
    )
      throw new Error(
        "Enter a latency threshold in seconds greater than zero and at most 86400.",
      );
    b.spec.thresholdSeconds = threshold;
  }
  return b;
}
export interface MeasurementPlan {
  action: string;
  expectedRevision: number;
  digest: string;
  manifest: SLODefinition;
}
export interface MeasurementPreview {
  compiled: {
    definition: BindingManifest;
    binding: { name: string; revision: number; digest: string };
    evidence: Discovery;
    warnings: string[];
  };
  plan: MeasurementPlan;
}
export async function prepareMeasuredObjective(
  scope: RequestScope,
  binding: { name: string; revision: number },
  objective: {
    name: string;
    displayName: string;
    target: string;
    window: string;
    policies: BurnRatePolicy[];
  },
  signal: AbortSignal,
): Promise<MeasurementPreview> {
  const target = Number((Number(objective.target) / 100).toPrecision(12));
  if (!Number.isFinite(target) || target <= 0 || target >= 1)
    throw new Error("Target must be greater than 0 and less than 100 percent.");
  const windowSeconds = durationSeconds(objective.window);
  if (windowSeconds < 1 || windowSeconds > 31 * 86400)
    throw new Error(
      "Use a rolling window greater than zero and no longer than 31 days.",
    );
  const compiled = await api<
    MeasurementPreview["compiled"] & { manifest: SLODefinition }
  >(`/api/bindings/${encodeURIComponent(binding.name)}/compile`, scope, {
    method: "POST",
    signal,
    body: JSON.stringify({
      expectedRevision: binding.revision,
      name: objective.name,
      displayName: objective.displayName,
      target,
      windowSeconds,
      ...(objective.policies.length ? { policies: objective.policies } : {}),
    }),
  });
  signal.throwIfAborted();
  const plan = await api<MeasurementPlan>("/api/resources/plan", scope, {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/yaml" },
    body: stringify(compiled.manifest),
  });
  if (plan.manifest.kind !== "ServiceLevelObjective")
    throw new Error("Expected an SLO plan.");
  return { compiled, plan };
}

export function sloFromMeasurementDraft(
  d: MeasurementDraft,
  targetText: string,
  windowText: string,
  policies: BurnRatePolicy[],
): SLODefinition {
  for (const [label, value] of [
    ["SLO identifier", d.name],
    ["Project", d.project],
    ["Display name", d.displayName],
    ["Service", d.service],
    ["Metrics connection", d.source],
    ["Metric", d.metric],
    ["Observation point", d.observationPoint],
  ]) {
    if (!value.trim()) throw new Error(`${label} is required.`);
  }
  const contract = bindingFromDraft(d);
  const target = Number(targetText) / 100;
  if (!Number.isFinite(target) || target <= 0 || target >= 1)
    throw new Error("Target must be greater than 0 and less than 100 percent.");
  const windowSeconds = durationSeconds(windowText),
    stepSeconds = contract.spec.stepSeconds;
  if (
    windowSeconds < stepSeconds ||
    windowSeconds > 31 * 86400 ||
    windowSeconds % stepSeconds !== 0 ||
    windowSeconds / stepSeconds > 100000
  )
    throw new Error(
      `Choose a window that is a multiple of the ${stepSeconds}s measurement interval, with at most 100000 intervals and no longer than 31 days.`,
    );
  return {
    apiVersion: "nines.dev/v1alpha1",
    kind: "ServiceLevelObjective",
    metadata: contract.metadata,
    spec: {
      displayName: d.displayName,
      service: d.service,
      source: d.source,
      target,
      windowSeconds,
      stepSeconds,
      measurement: {
        type: d.measurement,
        metric: d.metric,
        labels: contract.spec.labels,
        successLabels: contract.spec.successLabels,
        countFrom: contract.spec.countFrom,
        thresholdSeconds: contract.spec.thresholdSeconds,
        scrapeIntervalSeconds: Number(d.scrape),
        observationPoint: d.observationPoint,
      },
      ...(policies.length ? { policies } : {}),
    },
  };
}
export async function prepareInlineObjective(
  scope: RequestScope,
  manifest: SLODefinition,
  signal: AbortSignal,
) {
  const compiled = await api<{
    manifest: SLODefinition;
    measurementDigest: string;
    evidence: Discovery;
    warnings: string[];
  }>("/api/slos/compile", scope, {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/yaml" },
    body: stringify(manifest),
  });
  signal.throwIfAborted();
  const plan = await api<
    MeasurementPlan & { measurementChanged?: boolean; warnings?: string[] }
  >("/api/resources/plan", scope, {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/yaml" },
    body: stringify(compiled.manifest),
  });
  return { compiled, plan };
}
