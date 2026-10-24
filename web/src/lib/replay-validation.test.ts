import { expect, test } from "vitest";
import { validateReplayEvents } from "./replay-validation";
import { replayFixture } from "./replay-test-fixtures";
test("a complete captured page is accepted without modifying its events", () => {
  const events = replayFixture(),
    before = JSON.stringify(events);
  validateReplayEvents(events);
  expect(JSON.stringify(events)).toBe(before);
});
test.each([
  ["empty stream", []],
  ["missing snapshot", [replayFixture()[0], replayFixture()[0]]],
  [
    "empty snapshot",
    [
      replayFixture()[0],
      {
        type: 2,
        timestamp: 101,
        data: { node: { type: 0, id: 1, childNodes: [] } },
      },
    ],
  ],
  [
    "out of order",
    [replayFixture()[0], { ...replayFixture()[1], timestamp: 99 }],
  ],
  [
    "excess duration",
    [replayFixture()[0], { ...replayFixture()[1], timestamp: 400000 }],
  ],
  [
    "invalid timestamp",
    [replayFixture()[0], { ...replayFixture()[1], timestamp: NaN }],
  ],
  [
    "unsupported event",
    [...replayFixture(), { type: 6, timestamp: 102, data: {} }],
  ],
  [
    "unsupported plugin",
    [...replayFixture(), { type: 3, timestamp: 102, data: { source: 11 } }],
  ],
])("%s fails before reaching the player", (_name, value) => {
  expect(() => validateReplayEvents(value)).toThrow();
});
test("deep or oversized snapshots fail with bounded errors", () => {
  const events = replayFixture();
  events[1].data.node = { type: 0, id: 1, childNodes: [] };
  let node = events[1].data.node as Record<string, unknown>;
  for (let i = 0; i < 65; i++) {
    const child = { type: 2, id: i + 2, tagName: "div", childNodes: [] };
    node.childNodes = [child];
    node = child;
  }
  expect(() => validateReplayEvents(events)).toThrow("snapshot");
  const huge = replayFixture();
  huge[0].data.extra = "x".repeat(3 * 1024 * 1024);
  expect(() => validateReplayEvents(huge)).toThrow("size");
});
