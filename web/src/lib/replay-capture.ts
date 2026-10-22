// Shared by the application and the standalone browser recorder bundle.
// No credentials, endpoint, telemetry or automatic upload is built into this recorder.
export type ReplayEvent = {
  type: number;
  timestamp: number;
  data: Record<string, unknown>;
};
const allowedTags = new Set(
  "html head body style div span p a button input textarea select option label form main section article header footer nav aside h1 h2 h3 h4 h5 h6 ul ol li table thead tbody tfoot tr th td caption colgroup col dl dt dd details summary pre code br hr strong em small b i u s img figure figcaption fieldset legend progress meter dialog svg g path rect circle ellipse line polyline polygon defs clippath lineargradient radialgradient stop text tspan".split(
    " ",
  ),
);
const geometry = new Set(
  "viewbox d x y x1 y1 x2 y2 cx cy r rx ry width height points transform stroke-width stroke-linecap stroke-linejoin fill stroke fill-opacity stroke-opacity opacity offset stop-color".split(
    " ",
  ),
);
const presentation: Record<string, string[]> = {
  "data-orientation": ["horizontal", "vertical"],
  "data-variant": [
    "default",
    "line",
    "outline",
    "secondary",
    "destructive",
    "ghost",
    "link",
  ],
  "data-slot": [
    "tabs",
    "tabs-list",
    "tabs-trigger",
    "tabs-content",
    "card",
    "card-header",
    "card-title",
    "card-description",
    "card-action",
    "card-content",
    "card-footer",
    "badge",
    "button",
    "input",
    "progress",
    "progress-indicator",
    "select",
    "select-trigger",
    "select-value",
    "select-content",
    "select-item",
    "table",
    "table-header",
    "table-body",
    "table-row",
    "table-head",
    "table-cell",
  ],
};
const flags = new Set(["open", "hidden", "disabled", "checked", "selected"]);
const mask = (s: string) => s.slice(0, 20000).replace(/\S/gu, "*");
const css = (s: string) =>
  s.length > 256 * 1024
    ? ""
    : s
        .replace(/url\s*\([^)]*\)|@import[^;]*;|content\s*:[^;}]*/gis, "")
        .replace(/https?:\/\/[^\s"'<>;)]+/gi, "");
function attrs(value: unknown) {
  const out: Record<string, string | null> = {};
  for (const [key, v] of Object.entries(
    (value ?? {}) as Record<string, unknown>,
  )) {
    if (v === null && (key === "class" || key === "style" || flags.has(key))) {
      out[key] = null;
      continue;
    }
    if (typeof v !== "string") continue;
    if (presentation[key]?.includes(v)) {
      out[key] = v;
      continue;
    }
    if (flags.has(key)) {
      out[key] = "";
      continue;
    }
    if (
      geometry.has(key.toLowerCase()) &&
      v.length <= 20000 &&
      /^[a-zA-Z0-9#.,%() +\-]*$/.test(v) &&
      !/url|https|data|javascript/i.test(v)
    ) {
      out[key] = v;
      continue;
    }
    if (
      key === "data-state" &&
      ["open", "closed", "active", "inactive", "checked", "unchecked"].includes(
        v,
      )
    ) {
      out[key] = v;
      continue;
    }
    if (key === "class" && v.length <= 1024) out[key] = v;
    else if (["style", "_cssText"].includes(key)) out[key] = css(v);
    else if (
      ["width", "height", "rr_width", "rr_height"].includes(key) &&
      /^[0-9.]{1,6}(px|%)?$/.test(v)
    )
      out[key] = v;
    else if (key === "type" && ["checkbox", "radio", "button"].includes(v))
      out[key] = v;
  }
  return out;
}
function pick(value: unknown, keys: string[]) {
  const m = (value ?? {}) as Record<string, unknown>;
  return Object.fromEntries(
    keys
      .filter(
        (k) =>
          typeof m[k] === "boolean" ||
          (typeof m[k] === "number" &&
            Number.isFinite(m[k]) &&
            (m[k] as number) >= -100000 &&
            (m[k] as number) <= 10000000),
      )
      .map((k) => [k, m[k]]),
  );
}
function node(
  value: unknown,
  depth = 0,
  style = false,
): Record<string, unknown> {
  if (depth > 60) throw Error("Snapshot is too deep");
  const m = value as Record<string, unknown>;
  const type = Number(m.type);
  const out: Record<string, unknown> = { type, id: Number(m.id) };
  if (type === 0 || type === 2) {
    let tag = String(m.tagName ?? "").toLowerCase();
    if (tag === "link") tag = "style";
    if (type === 2) {
      if (!allowedTags.has(tag))
        return {
          type: 2,
          id: Number(m.id),
          tagName: "div",
          attributes: {},
          childNodes: [],
        };
      out.tagName =
        (
          {
            clippath: "clipPath",
            lineargradient: "linearGradient",
            radialgradient: "radialGradient",
          } as Record<string, string>
        )[tag] ?? tag;
      out.attributes = attrs(m.attributes);
      if (m.isSVG === true) out.isSVG = true;
      if (typeof m.isShadow === "boolean") out.isShadow = m.isShadow;
    }
    out.childNodes = Array.isArray(m.childNodes)
      ? m.childNodes.map((c) => node(c, depth + 1, tag === "style"))
      : [];
  } else if (type === 1) {
    out.name = "html";
    out.publicId = "";
    out.systemId = "";
  } else if ([3, 4, 5].includes(type)) {
    out.type = 3;
    out.textContent = style
      ? css(String(m.textContent ?? ""))
      : mask(String(m.textContent ?? ""));
  } else throw Error("Unsupported snapshot node");
  return out;
}
export function sanitizeReplayEvent(value: unknown): ReplayEvent | null {
  const e = value as ReplayEvent;
  if (!e || !Number.isFinite(e.timestamp) || !e.data)
    throw Error("Invalid replay event");
  const d = e.data;
  let data: Record<string, unknown> = {};
  if (e.type === 4) data = { ...pick(d, ["width", "height"]), href: "/" };
  else if (e.type === 2)
    data = {
      node: node(d.node),
      initialOffset: pick(d.initialOffset, ["left", "top"]),
    };
  else if (e.type === 3) {
    const source = Number(d.source);
    data.source = source;
    if (source === 0) {
      for (const key of ["texts", "attributes", "removes", "adds"]) {
        const entries = Array.isArray(d[key])
          ? (d[key] as Record<string, unknown>[])
          : [];
        data[key] = entries
          .map<Record<string, unknown>>((m) => ({
            ...pick(m, ["id", "parentId", "nextId", "previousId", "isShadow"]),
            ...(key === "texts"
              ? { value: mask(String(m.value ?? "")) }
              : key === "attributes"
                ? { attributes: attrs(m.attributes) }
                : key === "adds"
                  ? { node: node(m.node) }
                  : {}),
          }))
          .filter(
            (m) =>
              key !== "attributes" ||
              Object.keys((m.attributes ?? {}) as Record<string, unknown>)
                .length > 0,
          );
      }
      if (
        ["texts", "attributes", "removes", "adds"].every(
          (key) => (data[key] as unknown[]).length === 0,
        )
      )
        return null;
    } else if ([1, 6, 12].includes(source))
      data.positions = (Array.isArray(d.positions) ? d.positions : []).map(
        (p) => pick(p, ["id", "x", "y", "timeOffset"]),
      );
    else if (source === 2)
      data = { source, ...pick(d, ["type", "id", "x", "y"]) };
    else if (source === 3) data = { source, ...pick(d, ["id", "x", "y"]) };
    else if (source === 4) data = { source, ...pick(d, ["width", "height"]) };
    else if (source === 5)
      data = {
        source,
        ...pick(d, ["id", "isChecked"]),
        text: mask(String(d.text ?? "")),
      };
    else return null;
  } else return null;
  return { type: e.type, timestamp: e.timestamp, data };
}
export type CaptureState = {
  status: "idle" | "recording" | "paused" | "ready";
  eventCount: number;
  bytes: number;
  duration: number;
  reason: string;
};
export class MaskedReplayCapture {
  private events: ReplayEvent[] = [];
  private bytes = 0;
  private listeners = new Set<() => void>();
  private stopSegment: (() => void) | undefined;
  private generation = 0;
  private timer: ReturnType<typeof setInterval> | undefined;
  private started = 0;
  private state: CaptureState = {
    status: "idle",
    eventCount: 0,
    bytes: 0,
    duration: 0,
    reason: "",
  };
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  getSnapshot = () => this.state;
  private update(value: Partial<CaptureState>) {
    this.state = { ...this.state, ...value };
    this.listeners.forEach((f) => f());
  }
  async start(consent: boolean) {
    if (!consent) throw Error("Explicit recording consent is required");
    if (this.state.status !== "idle")
      throw Error("Finish or discard the current recording");
    this.started = Date.now();
    this.update({ status: "paused", reason: "" });
    this.timer = setInterval(() => {
      this.update({
        duration: Date.now() - this.started,
        eventCount: this.events.length,
        bytes: this.bytes,
      });
      if (Date.now() - this.started >= 300000)
        this.stop("Five-minute recording limit reached.");
    }, 1000);
    try {
      await this.resume();
    } catch (e) {
      this.discard();
      throw e;
    }
  }
  async resume() {
    if (this.state.status !== "paused") return;
    const gen = ++this.generation;
    const { record } = await import("@rrweb/record");
    if (gen !== this.generation || this.state.status !== "paused") return;
    this.update({ status: "recording" });
    this.stopSegment =
      record({
        emit: (value) => {
          if (gen !== this.generation || this.state.status !== "recording")
            return;
          try {
            const event = sanitizeReplayEvent(value);
            if (!event) return;
            const size = new TextEncoder().encode(
              JSON.stringify(event),
            ).byteLength;
            if (this.events.length >= 2000 || this.bytes + size > 1900000) {
              this.stop("Recording size limit reached.");
              return;
            }
            this.events.push(event);
            this.bytes += size;
          } catch {
            this.stop(
              "Recording stopped because an event could not be masked.",
            );
          }
        },
        maskAllInputs: true,
        maskTextSelector: "*",
        blockSelector:
          '[data-replay-block], [data-private], iframe, video, audio, canvas, input[type="password"]',
        inlineStylesheet: true,
        recordCanvas: false,
        recordCrossOriginIframes: false,
        collectFonts: false,
        slimDOMOptions: "all",
        sampling: { mousemove: 100, scroll: 200, input: "last" },
      }) ?? undefined;
    if (gen !== this.generation) {
      this.stopSegment?.();
      this.stopSegment = undefined;
    }
    if (gen === this.generation)
      this.update({ eventCount: this.events.length, bytes: this.bytes });
  }
  pause() {
    if (this.state.status !== "recording" && this.state.status !== "paused")
      return;
    ++this.generation;
    this.stopSegment?.();
    this.stopSegment = undefined;
    this.update({ status: "paused" });
  }
  stop(reason = "") {
    if (this.state.status === "idle" || this.state.status === "ready") return;
    ++this.generation;
    this.stopSegment?.();
    this.stopSegment = undefined;
    if (this.timer) clearInterval(this.timer);
    this.timer = undefined;
    this.update({
      status: "ready",
      eventCount: this.events.length,
      bytes: this.bytes,
      duration: Date.now() - this.started,
      reason,
    });
  }
  discard() {
    ++this.generation;
    this.stopSegment?.();
    this.stopSegment = undefined;
    if (this.timer) clearInterval(this.timer);
    this.timer = undefined;
    this.events = [];
    this.bytes = 0;
    this.update({
      status: "idle",
      eventCount: 0,
      bytes: 0,
      duration: 0,
      reason: "",
    });
  }
  export() {
    if (
      this.state.status !== "ready" ||
      !this.events.some((e) => e.type === 2) ||
      this.events.length < 2
    )
      throw Error("Stop a recording with a full snapshot before saving");
    return structuredClone(this.events);
  }
}
