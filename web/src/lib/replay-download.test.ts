import { expect, test } from "vitest";
import { portableReplayHTML } from "./replay-download";
test("portable recording embeds its own player and escapes untrusted closing tags", () => {
  const html = portableReplayHTML(
    [
      {
        type: 2,
        timestamp: 1,
        data: {
          node: {
            type: 0,
            childNodes: [{ textContent: "</script><script>alert(1)</script>" }],
          },
        },
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
