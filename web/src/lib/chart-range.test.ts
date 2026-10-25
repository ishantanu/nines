import { expect, test } from "vitest";
import {
  absoluteRange,
  nearestPointIndex,
  rangePresets,
  rangeQuery,
  utcInput,
} from "./chart-range";
const now = Date.parse("2026-10-24T12:00:00Z");

test("absolute chart ranges use UTC and align inward to complete measurement intervals", () => {
  const range = absoluteRange(
    "2026-10-24T10:00:01",
    "2026-10-24T11:00:59",
    60,
    86400,
    now,
  );
  expect(range).toEqual({
    kind: "absolute",
    from: "2026-10-24T10:01:00.000Z",
    to: "2026-10-24T11:00:00.000Z",
  });
  expect(rangeQuery(range, 86400).get("from")).toBe(range.from);
  expect(utcInput(range.to)).toBe("2026-10-24T11:00:00");
});
test("rejects missing, invalid, inverted, future, oversized and sub-interval date ranges", () => {
  for (const [from, to] of [
    ["", "2026-10-24T11:00"],
    ["2026-02-30T10:00", "2026-03-01T11:00"],
    ["2026-10-24T11:00", "2026-10-24T10:00"],
    ["2026-10-24T11:00", "2026-10-24T12:01"],
    ["2026-10-22T10:00", "2026-10-24T11:00"],
    ["2026-10-24T10:00:01", "2026-10-24T10:00:59"],
    ["2026-10-24T10:00Z", "2026-10-24T11:00Z"],
  ])
    expect(
      () => absoluteRange(from, to, 60, 86400, now),
      `${from} / ${to}`,
    ).toThrow();
});
test("relative ranges stay server-resolved and presets respect small retention windows", () => {
  expect(rangePresets(120)).toEqual([120]);
  expect(rangePresets(3600)).toEqual([900, 3600]);
  expect(
    rangeQuery({ kind: "relative", seconds: 86400 }, 3600).toString(),
  ).toBe("rangeSeconds=3600");
});
test("pointer lookup uses timestamps rather than uniform point indexes", () => {
  const points = ["10:00", "10:01", "11:00"].map((time) => ({
    at: `2026-10-24T${time}:00Z`,
  }));
  expect(nearestPointIndex(points, Date.parse("2026-10-24T10:10:00Z"))).toBe(1);
  expect(nearestPointIndex(points, Date.parse("2026-10-24T10:50:00Z"))).toBe(2);
  expect(nearestPointIndex([], now)).toBe(-1);
});
