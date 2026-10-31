import { expect, test } from "vitest";
import { parse } from "yaml";
import { normalizeBurnWindows, stringifyDefinition } from "./definition-yaml";
const policy = {
  shortWindowSeconds: 300,
  longWindowSeconds: 3600,
  burnRate: 14.4,
  forSeconds: 120,
  minCoverage: 1,
};
test("YAML exports readable windows without changing the runtime object", () => {
  const resource = { spec: { policies: [policy], intent: { policy } } };
  const yaml = stringifyDefinition(resource);
  expect(parse(yaml).spec.policies[0]).toEqual({
    shortWindow: "5m",
    longWindow: "1h",
    burnRate: 14.4,
    forSeconds: 120,
    minCoverage: 1,
  });
  expect(normalizeBurnWindows(parse(yaml))).toEqual(resource);
  expect(policy.shortWindowSeconds).toBe(300);
});
test("forms normalize equivalent durations and keep legacy seconds", () => {
  expect(
    normalizeBurnWindows({ ...policy, shortWindowSeconds: undefined }),
  ).toHaveProperty("longWindowSeconds", 3600);
  expect(
    normalizeBurnWindows({
      shortWindow: "300s",
      longWindow: "1h30m",
      burnRate: 6,
    }),
  ).toEqual({ shortWindowSeconds: 300, longWindowSeconds: 5400, burnRate: 6 });
});
test.each([
  "0s",
  "-5m",
  "1ms",
  "0.5m",
  "1d",
  "300",
  "999999999999999h",
  null,
  300,
])("rejects invalid duration %s", (value) => {
  expect(() =>
    normalizeBurnWindows({ shortWindow: value, burnRate: 6 }),
  ).toThrow();
});
test("refuses conflicting representations in both directions", () => {
  expect(() => normalizeBurnWindows({ ...policy, shortWindow: "5m" })).toThrow(
    "not both",
  );
  expect(() => stringifyDefinition({ ...policy, longWindow: "1h" })).toThrow(
    "not both",
  );
});
