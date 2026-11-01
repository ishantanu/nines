// Latency thresholds may be subsecond; burn and collection windows are separate.
export function latencySeconds(text: string): number {
  const match =
    /^\s*((?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+))\s*(ns|us|µs|μs|ms|s|m|h)\s*$/.exec(
      text,
    );
  if (!match)
    throw Error("Include a duration unit, for example 30ms, 300us or 0.03s.");
  const factors: Record<string, number> = {
    ns: 1e-9,
    us: 1e-6,
    µs: 1e-6,
    μs: 1e-6,
    ms: 1e-3,
    s: 1,
    m: 60,
    h: 3600,
  };
  const seconds = Number(
    (Number(match[1]) * factors[match[2]]).toPrecision(15),
  );
  if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 86400)
    throw Error("Latency must be greater than zero and at most 24h.");
  return seconds;
}
export function latencyDuration(seconds: number): string {
  const [factor, unit] =
    seconds < 1e-6
      ? [1e9, "ns"]
      : seconds < 0.001
        ? [1e6, "us"]
        : seconds < 1
          ? [1000, "ms"]
          : [1, "s"];
  return `${Number((seconds * Number(factor)).toPrecision(15))}${unit}`;
}
