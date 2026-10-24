import type { ReplayEvent } from "./replay-capture";
export function replayFixture(): ReplayEvent[] {
  return [
    { type: 4, timestamp: 100, data: { href: "/", width: 800, height: 600 } },
    {
      type: 2,
      timestamp: 101,
      data: {
        node: {
          type: 0,
          id: 1,
          childNodes: [
            {
              type: 2,
              id: 2,
              tagName: "html",
              attributes: {},
              childNodes: [
                {
                  type: 2,
                  id: 3,
                  tagName: "body",
                  attributes: {},
                  childNodes: [{ type: 3, id: 4, textContent: "******" }],
                },
              ],
            },
          ],
        },
        initialOffset: { left: 0, top: 0 },
      },
    },
  ];
}
