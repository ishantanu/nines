import { expect, test } from "vitest";
import { replayFixture } from "./replay-test-fixtures";
import { portableReplayHTML } from "./replay-download";
test("portable recording embeds its own player and escapes untrusted closing tags", () => {
  const html = portableReplayHTML(
    [
      ...replayFixture(),
      {
        type: 3,
        timestamp: 102,
        data: { source: 5, id: 4, text: "</script><script>alert(1)</script>" },
      },
    ],
    'console.log("trusted player")',
    "ab".repeat(16),
  );
  expect(html).toContain("connect-src 'none'");
  expect(html).toContain("script-src 'nonce-");
  expect(html).toContain('id="replay-events"');
  expect(html).toContain("console.log");
  expect(html).not.toContain('src="/src/replay-player.ts"');
  expect(html).not.toContain("<script>alert(1)</script>");
  expect(html).toContain("\\u003c/script>");
});
test("portable replay rejects an empty player and oversized embedded recording", () => {
  const events = replayFixture();
  events[0].data.text = "x".repeat(3 * 1024 * 1024);
  expect(() => portableReplayHTML(events, "player", "ab".repeat(16))).toThrow(
    "size",
  );
  expect(() =>
    portableReplayHTML(replayFixture(), " ", "ab".repeat(16)),
  ).toThrow("player");
});
