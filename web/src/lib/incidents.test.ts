import { expect, test } from "vitest";
import {
  badEventsText,
  budgetText,
  detectionLag,
  nextStates,
  worstImpact,
  type Incident,
  type IncidentImpact,
} from "./incidents";

const impact = (over: Partial<IncidentImpact>): IncidentImpact => ({
  sloId: "checkout",
  from: "2026-11-22T13:39:00Z",
  to: "2026-11-22T13:45:00Z",
  badEvents: 200,
  budgetConsumedFraction: 0.25,
  coverage: 1,
  ...over,
});

test("unknown impact is never shown as zero", () => {
  const unknown = impact({ badEvents: null, budgetConsumedFraction: null });
  expect(badEventsText(unknown)).toBe("Unknown");
  expect(budgetText(unknown)).toBe("Unknown");
  expect(
    badEventsText(
      impact({ badEventsAtLeast: true, budgetConsumedFraction: null }),
    ),
  ).toBe("At least 200");
  expect(budgetText(impact({}))).toBe("25.0% of the error budget");
  expect(budgetText(impact({ budgetConsumedFraction: 0.0002 }))).toBe(
    "<0.1% of the error budget",
  );
});

test("the list shows the worst known impact, or says it is unknown", () => {
  const incident = {
    impact: [
      impact({ sloId: "a", budgetConsumedFraction: 0.1 }),
      impact({ sloId: "b", budgetConsumedFraction: 0.4 }),
    ],
  } as Incident;
  expect(worstImpact(incident)).toBe("40.0% of the error budget on b");
  expect(
    worstImpact({
      impact: [impact({ budgetConsumedFraction: null })],
    } as Incident),
  ).toBe("Impact unknown");
  expect(worstImpact({ impact: [] } as unknown as Incident)).toBe(
    "No SLOs linked",
  );
});

test("detection lag compares the alert with when impact started", () => {
  const slo = {
    id: "checkout",
    configRevision: 1,
    firing: false,
    firingSince: "2026-11-22T13:41:00Z",
  };
  expect(
    detectionLag(slo, impact({ impactStartedAt: "2026-11-22T13:39:20Z" })),
  ).toBe("1 min 40 s");
  expect(detectionLag(slo, impact({}))).toBeNull();
});

test("resolved incidents offer no further transitions", () => {
  expect(nextStates.resolved).toEqual([]);
  expect(nextStates.detected).toContain("mitigated");
});
