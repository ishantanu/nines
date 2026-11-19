import { expect, test } from "vitest";
import { objectiveDefinition, objectiveDraft } from "./objective-create";
import { newObjectiveDefinition } from "@/lib/objective-editor";
import { badIntervalFromDraft, intervalDraft } from "@/lib/slo-extras";

const queries = () => {
  const base = newObjectiveDefinition();
  base.spec.measurement = undefined;
  base.spec.goodQuery = "sum(increase(ok[1m]))";
  base.spec.totalQuery = "sum(increase(all[1m]))";
  return base;
};

test("good-minute judging is written from the Settings form for query SLOs only", () => {
  const base = queries();
  const draft = objectiveDraft(base);
  expect(draft.interval.enabled).toBe(false);
  expect(objectiveDefinition(base, draft).spec.badInterval).toBeUndefined();
  const on = {
    ...draft,
    interval: {
      enabled: true,
      mode: "errorRate" as const,
      errorRate: "0.06",
      minEvents: "20",
      minGoodEvents: "",
    },
  };
  expect(objectiveDefinition(base, on).spec.badInterval).toEqual({
    errorRate: 0.06,
    minEvents: 20,
  });
  // Provider-window counters are evaluated by the provider, which cannot judge intervals.
  expect(
    objectiveDefinition(base, { ...on, providerWindow: true }).spec.badInterval,
  ).toBeUndefined();
  // An existing definition keeps its policy when reopened.
  const saved = objectiveDefinition(base, on);
  expect(objectiveDraft(saved).interval).toEqual({
    enabled: true,
    mode: "errorRate",
    errorRate: "0.06",
    minEvents: "20",
    minGoodEvents: "",
  });
});

test("throughput judging writes only minGoodEvents and reopens as throughput", () => {
  const base = queries();
  const draft = objectiveDraft(base);
  const on = {
    ...draft,
    interval: {
      enabled: true,
      mode: "throughput" as const,
      errorRate: "0.05",
      minEvents: "20",
      minGoodEvents: "500",
    },
  };
  // The good-minute fields stay in the form but never reach the definition.
  const saved = objectiveDefinition(base, on);
  expect(saved.spec.badInterval).toEqual({ minGoodEvents: 500 });
  expect(objectiveDraft(saved).interval).toMatchObject({
    enabled: true,
    mode: "throughput",
    minGoodEvents: "500",
  });
  for (const minGoodEvents of ["", "0", "-5", "abc"])
    expect(() =>
      badIntervalFromDraft({ ...on.interval, minGoodEvents }),
    ).toThrow(/fewest good events/);
});

test("interval settings reject values the server would refuse", () => {
  for (const errorRate of ["0", "1", "-0.1", "abc", ""])
    expect(() =>
      badIntervalFromDraft({
        enabled: true,
        mode: "errorRate",
        errorRate,
        minEvents: "",
        minGoodEvents: "",
      }),
    ).toThrow(/error rate/);
  expect(() =>
    badIntervalFromDraft({
      enabled: true,
      mode: "errorRate",
      errorRate: "0.05",
      minEvents: "1.5",
      minGoodEvents: "",
    }),
  ).toThrow(/whole number/);
  expect(badIntervalFromDraft(intervalDraft(undefined))).toBeUndefined();
});

test("labels are validated when the definition is built", () => {
  const base = queries();
  expect(() =>
    objectiveDefinition(base, {
      ...objectiveDraft(base),
      labels: { Team: "x" },
    }),
  ).toThrow(/Label names/);
  expect(
    objectiveDefinition(base, {
      ...objectiveDraft(base),
      labels: { team: "x" },
    }).spec.labels,
  ).toEqual({ team: "x" });
});
