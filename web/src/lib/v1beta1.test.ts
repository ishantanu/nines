import { describe, expect, it } from "vitest";
import fixtures from "./v1beta1-fixtures.json";
import type { SLODefinition } from "./definitions";
import {
  bindingFromBeta,
  bindingToBeta,
  dependencyMapFromBeta,
  dependencyMapToBeta,
  formatDuration,
  journeyFromBeta,
  journeyToBeta,
  parseDuration,
  sloFromBeta,
  sloToBeta,
} from "./v1beta1";

// Fixtures come from the Go conversion (api/v1beta1/fixtures_test.go). Every
// case must convert identically here, so the portal and server agree.
describe("v1beta1 conversion matches the Go implementation", () => {
  it.each(fixtures.export.map((c) => [c.name, c] as const))(
    "exports %s",
    (_name, c) => {
      expect(sloToBeta(c.hub as unknown as SLODefinition)).toEqual(c.beta);
    },
  );

  it.each(fixtures.import.map((c) => [c.name, c] as const))(
    "imports %s",
    (_name, c) => {
      if ("error" in c && c.error) {
        expect(() => sloFromBeta(c.beta)).toThrow(c.error);
        return;
      }
      const { definition, notes } = sloFromBeta(c.beta);
      expect(definition).toEqual(c.hub);
      expect(notes).toEqual(c.notes);
    },
  );
});

describe("durations", () => {
  it("parses and formats the units Go accepts", () => {
    expect(parseDuration("28d", "w")).toBe(28 * 86_400_000);
    expect(parseDuration("1h30m", "w")).toBe(5_400_000);
    expect(parseDuration("300ms", "w")).toBe(300);
    expect(formatDuration(25 * 3_600_000 + 1000)).toBe("1d1h1s");
    for (const bad of ["", "0s", "5", "1.5h", "1w", "401d", 5])
      expect(() => parseDuration(bad, "w")).toThrow();
  });
});

const lookupFrom =
  (slos: Record<string, string | undefined>) =>
  (id: string): string | undefined =>
    id in slos ? slos[id] : undefined;

describe("journeys, bindings and dependency maps match the Go implementation", () => {
  it.each(fixtures.journeyExport.map((c) => [c.name, c] as const))(
    "exports journey %s",
    (_name, c) => {
      expect(journeyToBeta(c.hub as never, lookupFrom(c.slos))).toEqual(c.beta);
    },
  );
  it.each(fixtures.journeyImport.map((c) => [c.name, c] as const))(
    "imports journey %s",
    (_name, c) => {
      if ("error" in c && c.error) {
        expect(() => journeyFromBeta(c.beta, lookupFrom(c.slos))).toThrow(
          c.error,
        );
        return;
      }
      const { journey, notes } = journeyFromBeta(c.beta, lookupFrom(c.slos));
      expect(journey).toEqual(c.hub);
      expect(notes).toEqual(c.notes);
    },
  );
  it.each(fixtures.bindings.map((c) => [c.name, c] as const))(
    "round-trips binding %s",
    (_name, c) => {
      expect(bindingToBeta(c.hub as never)).toEqual(c.beta);
      expect(bindingFromBeta(c.beta)).toEqual(c.hub);
    },
  );
  it.each(fixtures.dependencyMaps.map((c) => [c.name, c] as const))(
    "round-trips dependency map %s",
    (_name, c) => {
      expect(dependencyMapToBeta(c.hub as never)).toEqual(c.beta);
      expect(dependencyMapFromBeta(c.beta)).toEqual(c.hub);
    },
  );
});
