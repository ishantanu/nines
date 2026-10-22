import { expect, it } from "vitest";
import { duration, durationSeconds } from "./history";
it("round trips aligned window durations including weeks and days", () => {
  for (const text of ["28d", "1h30m", "0s", "1w2d", "60s"]) {
    const seconds = durationSeconds(text);
    expect(durationSeconds(duration(seconds))).toBe(seconds);
  }
  expect(durationSeconds("28d")).toBe(2419200);
  expect(durationSeconds("5m")).toBe(300);
});
it("rejects ambiguous, fractional and overflowing window values", () => {
  for (const text of ["1.5h", "-1h", "1", "9999999999999d", "5 minutes", ""])
    expect(() => durationSeconds(text)).toThrow();
});
