export type ChartRange =
  | { kind: "relative"; seconds: number }
  | { kind: "absolute"; from: string; to: string };

export function rangePresets(maxSeconds: number): number[] {
  return [
    ...new Set(
      [900, 3600, 21600, 86400, 604800, 2419200, maxSeconds].filter(
        (n) => n <= maxSeconds && n > 0,
      ),
    ),
  ].sort((a, b) => a - b);
}

export function utcInput(iso: string): string {
  const date = new Date(iso);
  return Number.isFinite(date.getTime()) ? date.toISOString().slice(0, 19) : "";
}

export function absoluteRange(
  from: string,
  to: string,
  stepSeconds: number,
  maxSeconds: number,
  now = Date.now(),
): Extract<ChartRange, { kind: "absolute" }> {
  const parse = (value: string) => {
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?$/.test(value))
      throw Error("Enter a complete start and end date in UTC.");
    const normalized = value.length === 16 ? value + ":00" : value;
    const time = Date.parse(normalized + "Z");
    if (
      !Number.isFinite(time) ||
      utcInput(new Date(time).toISOString()) !== normalized
    )
      throw Error("Enter a valid calendar date and time in UTC.");
    return time;
  };
  const start = parse(from),
    end = parse(to);
  if (end <= start) throw Error("End time must be after start time.");
  if (end > now) throw Error("End time cannot be in the future.");
  if (end - start > maxSeconds * 1000)
    throw Error("The selected range exceeds the retained history limit.");
  const step = stepSeconds * 1000;
  const alignedStart = Math.ceil(start / step) * step;
  const alignedEnd = Math.floor(end / step) * step;
  if (alignedEnd <= alignedStart)
    throw Error("Select at least one complete measurement interval.");
  return {
    kind: "absolute",
    from: new Date(alignedStart).toISOString(),
    to: new Date(alignedEnd).toISOString(),
  };
}

export function rangeQuery(
  range: ChartRange,
  maxSeconds: number,
): URLSearchParams {
  return range.kind === "relative"
    ? new URLSearchParams({
        rangeSeconds: String(Math.min(range.seconds, maxSeconds)),
      })
    : new URLSearchParams({ from: range.from, to: range.to });
}

export function nearestPointIndex(
  points: { at: string }[],
  time: number,
): number {
  if (!points.length) return -1;
  let left = 0,
    right = points.length - 1;
  while (left < right) {
    const mid = Math.floor((left + right) / 2);
    if (Date.parse(points[mid].at) < time) left = mid + 1;
    else right = mid;
  }
  if (
    left > 0 &&
    time - Date.parse(points[left - 1].at) <= Date.parse(points[left].at) - time
  )
    return left - 1;
  return left;
}
