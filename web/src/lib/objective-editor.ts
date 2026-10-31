import { normalizeBurnWindows } from "./definition-yaml";
import { parse } from "yaml";
import { RESOURCE_API_VERSION, type SLODefinition } from "./definitions";
import type { SLO, BurnRatePolicy } from "./api";
import { bindingStarter } from "./topology";
import {
  bindingFromDraft,
  measurementDraft,
  sloFromMeasurementDraft,
  type MeasurementDraft,
} from "./measurement";
import { duration, durationSeconds } from "./history";

export interface InlineObjectiveDraft {
  base: SLODefinition;
  measurement: MeasurementDraft;
  target: string;
  window: string;
  lifecycle: NonNullable<SLO["lifecycle"]>;
  quality?: SLO["quality"];
  errorBudgetPolicy?: SLO["errorBudgetPolicy"];
  policies: BurnRatePolicy[];
}
export function newObjectiveDefinition(): SLODefinition {
  return {
    apiVersion: RESOURCE_API_VERSION,
    kind: "ServiceLevelObjective",
    metadata: { name: "", project: "production" },
    spec: {
      displayName: "",
      service: "",
      source: "primary",
      target: 0.999,
      windowSeconds: 28 * 86400,
      stepSeconds: 60,
      measurement: {
        type: "availability",
        metric: "",
        labels: {},
        successLabels: { status: "200" },
        countFrom: "counter",
        scrapeIntervalSeconds: 15,
        observationPoint: "",
      },
    },
  };
}
export function inlineObjectiveDraft(
  base: SLODefinition,
): InlineObjectiveDraft {
  const s = base.spec,
    m = s.measurement;
  if (!m)
    throw Error(
      "This SLO uses custom queries or a shared measurement. Edit it in Settings or YAML; guided measurement cannot replace it implicitly.",
    );
  if (s.providerWindow || s.measurementBinding || s.goodQuery || s.totalQuery)
    throw Error(
      "The inline measurement conflicts with custom queries or a binding. Correct the YAML before opening the Measurement tab.",
    );
  return {
    base,
    measurement: measurementDraft({
      ...bindingStarter(s.source || "primary", m.metric, m.labels),
      metadata: base.metadata,
      spec: {
        ...m,
        measurement: m.type,
        displayName: s.displayName,
        source: s.source || "primary",
        service: s.service,
        stepSeconds: s.stepSeconds,
      },
    }),
    target: String(Number((s.target * 100).toPrecision(12))),
    window: duration(s.windowSeconds),
    lifecycle: s.lifecycle ?? "committed",
    quality: s.quality,
    errorBudgetPolicy: s.errorBudgetPolicy,
    policies:
      s.policies ??
      (s.policy
        ? [
            {
              ...s.policy,
              name: s.policy.name || "default",
              severity: s.policy.severity || "critical",
            },
          ]
        : []),
  };
}
export function inlineObjectiveDefinition(
  d: InlineObjectiveDraft,
  validate = true,
): SLODefinition {
  let generated: SLODefinition;
  if (validate)
    generated = sloFromMeasurementDraft(
      d.measurement,
      d.target,
      d.window,
      d.policies,
    );
  else {
    // Incomplete text fields remain drafts. Ambiguous filters/invalid durations
    // cannot be represented in YAML and must be corrected before changing tabs.
    const b = bindingFromDraft(d.measurement, false),
      s = b.spec;
    generated = {
      apiVersion: RESOURCE_API_VERSION,
      kind: "ServiceLevelObjective",
      metadata: b.metadata,
      spec: {
        displayName: s.displayName,
        service: s.service,
        source: s.source,
        target: Number((Number(d.target) / 100).toPrecision(12)),
        windowSeconds: durationSeconds(d.window),
        stepSeconds: s.stepSeconds,
        measurement: {
          type: s.measurement,
          metric: s.metric,
          labels: s.labels,
          successLabels: s.successLabels,
          countFrom: s.countFrom,
          thresholdSeconds: s.thresholdSeconds,
          scrapeIntervalSeconds: Number(d.measurement.scrape),
          observationPoint: s.observationPoint,
        },
      },
    };
  }
  generated.metadata = { ...d.base.metadata, ...generated.metadata };
  generated.spec.measurement = {
    ...d.base.spec.measurement,
    ...generated.spec.measurement,
  } as SLO["measurement"];
  generated.spec.target = Number((Number(d.target) / 100).toPrecision(12));
  if (!d.base.spec.source && generated.spec.source === "primary")
    generated.spec.source = d.base.spec.source;
  if (
    d.base.spec.measurement?.countFrom === undefined &&
    generated.spec.measurement?.countFrom === "counter"
  )
    generated.spec.measurement.countFrom = undefined;
  const spec: SLODefinition["spec"] = {
    ...d.base.spec,
    ...generated.spec,
    providerWindow: undefined,
    goodQuery: undefined,
    totalQuery: undefined,
    measurementBinding: undefined,
    policy: undefined,
    policies: d.policies.length ? d.policies : undefined,
    quality: d.quality,
    errorBudgetPolicy: d.errorBudgetPolicy,
    lifecycle:
      d.lifecycle === "committed" && !d.base.spec.lifecycle
        ? undefined
        : d.lifecycle,
  };
  if (
    d.base.spec.policy &&
    !d.base.spec.policies &&
    JSON.stringify(d.policies) ===
      JSON.stringify([
        {
          ...d.base.spec.policy,
          name: d.base.spec.policy.name || "default",
          severity: d.base.spec.policy.severity || "critical",
        },
      ])
  ) {
    spec.policy = d.base.spec.policy;
    spec.policies = undefined;
  }
  if (d.base.spec.intent)
    spec.intent = {
      ...d.base.spec.intent,
      lifecycle: spec.lifecycle,
      quality: spec.quality,
      errorBudgetPolicy: spec.errorBudgetPolicy,
      service: spec.service,
      source: spec.source ?? "",
      target: spec.target,
      windowSeconds: spec.windowSeconds,
      stepSeconds: spec.stepSeconds,
      measurement: spec.measurement,
      outcome: spec.measurement!.type,
      population: spec.measurement!.labels,
      thresholdSeconds: spec.measurement!.thresholdSeconds,
      policy: spec.policy,
      policies: spec.policies,
    };
  return { ...d.base, ...generated, spec };
}
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);
const finite = (v: unknown) => typeof v === "number" && Number.isFinite(v);
const optional = (v: unknown, check: (v: unknown) => boolean) =>
  v === undefined || check(v);
const strings = (v: unknown) =>
  record(v) && Object.values(v).every((x) => typeof x === "string");
const policy = (v: unknown) =>
  record(v) &&
  [
    "shortWindowSeconds",
    "longWindowSeconds",
    "burnRate",
    "forSeconds",
    "minCoverage",
  ].every((k) => finite(v[k])) &&
  optional(v.minEvents, finite) &&
  optional(v.name, (x) => typeof x === "string") &&
  optional(v.severity, (x) =>
    ["critical", "warning", "info"].includes(String(x)),
  );
export function parseObjectiveDefinition(text: string): SLODefinition {
  const v: unknown = normalizeBurnWindows(parse(text));
  if (
    !record(v) ||
    v.apiVersion !== RESOURCE_API_VERSION ||
    v.kind !== "ServiceLevelObjective" ||
    !record(v.metadata) ||
    !record(v.spec)
  )
    throw Error(
      `Expected apiVersion ${RESOURCE_API_VERSION} and kind ServiceLevelObjective with metadata and spec.`,
    );
  const s = v.spec,
    m = s.measurement;
  if (
    ![v.metadata.name, v.metadata.project, s.displayName, s.service].every(
      (x) => typeof x === "string",
    ) ||
    ![s.target, s.windowSeconds, s.stepSeconds].every(finite) ||
    !optional(s.source, (x) => typeof x === "string") ||
    !optional(s.goodQuery, (x) => typeof x === "string") ||
    !optional(s.totalQuery, (x) => typeof x === "string") ||
    !optional(s.lifecycle, (x) =>
      ["experimental", "aspirational", "committed", "deprecated"].includes(
        String(x),
      ),
    ) ||
    !optional(s.policies, (x) => Array.isArray(x) && x.every(policy)) ||
    !optional(s.policy, policy)
  )
    throw Error(
      "Correct the SLO metadata, target, durations and policies in YAML before opening the form.",
    );
  if (
    m !== undefined &&
    (!record(m) ||
      !["availability", "latency"].includes(String(m.type)) ||
      typeof m.metric !== "string" ||
      !strings(m.labels) ||
      !optional(m.successLabels, strings) ||
      !finite(m.scrapeIntervalSeconds) ||
      typeof m.observationPoint !== "string" ||
      !optional(m.thresholdSeconds, finite) ||
      !optional(m.countFrom, (x) =>
        ["counter", "histogram"].includes(String(x)),
      ))
  )
    throw Error(
      "Correct the inline measurement fields in YAML before opening the form.",
    );
  if (
    !optional(
      s.providerWindow,
      (x) =>
        record(x) &&
        typeof x.goodSelector === "string" &&
        typeof x.totalSelector === "string" &&
        finite(x.scrapeIntervalSeconds),
    ) ||
    !optional(
      s.measurementBinding,
      (x) =>
        record(x) &&
        typeof x.name === "string" &&
        finite(x.revision) &&
        typeof x.digest === "string",
    ) ||
    !optional(
      s.quality,
      (x) =>
        record(x) &&
        optional(x.minEvents, finite) &&
        optional(x.maxSampleAgeSeconds, finite) &&
        optional(
          x.cohortLabels,
          (y) => Array.isArray(y) && y.every((z) => typeof z === "string"),
        ),
    ) ||
    !optional(
      s.errorBudgetPolicy,
      (x) =>
        record(x) &&
        typeof x.owner === "string" &&
        typeof x.runbookURL === "string" &&
        finite(x.reviewBelow) &&
        optional(x.onExhausted, (y) =>
          ["blocked", "review_required"].includes(String(y)),
        ),
    ) ||
    !optional(s.intent, record)
  )
    throw Error(
      "Correct the measurement references, quality, budget or intent fields in YAML before opening the form.",
    );
  return v as unknown as SLODefinition;
}
