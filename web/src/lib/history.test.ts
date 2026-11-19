import { expect, it } from "vitest";
import { duration, durationSeconds, historyRetentionSeconds } from "./history";
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

it("keeps at least a day of chart history without shortening long objective retention", () => {
  expect(historyRetentionSeconds(60)).toBe(86400);
  expect(historyRetentionSeconds(600)).toBe(86400);
  expect(historyRetentionSeconds(70, 7)).toBe(86401);
  expect(historyRetentionSeconds(86400)).toBe(172800);
  expect(historyRetentionSeconds(28 * 86400)).toBe(56 * 86400);
});
