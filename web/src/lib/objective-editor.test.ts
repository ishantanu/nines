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

test("a policy confidence survives a round trip and a bad value is rejected", () => {
  const d = definition();
  d.spec.policy = {
    shortWindowSeconds: 300,
    longWindowSeconds: 3600,
    burnRate: 14.4,
    forSeconds: 120,
    minCoverage: 1,
    confidence: 0.99,
  };
  expect(parseObjectiveDefinition(stringify(d)).spec.policy?.confidence).toBe(
    0.99,
  );
  const bad = definition();
  bad.spec.policy = { ...d.spec.policy, confidence: "high" as never };
  expect(() => parseObjectiveDefinition(stringify(bad))).toThrow();
});

test("labels survive a round trip and non-string values are rejected", () => {
  const d = definition();
  d.spec.labels = { team: "payments", product: "Checkout" };
  expect(parseObjectiveDefinition(stringify(d)).spec.labels).toEqual({
    team: "payments",
    product: "Checkout",
  });
  const bad = definition();
  bad.spec.labels = { team: 3 as never };
  expect(() => parseObjectiveDefinition(stringify(bad))).toThrow();
});

test("editing an existing SLO keeps its labels", async () => {
  const { definitionFromSLO } = await import("./definitions");
  const d = definitionFromSLO({
    id: "api",
    project: "p",
    service: "s",
    name: "API",
    target: 0.99,
    windowSeconds: 3600,
    stepSeconds: 60,
    labels: { team: "payments" },
  });
  expect(d.spec.labels).toEqual({ team: "payments" });
  const empty = definitionFromSLO({
    id: "api",
    project: "p",
    service: "s",
    name: "API",
    target: 0.99,
    windowSeconds: 3600,
    stepSeconds: 60,
    labels: {},
  });
  expect("labels" in empty.spec).toBe(false);
});

test("a bad-interval definition survives editing and invalid ones are rejected", async () => {
  const { definitionFromSLO } = await import("./definitions");
  const d = definitionFromSLO({
    id: "api",
    project: "p",
    service: "s",
    name: "API",
    target: 0.999,
    windowSeconds: 3600,
    stepSeconds: 60,
    goodQuery: "g",
    totalQuery: "t",
    badInterval: { errorRate: 0.06, minEvents: 20 },
  });
  expect(d.spec.badInterval).toEqual({ errorRate: 0.06, minEvents: 20 });
  expect(parseObjectiveDefinition(stringify(d)).spec.badInterval).toEqual({
    errorRate: 0.06,
    minEvents: 20,
  });
  for (const badInterval of [
    { errorRate: 0 },
    { errorRate: 1 },
    { errorRate: "x" },
  ]) {
    const bad = { ...d, spec: { ...d.spec, badInterval } };
    expect(() => parseObjectiveDefinition(stringify(bad))).toThrow();
  }
});

test("a customers policy survives editing and a malformed one is rejected", async () => {
  const { definitionFromSLO } = await import("./definitions");
  const customers = {
    label: "account",
    customers: [{ value: "acme-01", name: "Acme", target: 0.995 }],
  };
  const d = definitionFromSLO({
    id: "api",
    project: "p",
    service: "s",
    name: "API",
    target: 0.999,
    windowSeconds: 3600,
    stepSeconds: 60,
    customers,
  });
  expect(d.spec.customers).toEqual(customers);
  expect(parseObjectiveDefinition(stringify(d)).spec.customers).toEqual(
    customers,
  );
  for (const bad of [
    { label: "a", customers: [] },
    { label: 1, customers: [{ value: "x" }] },
    { label: "a", customers: [{ value: 3 }] },
  ]) {
    expect(() =>
      parseObjectiveDefinition(
        stringify({ ...d, spec: { ...d.spec, customers: bad } }),
      ),
    ).toThrow();
  }
});

test("a session measurement survives editing and round-trips through the guided form", async () => {
  const { definitionFromSLO } = await import("./definitions");
  const measurement = {
    type: "sessions" as const,
    metric: "app_sessions_total",
    eventMetric: "app_sessions_crashed_total",
    labels: { app: "ios" },
    scrapeIntervalSeconds: 15,
    observationPoint: "One increment per app session.",
  };
  const d = definitionFromSLO({
    id: "ios",
    project: "p",
    service: "ios",
    name: "iOS crash-free sessions",
    target: 0.995,
    windowSeconds: 3600,
    stepSeconds: 60,
    measurement,
  });
  expect(d.spec.measurement).toEqual(measurement);
  expect(parseObjectiveDefinition(stringify(d)).spec.measurement).toEqual(
    measurement,
  );
  // The guided form keeps the event counter and rebuilds the same measurement.
  const draft = inlineObjectiveDraft(d);
  expect(draft.measurement.measurement).toBe("sessions");
  expect(draft.measurement.eventMetric).toBe("app_sessions_crashed_total");
  expect(inlineObjectiveDefinition(draft).spec.measurement).toEqual(
    measurement,
  );
  // Both counters are required and must differ.
  expect(() =>
    inlineObjectiveDefinition({
      ...draft,
      measurement: { ...draft.measurement, eventMetric: "" },
    }),
  ).toThrow(/bad event/);
  expect(() =>
    inlineObjectiveDefinition({
      ...draft,
      measurement: { ...draft.measurement, eventMetric: "app_sessions_total" },
    }),
  ).toThrow(/differ/);
  // Labels and per-customer reporting are written through the guided form and validated.
  const withExtras = inlineObjectiveDefinition({
    ...inlineObjectiveDraft({
      ...d,
      spec: {
        ...d.spec,
        measurement: {
          ...measurement,
          type: "availability",
          successLabels: { status: "200" },
          eventMetric: undefined,
        },
      },
    }),
    labels: { team: "mobile" },
    customers: {
      label: "account",
      customers: [{ value: "acme", name: "Acme", target: 0.995 }],
    },
  });
  expect(withExtras.spec.labels).toEqual({ team: "mobile" });
  expect(withExtras.spec.customers?.customers[0].target).toBe(0.995);
  expect(() =>
    inlineObjectiveDefinition({ ...draft, labels: { "Bad Key": "x" } }),
  ).toThrow(/Label names/);
  expect(() =>
    inlineObjectiveDefinition({
      ...draft,
      customers: {
        label: "account",
        customers: [{ value: "a" }, { value: "a" }],
      },
    }),
  ).toThrow(/repeated/);
  const bad = {
    ...d,
    spec: { ...d.spec, measurement: { ...measurement, eventMetric: 5 } },
  };
  expect(() => parseObjectiveDefinition(stringify(bad))).toThrow();
});

test("the book's SLI kinds round-trip between YAML and the guided form", () => {
  for (const kind of ["correctness", "durability", "coverage"] as const) {
    const d = definition();
    d.spec.measurement!.type = kind;
    d.spec.measurement!.successLabels = { verdict: "pass" };
    const parsed = parseObjectiveDefinition(stringify(d));
    expect(parsed.spec.measurement!.type).toBe(kind);
    const result = inlineObjectiveDefinition(inlineObjectiveDraft(parsed));
    expect(result.spec.measurement).toMatchObject({
      type: kind,
      successLabels: { verdict: "pass" },
    });
  }
  const d = definition();
  d.spec.measurement = {
    type: "freshness",
    metric: "record_age_seconds",
    labels: { pipeline: "index" },
    thresholdSeconds: 300,
    scrapeIntervalSeconds: 15,
    observationPoint: "Age of each record when served.",
  };
  const parsed = parseObjectiveDefinition(stringify(d));
  const result = inlineObjectiveDefinition(inlineObjectiveDraft(parsed));
  expect(result.spec.measurement).toMatchObject({
    type: "freshness",
    thresholdSeconds: 300,
  });
  expect(result.spec.measurement!.successLabels).toBeUndefined();
});
