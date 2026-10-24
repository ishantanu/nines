import type { ReplayEvent } from "./replay-capture";

const sources = new Set([0, 1, 2, 3, 4, 5, 6, 12]);
function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Check stored/exported streams before handing them to the player. This is
// structural validation; masking remains a separate capture/server boundary.
export function validateReplayEvents(
  value: unknown,
): asserts value is ReplayEvent[] {
  if (!Array.isArray(value) || value.length < 2 || value.length > 2000)
    throw Error("Recording requires 2–2000 events.");
  if (
    new TextEncoder().encode(JSON.stringify(value)).byteLength >
    3 * 1024 * 1024
  )
    throw Error("Recording exceeds the player size limit.");
  let nodes = 0;
  function node(value: unknown, depth: number): boolean {
    if (
      !object(value) ||
      ++nodes > 12000 ||
      depth > 60 ||
      !Number.isFinite(value.id)
    )
      throw Error("Recording contains an invalid snapshot.");
    if (value.type === 3) {
      if (typeof value.textContent !== "string")
        throw Error("Invalid snapshot text.");
      return false;
    }
    if (value.type === 1) return false;
    if (
      (value.type !== 0 && value.type !== 2) ||
      !Array.isArray(value.childNodes)
    )
      throw Error("Recording contains an invalid snapshot.");
    if (value.type === 2 && typeof value.tagName !== "string")
      throw Error("Recording contains an invalid element.");
    let body = value.type === 2 && value.tagName === "body";
    for (const child of value.childNodes) body = node(child, depth + 1) || body;
    return body;
  }
  let first = 0,
    previous = 0,
    snapshot = false;
  for (const [index, event] of value.entries()) {
    if (
      !object(event) ||
      typeof event.timestamp !== "number" ||
      !Number.isFinite(event.timestamp) ||
      event.timestamp < 1 ||
      !object(event.data)
    )
      throw Error("Recording contains an invalid event.");
    if (index === 0) first = event.timestamp;
    if (event.timestamp < previous || event.timestamp - first > 300000)
      throw Error("Recording timestamps must be ordered within five minutes.");
    previous = event.timestamp;
    if (event.type === 2) {
      if (
        !object(event.data.node) ||
        event.data.node.type !== 0 ||
        !node(event.data.node, 0)
      )
        throw Error("Recording is missing a complete page snapshot.");
      snapshot = true;
    } else if (event.type === 3) {
      if (!snapshot || !sources.has(Number(event.data.source)))
        throw Error("Recording contains an unsupported timeline event.");
      if (event.data.source === 0) {
        for (const key of ["texts", "attributes", "removes", "adds"]) {
          const entries = event.data[key];
          if (
            !Array.isArray(entries) ||
            entries.length > 12000 ||
            !entries.every(object)
          )
            throw Error("Recording contains an invalid mutation.");
          if (key === "adds") for (const entry of entries) node(entry.node, 0);
        }
      }
    } else if (event.type !== 4)
      throw Error("Recording contains an unsupported event.");
  }
  if (!snapshot) throw Error("Recording is missing a complete page snapshot.");
}
