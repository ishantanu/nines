import { describe, expect, it } from "vitest";
import { backtestTime, intentGoal, intentPopulation } from "./intent";
import { definitionFromSLO } from "./definitions";
describe("intent authoring", () => {
  it("preserves explicit population values and rejects ambiguous YAML", () => {
    expect(
      intentPopulation('environment: "prod"\nservice: "checkout"'),
    ).toEqual({ environment: "prod", service: "checkout" });
    for (const text of [
      "",
      "- prod",
      "count: 200",
      "env: a\nenv: b",
      "env: prod\n---\nenv: test",
    ])
      expect(() => intentPopulation(text)).toThrow();
  });
  it("uses exact target fractions and custom human windows", () => {
    expect(intentGoal("99.9", "28d")).toEqual({
      target: 0.999,
      windowSeconds: 2419200,
    });
    for (const value of ["100", "0", "NaN", ""])
      expect(() => intentGoal(value, "28d")).toThrow();
  });
  it("preserves intent provenance in existing SLO exports", () => {
    const intent = {
      description: "API success",
      service: "api",
      source: "primary",
      outcome: "availability",
      population: { service: "api" },
      target: 0.999,
      windowSeconds: 3600,
      noDataPolicy: "unknown" as const,
    };
    const result = definitionFromSLO({
      id: "api",
      name: "API",
      project: "test",
      service: "api",
      target: 0.999,
      windowSeconds: 3600,
      stepSeconds: 60,
      intent,
    });
    expect(result.spec.intent).toEqual(intent);
  });
});

describe("historical cutoff time input", () => {
  it("requires explicit timezone and whole aligned seconds without normalizing impossible dates", () => {
    expect(backtestTime("2026-10-20T06:00:00+01:00")).toBe(
      "2026-10-20T05:00:00.000Z",
    );
    for (const value of [
      "2026-10-20T06:00:00",
      "2026-02-30T06:00:00Z",
      "2026-10-20T06:00:00.5Z",
      "2026-10-20T24:00:00Z",
    ])
      expect(() => backtestTime(value)).toThrow();
  });
});
