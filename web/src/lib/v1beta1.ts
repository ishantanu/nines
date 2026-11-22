// Conversion between the stored definition form (nines.dev/v1alpha1) and the
// nines.dev/v1beta1 form users read and write in the YAML tab. This mirrors
// api/v1beta1 in Go; v1beta1.test.ts checks every case in
// v1beta1-fixtures.json, which the Go tests generate, so the two cannot drift.
import type { SLODefinition } from "./definitions";

export const V1BETA1 = "nines.dev/v1beta1" as const;
const V1ALPHA1 = "nines.dev/v1alpha1" as const;

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj =>
  v !== null && typeof v === "object" && !Array.isArray(v);

// --- durations -------------------------------------------------------------

const units: [string, number][] = [
  ["d", 86_400_000],
  ["h", 3_600_000],
  ["m", 60_000],
  ["s", 1_000],
  ["ms", 1],
];

/** Milliseconds in a duration such as 28d, 1h30m or 300ms. */
export function parseDuration(text: unknown, field: string): number {
  if (
    typeof text !== "string" ||
    text.length > 32 ||
    !/^([0-9]+(d|h|m|s|ms))+$/.test(text)
  )
    throw Error(
      `${field}: ${JSON.stringify(text)} is not a duration; use units d, h, m, s or ms, for example 28d, 5m or 300ms`,
    );
  let total = 0;
  for (const [, n, unit] of text.matchAll(/([0-9]+)(ms|d|h|m|s)/g))
    total += Number(n) * (units.find(([u]) => u === unit)?.[1] ?? 0);
  if (total <= 0) throw Error(`${field}: ${text} must be longer than zero`);
  if (total > 400 * 86_400_000)
    throw Error(`${field}: ${text} is longer than 400 days`);
  return total;
}

/** The shortest exact form: 28d, 1h30m, 5m, 300ms. */
export function formatDuration(ms: number): string {
  if (ms <= 0) return "0s";
  let rest = Math.round(ms);
  let out = "";
  for (const [unit, size] of units) {
    const n = Math.floor(rest / size);
    if (n > 0) {
      out += `${n}${unit}`;
      rest -= n * size;
    }
  }
  return out;
}

const seconds = (s: number) => formatDuration(s * 1000);
function wholeSeconds(text: unknown, field: string): number {
  const ms = parseDuration(text, field);
  if (ms % 1000 !== 0)
    throw Error(`${field} must be whole seconds, not ${formatDuration(ms)}`);
  return ms / 1000;
}
const optionalSeconds = (v: unknown, def: number, field: string) =>
  v === undefined ? def : wholeSeconds(v, field);

// Go's %g for the numbers that appear in notes.
const g = (n: number) => String(n);

// --- defaults ----------------------------------------------------------------

export const DEFAULT_WINDOW_SECONDS = 28 * 86400;
export const DEFAULT_STEP_SECONDS = 60;
export const DEFAULT_SCRAPE_SECONDS = 15;

type HubPolicy = {
  name?: string;
  severity?: string;
  shortWindowSeconds: number;
  longWindowSeconds: number;
  burnRate: number;
  forSeconds: number;
  minCoverage: number;
  minEvents?: number;
  confidence?: number;
};

const standardPolicies: HubPolicy[] = [
  {
    name: "fast-burn",
    severity: "critical",
    shortWindowSeconds: 300,
    longWindowSeconds: 3600,
    burnRate: 14.4,
    forSeconds: 0,
    minCoverage: 1,
  },
  {
    name: "sustained-burn",
    severity: "critical",
    shortWindowSeconds: 1800,
    longWindowSeconds: 21600,
    burnRate: 6,
    forSeconds: 0,
    minCoverage: 1,
  },
  {
    name: "slow-burn",
    severity: "warning",
    shortWindowSeconds: 21600,
    longWindowSeconds: 259200,
    burnRate: 1,
    forSeconds: 0,
    minCoverage: 1,
  },
];

/** The standard alerts that fit an SLO, with a note for each left out. */
export function defaultPolicies(
  target: number,
  windowSeconds: number,
  stepSeconds: number,
): { policies: HubPolicy[]; notes: string[] } {
  const policies: HubPolicy[] = [];
  const notes: string[] = [];
  for (const p of standardPolicies) {
    if (p.longWindowSeconds > windowSeconds)
      notes.push(
        `left out the default ${p.name} alert: its ${seconds(p.longWindowSeconds)} window is longer than the ${seconds(windowSeconds)} SLO window`,
      );
    else if (
      stepSeconds > 0 &&
      (p.shortWindowSeconds % stepSeconds !== 0 ||
        p.longWindowSeconds % stepSeconds !== 0)
    )
      notes.push(
        `left out the default ${p.name} alert: its windows are not multiples of the ${seconds(stepSeconds)} step`,
      );
    else if (target < 1 && p.burnRate > 1 / (1 - target))
      notes.push(
        `left out the default ${p.name} alert: a ${g(p.burnRate)}x burn rate is unreachable with a ${g(target)} target`,
      );
    else policies.push({ ...p });
  }
  return { policies, notes };
}

// --- helpers mirroring Go's omitempty ----------------------------------------

/** Drop keys whose values Go's omitempty would leave out. */
function omitEmpty(o: Obj, keys: string[]): Obj {
  const out: Obj = {};
  for (const [k, v] of Object.entries(o)) {
    if (v === undefined) continue;
    if (keys.includes(k)) {
      if (v === "" || v === 0 || v === false || v === null) continue;
      if (Array.isArray(v) && v.length === 0) continue;
      if (isObj(v) && Object.keys(v).length === 0) continue;
    }
    out[k] = v;
  }
  return out;
}
const str = (v: unknown) => (typeof v === "string" ? v : "");
const num = (v: unknown) => (typeof v === "number" ? v : 0);

// --- stored -> v1beta1 -------------------------------------------------------

/** Mirror of Go FromHub: a stored SLO as v1beta1 for display and export. */
export function sloToBeta(hub: SLODefinition): Obj {
  const s = hub.spec as unknown as Obj;
  const spec: Obj = {
    displayName: s.displayName,
    service: s.service,
    target: s.target,
    window: seconds(num(s.windowSeconds)),
    step: seconds(num(s.stepSeconds)),
    connection: s.source,
    labels: s.labels,
    lifecycle: s.lifecycle,
  };
  const m = s.measurement as Obj | undefined;
  if (m) {
    spec.measurement = omitEmpty(
      {
        type: m.type,
        metric: m.metric,
        eventMetric: m.eventMetric,
        labels: m.labels,
        successLabels: m.successLabels,
        countFrom: m.countFrom,
        threshold:
          typeof m.thresholdSeconds === "number"
            ? formatDuration(m.thresholdSeconds * 1000)
            : undefined,
        scrapeInterval:
          num(m.scrapeIntervalSeconds) !== DEFAULT_SCRAPE_SECONDS
            ? seconds(num(m.scrapeIntervalSeconds))
            : undefined,
        observationPoint: str(m.observationPoint),
      },
      [
        "eventMetric",
        "labels",
        "successLabels",
        "countFrom",
        "threshold",
        "scrapeInterval",
      ],
    );
  } else if (s.measurementBinding) {
    spec.binding = s.measurementBinding;
  } else if (s.goodQuery || s.totalQuery || s.providerWindow) {
    const w = s.providerWindow as Obj | undefined;
    spec.queries = omitEmpty(
      {
        good: s.goodQuery,
        total: s.totalQuery,
        language: s.queryLanguage,
        window: w
          ? omitEmpty(
              {
                good: w.goodSelector,
                total: str(w.totalSelector),
                bad: w.badSelector,
                scrapeInterval:
                  num(w.scrapeIntervalSeconds) !== DEFAULT_SCRAPE_SECONDS
                    ? seconds(num(w.scrapeIntervalSeconds))
                    : undefined,
              },
              ["good", "bad", "scrapeInterval"],
            )
          : undefined,
      },
      ["good", "total", "language", "window"],
    );
  }
  const b = s.badInterval as Obj | undefined;
  if (b)
    spec.intervals = omitEmpty(
      {
        errorRate: b.errorRate,
        minEvents: b.minEvents,
        minGoodEvents: b.minGoodEvents,
      },
      ["errorRate", "minEvents", "minGoodEvents"],
    );
  const q = s.quality as Obj | undefined;
  if (q)
    spec.quality = omitEmpty(
      {
        minEvents: q.minEvents,
        maxSampleAge:
          num(q.maxSampleAgeSeconds) > 0
            ? seconds(num(q.maxSampleAgeSeconds))
            : undefined,
        cohortLabels: q.cohortLabels,
      },
      ["minEvents", "maxSampleAge", "cohortLabels"],
    );
  let hubPolicies = (s.policies as HubPolicy[] | undefined) ?? [];
  if (hubPolicies.length === 0 && s.policy)
    hubPolicies = [s.policy as HubPolicy];
  spec.policies = hubPolicies.map((p) =>
    omitEmpty(
      {
        name: p.name,
        severity: p.severity,
        shortWindow: seconds(p.shortWindowSeconds),
        longWindow: seconds(p.longWindowSeconds),
        burnRate: p.burnRate,
        for: p.forSeconds > 0 ? seconds(p.forSeconds) : undefined,
        minCoverage: p.minCoverage !== 1 ? p.minCoverage : undefined,
        minEvents: p.minEvents,
        confidence: p.confidence,
      },
      ["name", "severity", "for", "minEvents", "confidence"],
    ),
  );
  spec.errorBudgetPolicy = s.errorBudgetPolicy;
  spec.customers = s.customers;
  return {
    apiVersion: V1BETA1,
    kind: "ServiceLevelObjective",
    metadata: hub.metadata,
    spec: omitEmpty(spec, ["connection", "labels", "lifecycle"]),
  };
}

// --- v1beta1 -> stored -------------------------------------------------------

/** Mirror of Go ToHub: a v1beta1 SLO in the stored form, with notes. */
export function sloFromBeta(beta: unknown): {
  definition: SLODefinition;
  notes: string[];
} {
  if (
    !isObj(beta) ||
    beta.apiVersion !== V1BETA1 ||
    beta.kind !== "ServiceLevelObjective" ||
    !isObj(beta.spec)
  )
    throw Error(
      `expected apiVersion ${V1BETA1} and kind ServiceLevelObjective`,
    );
  const s = beta.spec;
  const notes: string[] = [];
  if (s.window === undefined)
    notes.push(`window defaults to ${seconds(DEFAULT_WINDOW_SECONDS)}`);
  const windowSeconds = optionalSeconds(
    s.window,
    DEFAULT_WINDOW_SECONDS,
    "window",
  );
  const stepSeconds = optionalSeconds(s.step, DEFAULT_STEP_SECONDS, "step");
  const out: Obj = {
    displayName: s.displayName,
    service: s.service,
    target: s.target,
    windowSeconds,
    stepSeconds,
    source: s.connection,
    labels: s.labels,
    lifecycle: s.lifecycle,
    errorBudgetPolicy: s.errorBudgetPolicy,
    customers: s.customers,
    measurementBinding: s.binding,
  };
  let sources = 0;
  if (isObj(s.measurement)) {
    sources++;
    const m = s.measurement;
    out.measurement = omitEmpty(
      {
        type: m.type,
        metric: m.metric,
        eventMetric: m.eventMetric,
        labels: isObj(m.labels) ? m.labels : {},
        successLabels: m.successLabels,
        countFrom: m.countFrom,
        thresholdSeconds:
          m.threshold !== undefined
            ? parseDuration(m.threshold, "measurement.threshold") / 1000
            : undefined,
        scrapeIntervalSeconds: optionalSeconds(
          m.scrapeInterval,
          DEFAULT_SCRAPE_SECONDS,
          "measurement.scrapeInterval",
        ),
        observationPoint: str(m.observationPoint),
      },
      ["eventMetric", "successLabels", "countFrom"],
    );
  }
  if (isObj(s.queries)) {
    sources++;
    const q = s.queries;
    if (Boolean(q.good || q.total) === isObj(q.window))
      throw Error(
        "queries need either good and total, or window selectors, not both",
      );
    out.goodQuery = q.good;
    out.totalQuery = q.total;
    out.queryLanguage = q.language;
    if (isObj(q.window)) {
      const w = q.window;
      out.providerWindow = omitEmpty(
        {
          goodSelector: str(w.good),
          totalSelector: str(w.total),
          badSelector: w.bad,
          scrapeIntervalSeconds: optionalSeconds(
            w.scrapeInterval,
            DEFAULT_SCRAPE_SECONDS,
            "queries.window.scrapeInterval",
          ),
        },
        ["badSelector"],
      );
    }
  }
  if (s.binding !== undefined) sources++;
  if (sources > 1)
    throw Error(
      `an SLO is measured by at most one of measurement, queries or binding; found ${sources}`,
    );
  if (sources === 0)
    notes.push(
      "no measurement, queries or binding: the SLO is measured from samples pushed to the API",
    );
  if (isObj(s.intervals)) {
    const iv = s.intervals;
    if (
      num(iv.minGoodEvents) > 0 &&
      (num(iv.errorRate) > 0 || num(iv.minEvents) > 0)
    )
      throw Error(
        "intervals take errorRate (good-minute SLO) or minGoodEvents (throughput SLO), not both",
      );
    out.badInterval = omitEmpty(
      {
        errorRate: iv.errorRate,
        minEvents: iv.minEvents,
        minGoodEvents: iv.minGoodEvents,
      },
      ["errorRate", "minEvents", "minGoodEvents"],
    );
  }
  if (isObj(s.quality)) {
    const q = s.quality;
    out.quality = omitEmpty(
      {
        minEvents: q.minEvents,
        maxSampleAgeSeconds:
          q.maxSampleAge !== undefined
            ? wholeSeconds(q.maxSampleAge, "quality.maxSampleAge")
            : undefined,
        cohortLabels: q.cohortLabels,
      },
      ["minEvents", "maxSampleAgeSeconds", "cohortLabels"],
    );
  }
  if (s.policies === undefined) {
    const { policies, notes: left } = defaultPolicies(
      num(s.target),
      windowSeconds,
      stepSeconds,
    );
    out.policies = policies;
    if (policies.length > 0)
      notes.push(
        "policies default to the standard burn-rate alerts: " +
          policies.map((p) => p.name).join(", "),
      );
    notes.push(...left);
  } else {
    if (!Array.isArray(s.policies)) throw Error("policies must be a list");
    out.policies = s.policies.map((p, i) => {
      if (!isObj(p)) throw Error(`policies[${i}] must be an object`);
      return omitEmpty(
        {
          name: p.name,
          severity: p.severity,
          shortWindowSeconds: wholeSeconds(
            p.shortWindow,
            `policies[${i}].shortWindow`,
          ),
          longWindowSeconds: wholeSeconds(
            p.longWindow,
            `policies[${i}].longWindow`,
          ),
          burnRate: p.burnRate,
          forSeconds:
            p.for !== undefined ? wholeSeconds(p.for, `policies[${i}].for`) : 0,
          minCoverage: typeof p.minCoverage === "number" ? p.minCoverage : 1,
          minEvents: p.minEvents,
          confidence: p.confidence,
        },
        ["name", "severity", "minEvents", "confidence"],
      );
    });
    const list = out.policies as Obj[];
    // A lone unnamed policy is the legacy single policy, so it round-trips.
    if (list.length === 1 && !list[0].name) {
      out.policy = list[0];
      delete out.policies;
    } else if (list.length === 0)
      notes.push("alerting is off: policies is an empty list");
  }
  const spec = omitEmpty(out, [
    "source",
    "labels",
    "lifecycle",
    "errorBudgetPolicy",
    "customers",
    "measurementBinding",
    "goodQuery",
    "totalQuery",
    "queryLanguage",
    "policies",
  ]);
  return {
    definition: {
      apiVersion: V1ALPHA1,
      kind: "ServiceLevelObjective",
      metadata: beta.metadata as SLODefinition["metadata"],
      spec: spec as unknown as SLODefinition["spec"],
    },
    notes,
  };
}
