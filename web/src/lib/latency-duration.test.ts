import { expect, test } from "vitest";
import { latencySeconds, latencyDuration } from "./latency-duration";
test.each([
  ["30ms", 0.03],
  ["30s", 30],
  ["300us", 0.0003],
  ["300µs", 0.0003],
  ["1ns", 1e-9],
  [".03s", 0.03],
  ["1m", 60],
])("preserves units in %s", (input, expected) => {
  expect(latencySeconds(String(input))).toBe(expected);
});
test.each(["30", "30seconds", "0ms", "-1ms", "NaNms", "30ms junk"])(
  "rejects ambiguous or invalid %s",
  (value) => {
    expect(() => latencySeconds(value)).toThrow();
  },
);
test("format and parse retains a 30ms threshold", () => {
  expect(latencyDuration(0.03)).toBe("30ms");
  expect(latencySeconds(latencyDuration(0.03))).toBe(0.03);
});
