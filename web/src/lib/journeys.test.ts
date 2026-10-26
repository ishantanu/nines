import { expect, test } from "vitest";
import { stringify } from "yaml";
import {
  newJourney,
  parseJourney,
  removeJourneyLink,
  removeJourneyStep,
  renameJourneyStep,
} from "./journeys";
function journey() {
  const m = newJourney();
  m.metadata.name = "checkout";
  m.spec.slos = [
    { id: "availability", dimension: "availability", required: true },
    { id: "latency", dimension: "latency", required: false },
  ];
  m.spec.steps = [
    { id: "request", name: "Start", actor: "user", sloRefs: ["availability"] },
    {
      id: "confirm",
      name: "Finish",
      actor: "service",
      sloRefs: ["latency"],
      dependsOn: ["request"],
    },
  ];
  return m;
}
test("form/YAML round trip preserves step graph, optional zero-cost limit and agent contract", () => {
  const m = journey();
  m.spec.agentic = {
    successDefinition: "Confirmed with evidence",
    maxDurationSeconds: 120,
    maxCostUSD: 0,
    maxSteps: 8,
    approvalRequired: true,
  };
  expect(parseJourney(stringify(m))).toEqual(m);
});
test("renaming and removing a step keep dependency references consistent", () => {
  const m = journey();
  const renamed = renameJourneyStep(m, 0, "begin");
  expect(renamed.spec.steps[1].dependsOn).toEqual(["begin"]);
  expect(removeJourneyStep(renamed, 0).spec.steps[0].dependsOn).toEqual([]);
  expect(m.spec.steps[0].id).toBe("request");
});
test("unlinking an SLO removes its step references without changing other links", () => {
  const next = removeJourneyLink(journey(), "availability");
  expect(next.spec.slos.map((l) => l.id)).toEqual(["latency"]);
  expect(next.spec.steps.map((s) => s.sloRefs)).toEqual([[], ["latency"]]);
});
test("malformed YAML cannot switch into the form or discard its fields", () => {
  const m = journey();
  for (const text of [
    "null",
    stringify({ ...m, kind: "ServiceLevelObjective" }),
    stringify({
      ...m,
      spec: { ...m.spec, steps: [{ id: "bad", sloRefs: "availability" }] },
    }),
    stringify({
      ...m,
      spec: {
        ...m.spec,
        agentic: {
          successDefinition: "ok",
          approvalRequired: false,
          maxSteps: "three",
        },
      },
    }),
  ])
    expect(() => parseJourney(text)).toThrow();
});
