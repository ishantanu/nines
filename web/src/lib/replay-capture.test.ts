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

test("an unavailable recorder fails visibly and clears its recording timer", async () => {
  vi.useFakeTimers();
  mocks.record.mockReturnValue(undefined);
  const capture = new MaskedReplayCapture();
  await expect(capture.start(true)).rejects.toThrow("could not start");
  expect(capture.getSnapshot().status).toBe("idle");
  expect(capture.getSnapshot().eventCount).toBe(0);
  expect(vi.getTimerCount()).toBe(0);
});

test("a synchronous masking failure releases the newly initialized recorder", async () => {
  vi.useFakeTimers();
  mocks.record.mockImplementation((options) => {
    options.emit({ type: 2, timestamp: 100, data: { node: { type: 99 } } });
    return mocks.stop;
  });
  const capture = new MaskedReplayCapture();
  await capture.start(true);
  expect(capture.getSnapshot().status).toBe("ready");
  expect(capture.getSnapshot().reason).toContain("could not be masked");
  expect(mocks.stop).toHaveBeenCalledOnce();
  expect(vi.getTimerCount()).toBe(0);
  expect(() => capture.export()).toThrow("full snapshot");
  capture.discard();
});

test("CSS masking removes generated text without corrupting layout or quoted values", () => {
  const event = sanitizeReplayEvent({
    type: 3,
    timestamp: 100,
    data: {
      source: 0,
      attributes: [
        {
          id: 1,
          attributes: {
            style:
              'display:flex;justify-content:center;align-content:stretch;--tw-content:"secret; token";content:"secret; text";color:red',
          },
        },
      ],
    },
  });
  const attrs = (
    event!.data.attributes as { attributes: { style: string } }[]
  )[0].attributes;
  expect(attrs.style).toContain("justify-content:center");
  expect(attrs.style).toContain("align-content:stretch");
  expect(attrs.style).toContain("color:red");
  expect(attrs.style).not.toContain("secret");
  expect(attrs.style).not.toContain("--tw-;");
});

test("private-region markers remove the complete subtree before transport", () => {
  for (const marker of ["data-private", "data-replay-block"]) {
    const incoming = structuredClone(snapshot);
    const body = incoming.data.node.childNodes[0];
    body.attributes = { ...body.attributes, [marker]: "" };
    const serialized = JSON.stringify(sanitizeReplayEvent(incoming));
    expect(serialized).not.toContain("textContent");
    expect(serialized).not.toContain("secret");
    expect(serialized).toContain('"childNodes":[]');
  }
});

// Run the identical adversarial stream through the browser and Go sanitizers.
import selectiveEvents from "../../../internal/replay/testdata/selective-events.json";
import { publicReplayText } from "./replay-labels";
test("only catalogued, directly annotated labels survive snapshots and mutations", () => {
  const labels = new Map<number, string>();
  const clean = selectiveEvents.map((e) => sanitizeReplayEvent(e, labels)!);
  const body = (clean[1].data.node as any).childNodes[0].childNodes;
  expect(body[0].childNodes[0].textContent).toBe("Home");
  expect(body[1].textContent).toBe("****");
  expect(body[4].childNodes).toEqual([]);
  expect(body[5].childNodes[0].textContent).toBe("****");
  expect(body[6].childNodes[0].textContent).toBe("No SLI data");
  expect(body[8].childNodes[0].childNodes[0].textContent).toBe("****");
  expect((clean[2].data.texts as any[]).map((t) => t.value)).toEqual([
    "Home",
    "****",
    "*********************",
    "No SLI data",
  ]);
  expect((clean[2].data.adds as any[]).map((t) => t.node.textContent)).toEqual([
    "Rules",
    "****",
    "****",
  ]);
  expect(clean[3].data.text).toBe("****");
  expect(JSON.stringify(clean[4])).not.toContain('"Home"');
  expect((clean[5].data.texts as any[])[0].value).toBe("****");
  expect(JSON.stringify(clean)).not.toContain("secret");
  const again = new Map<number, string>();
  expect(clean.map((e) => sanitizeReplayEvent(e, again))).toEqual(clean);
  expect(publicReplayText("Home secret", "Home")).toBe("**** ******");
});
