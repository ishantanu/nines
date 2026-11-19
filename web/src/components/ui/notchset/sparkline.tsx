/**
 * Sparkline adapted from MIT Notchset v1.0.0, Beautiful UI (Chimpy).
 * https://beautiful-ui.dev/components/sparkline
 * Nines: actual timestamps, no resampling/smoothing, gaps and partial coverage.
 * See /beautiful-ui-license.txt.
 */
import { sliDomain } from "@/lib/chart-range";

export interface SparkPoint {
  at: string;
  value: number | null;
  partial?: boolean;
}
export function Sparkline({
  points,
  label,
  target,
  from,
  to,
}: {
  points: readonly SparkPoint[];
  label: string;
  target?: number;
  from?: string;
  to?: string;
}) {
  const timestamps = points
    .map((p) => Date.parse(p.at))
    .filter(Number.isFinite);
  const start = from ? Date.parse(from) : Math.min(...timestamps),
    end = to ? Date.parse(to) : Math.max(...timestamps);
  const valid = (p: SparkPoint) =>
    p.value !== null &&
    Number.isFinite(p.value) &&
    Number.isFinite(Date.parse(p.at));
  const observed = points.filter(valid);
  if (!observed.length)
    return <span className="text-xs text-muted-foreground">No trend data</span>;
  const values = observed.map((p) => p.value!);
  const { low, high } = sliDomain(values, target ?? values[0]);
  const x = (p: SparkPoint) =>
    4 + ((Date.parse(p.at) - start) / Math.max(1, end - start)) * 144;
  const y = (p: SparkPoint) =>
    32 - ((p.value! - low) / Math.max(Number.EPSILON, high - low)) * 26;
  const path = (partial: boolean) =>
    points
      .map((p, i) => {
        if (!valid(p)) return "";
        const before = points[i - 1];
        const edgePartial = !!p.partial || !!before?.partial;
        return before && valid(before) && edgePartial === partial
          ? `M${x(before)},${y(before)}L${x(p)},${y(p)}`
          : "";
      })
      .join(" ");
  const valueLow = Math.min(...values),
    valueHigh = Math.max(...values);
  return (
    <svg
      data-slot="sparkline"
      className="select-none block h-10 w-36 max-w-full"
      viewBox="0 0 152 40"
      role="img"
      aria-label={`${label}. ${observed.length} observations, ${(valueLow * 100).toFixed(3)}% to ${(valueHigh * 100).toFixed(3)}%.${target === undefined ? "" : ` SLO target ${(target * 100).toFixed(3)}%.`} Dashed segments have partial coverage.`}
    >
      {target !== undefined ? (
        <line
          data-slot="sparkline-target"
          x1="4"
          x2="148"
          y1={y({ at: "", value: target })}
          y2={y({ at: "", value: target })}
          stroke="var(--muted-foreground)"
          strokeDasharray="2 3"
        />
      ) : null}
      <line x1="4" x2="148" y1="37" y2="37" stroke="var(--border)" />
      {Array.from({ length: 11 }, (_, i) => (
        <line
          key={i}
          x1={4 + i * 14.4}
          x2={4 + i * 14.4}
          y1="37"
          y2="39"
          stroke="var(--muted-foreground)"
        />
      ))}
      <path
        d={path(false)}
        fill="none"
        stroke="var(--chart-sli)"
        strokeWidth="1.6"
        vectorEffect="non-scaling-stroke"
      />
      <path
        d={path(true)}
        fill="none"
        stroke="var(--chart-sli)"
        strokeWidth="1.6"
        strokeDasharray="3 3"
        vectorEffect="non-scaling-stroke"
      />
      {points.map((p, i) => {
        if (!valid(p)) return null;
        const before = points[i - 1],
          after = points[i + 1];
        if (
          p !== observed.at(-1) &&
          ((before && valid(before)) || (after && valid(after)))
        )
          return null;
        return (
          <circle
            key={p.at}
            cx={x(p)}
            cy={y(p)}
            r="1.5"
            fill="var(--chart-sli)"
          >
            <title>
              {p.at}: {(p.value! * 100).toFixed(3)}%
              {p.partial ? " (partial window)" : ""}
            </title>
          </circle>
        );
      })}
    </svg>
  );
}
