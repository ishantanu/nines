/** The only fields the classification reads. */
export interface ClassifiableSLO {
  measurement?: { type: "availability" | "latency" | "sessions" };
  badInterval?: unknown;
  customers?: { label?: string; customers?: unknown[] };
}

// One catalog of the kinds of SLO Nines can express, so the portal and the
// public site describe them the same way. The classification is derived from
// the saved definition only; it never guesses from metric names.
export type SLOTypeId =
  | "availability"
  | "latency"
  | "sessions"
  | "good-minute"
  | "customers"
  | "journey"
  | "custom";

export interface SLOType {
  id: SLOTypeId;
  name: string;
  /** What the SLI counts. */
  measures: string;
  /** When to choose it. */
  useWhen: string;
  /** Where it is defined: every kind has a guided form; YAML is the equivalent. */
  definedIn: "Guided form" | "Journey form";
  /** Documentation anchor under /docs/. */
  docs: string;
  snippet?: string;
}

export const DOCS_BASE = "https://ishantanu.github.io/nines/docs/";

export const SLO_TYPES: SLOType[] = [
  {
    id: "availability",
    name: "Availability",
    measures: "The share of eligible events that succeeded.",
    useWhen: "Requests, jobs or calls where each event succeeds or fails.",
    definedIn: "Guided form",
    docs: "measurements/#create-an-slo-in-the-portal",
  },
  {
    id: "latency",
    name: "Latency",
    measures: "The share of events that finished under a duration threshold.",
    useWhen: "You have a histogram and care how fast, not only whether.",
    definedIn: "Guided form",
    docs: "latency-slos/",
  },
  {
    id: "sessions",
    name: "Sessions",
    measures: "The share of sessions that did not hit a bad event.",
    useWhen:
      "Client-side signals such as crashes, where raw events have no stable denominator.",
    definedIn: "Guided form",
    docs: "measurements/#measure-sessions-instead-of-requests",
    snippet: `spec:
  measurement:
    type: sessions
    metric: app_sessions_total
    eventMetric: app_sessions_crashed_total
    labels:
      app: ios
    scrapeIntervalSeconds: 15
    observationPoint: One increment per app session; the event counter increments once per crashed session.
  target: 0.995`,
  },
  {
    id: "good-minute",
    name: "Good-minute",
    measures: "The share of judged intervals whose error ratio stayed low.",
    useWhen:
      "A steady noise floor makes an event ratio too loose to catch a real incident.",
    definedIn: "Guided form",
    docs: "measurements/#judge-intervals-instead-of-events",
    snippet: `spec:
  stepSeconds: 60
  target: 0.999
  windowSeconds: 2592000
  goodQuery: sum(increase(client_requests_ok_total[1m]))
  totalQuery: sum(increase(client_requests_total[1m]))
  badInterval:
    errorRate: 0.06
    minEvents: 20`,
  },
  {
    id: "customers",
    name: "Per-customer",
    measures:
      "The same SLI reported for a short list of customers next to the service-wide figure.",
    useWhen:
      "One large customer could fail while the average stays inside budget.",
    definedIn: "Guided form",
    docs: "measurement-quality/#report-an-slo-for-key-customers",
    snippet: `spec:
  customers:
    label: account
    customers:
      - value: acme-01
        name: Acme
      - value: globex-7
        target: 0.995`,
  },
  {
    id: "journey",
    name: "Journey run objective",
    measures: "The share of an agent journey's judged runs that passed.",
    useWhen: "An agentic journey records runs against a success contract.",
    definedIn: "Journey form",
    docs: "critical-user-journeys/#measure-an-agentic-journey-from-its-runs",
    snippet: `spec:
  runObjective:
    target: 0.9
    windowSeconds: 604800
    minRuns: 20`,
  },
  {
    id: "custom",
    name: "Custom queries",
    measures: "Counts from your own good and total queries or imported points.",
    useWhen: "The guided measurement cannot express the SLI.",
    definedIn: "Guided form",
    docs: "slo-guide/",
  },
];

const byId = new Map(SLO_TYPES.map((t) => [t.id, t]));
export const sloType = (id: SLOTypeId) => byId.get(id)!;

export interface SLOClassification {
  primary: Exclude<SLOTypeId, "customers" | "journey">;
  addOns: SLOTypeId[];
}

/** Derived from the saved definition: what it counts, plus opt-in reporting. */
export function classifySLO(slo: ClassifiableSLO): SLOClassification {
  const primary = slo.badInterval
    ? "good-minute"
    : (slo.measurement?.type ?? "custom");
  const addOns: SLOTypeId[] = [];
  if (slo.customers?.customers?.length) addOns.push("customers");
  return { primary, addOns };
}

export function countSLOTypes(slos: ClassifiableSLO[]) {
  const counts = new Map<SLOTypeId, number>();
  for (const slo of slos) {
    const { primary } = classifySLO(slo);
    counts.set(primary, (counts.get(primary) ?? 0) + 1);
  }
  return counts;
}
