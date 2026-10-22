import { test, expect, vi, afterEach } from "vitest";
import { MaskedReplayCapture, sanitizeReplayEvent } from "./replay-capture";
const mocks = vi.hoisted(() => ({ record: vi.fn(), stop: vi.fn() }));
vi.mock("@rrweb/record", () => ({ record: mocks.record }));
afterEach(() => {
  vi.useRealTimers();
  mocks.record.mockReset();
  mocks.stop.mockReset();
});
const snapshot = {
  type: 2,
  timestamp: 100,
  data: {
    node: {
      type: 0,
      id: 1,
      childNodes: [
        {
          type: 2,
          id: 2,
          tagName: "body",
          attributes: { onclick: "secret-script", title: "secret-title" },
          childNodes: [{ type: 3, id: 3, textContent: "secret@example.com" }],
        },
      ],
    },
    initialOffset: { left: 0, top: 0 },
  },
};
test("text, credentials, URLs and plugin data are masked before transport", () => {
  const clean = sanitizeReplayEvent(snapshot);
  expect(JSON.stringify(clean)).not.toContain("secret");
  const meta = sanitizeReplayEvent({
    type: 4,
    timestamp: 99,
    data: { href: "https://example.test/?key=secret", width: 800, height: 600 },
  });
  expect(meta?.data.href).toBe("/");
  expect(
    sanitizeReplayEvent({
      type: 3,
      timestamp: 101,
      data: { source: 11, payload: "secret" },
    }),
  ).toBeNull();
});
test("discarded attribute mutations cannot cause a recorder feedback loop", () => {
  expect(
    sanitizeReplayEvent({
      type: 3,
      timestamp: 101,
      data: {
        source: 0,
        attributes: [
          {
            id: 42,
            attributes: { "aria-valuenow": "50", "data-state": "ready" },
          },
        ],
      },
    }),
  ).toBeNull();
  expect(
    sanitizeReplayEvent({
      type: 3,
      timestamp: 102,
      data: {
        source: 0,
        attributes: [{ id: 42, attributes: { class: "active" } }],
      },
    })?.data.attributes,
  ).toEqual([{ id: 42, attributes: { class: "active" } }]);
});
test("consent is required and late emissions cannot restart a discarded recording", async () => {
  vi.useFakeTimers();
  const c = new MaskedReplayCapture();
  await expect(c.start(false)).rejects.toThrow("consent");
  expect(mocks.record).not.toHaveBeenCalled();
  let emit: (event: unknown) => void = () => {};
  mocks.record.mockImplementation((options) => {
    emit = options.emit;
    emit({
      type: 4,
      timestamp: 99,
      data: { href: "/?secret", width: 800, height: 600 },
    });
    emit(snapshot);
    return mocks.stop;
  });
  await c.start(true);
  expect(c.getSnapshot().status).toBe("recording");
  c.pause();
  expect(c.getSnapshot().status).toBe("paused");
  expect(mocks.stop).toHaveBeenCalledTimes(1);
  c.discard();
  emit(snapshot);
  expect(c.getSnapshot().eventCount).toBe(0);
  expect(c.getSnapshot().status).toBe("idle");
  expect(() => c.export()).toThrow();
});
test("a stopped recording has masked events and the five-minute bound is enforced", async () => {
  vi.useFakeTimers();
  mocks.record.mockImplementation((options) => {
    options.emit({
      type: 4,
      timestamp: 99,
      data: { href: "/", width: 800, height: 600 },
    });
    options.emit(snapshot);
    return mocks.stop;
  });
  const c = new MaskedReplayCapture();
  await c.start(true);
  vi.advanceTimersByTime(300000);
  expect(c.getSnapshot().status).toBe("ready");
  expect(JSON.stringify(c.export())).not.toContain("secret");
  c.discard();
});
test("SVG geometry and disclosure state survive masking without executable references", () => {
  const clean = sanitizeReplayEvent({
    type: 2,
    timestamp: 1,
    data: {
      node: {
        type: 0,
        id: 1,
        childNodes: [
          {
            type: 2,
            id: 2,
            tagName: "svg",
            isSVG: true,
            attributes: { viewBox: "0 0 800 300", onload: "secret" },
            childNodes: [
              {
                type: 2,
                id: 3,
                tagName: "path",
                isSVG: true,
                attributes: {
                  d: "M0 50 L100 20",
                  fill: "url(https://secret)",
                  stroke: "currentColor",
                },
                childNodes: [],
              },
              {
                type: 2,
                id: 4,
                tagName: "foreignObject",
                attributes: {},
                childNodes: [{ type: 3, id: 5, textContent: "secret" }],
              },
            ],
          },
          {
            type: 2,
            id: 6,
            tagName: "details",
            attributes: {
              open: "",
              "data-state": "open",
              "data-user": "secret",
              "data-orientation": "horizontal",
              "data-slot": "tabs",
            },
            childNodes: [],
          },
        ],
      },
    },
  });
  const json = JSON.stringify(clean);
  expect(json).toContain('"d":"M0 50 L100 20"');
  expect(json).toContain('"isSVG":true');
  expect(json).toContain('"open":""');
  expect(json).not.toContain("secret");
  expect(json).not.toContain("foreignObject");
});

test("shadcn layout attributes survive without arbitrary data fields", () => {
  const event = sanitizeReplayEvent({
    type: 3,
    timestamp: 1,
    data: {
      source: 0,
      attributes: [
        {
          id: 1,
          attributes: {
            "data-orientation": "horizontal",
            "data-slot": "tabs",
            "data-user": "secret",
          },
        },
      ],
    },
  });
  expect(event?.data.attributes).toEqual([
    {
      id: 1,
      attributes: { "data-orientation": "horizontal", "data-slot": "tabs" },
    },
  ]);
});
