import { useId, useRef } from "react";
import { duration, type HistoryPoint, type SLOHistory } from "@/lib/history";
import { nearestPointIndex } from "@/lib/chart-range";
import { useChartSize } from "@/lib/use-chart-size";
import { ReplayLabel } from "@/components/replay-label";
import { Card, CardHeader, CardContent } from "@/components/ui/card";
interface Line {
  name: string;
  color: string;
  get: (p: HistoryPoint) => number | null;
  dashed?: boolean;
  partial?: (p: HistoryPoint) => boolean;
}
export const sliTimelineLines: Line[] = [
  {
    name: "Measured SLI",
    color: "var(--chart-sli)",
    get: (p) => (p.coverage > 0 ? p.reliability : null),
    partial: (p) => p.coverage < 1,
  },
];

export function Timeline({
  title,
  history,
  lines,
  threshold,
  thresholdLabel,
  index,
  onIndex,
  percentages = false,
}: {
  title: string;
  history: SLOHistory;
  lines: Line[];
  threshold: number;
  thresholdLabel: string;
  index: number;
  onIndex: (i: number) => void;
  percentages?: boolean;
}) {
  const { ref, width, height } = useChartSize();
  const id = useId();
  const plot = useRef<SVGSVGElement>(null);
  const finite = (v: number | null): v is number =>
    v !== null && Number.isFinite(v);
  const values = history.points
    .flatMap((p) => lines.map((line) => line.get(p)))
    .filter(finite);
  let low = threshold,
    high = threshold;
  for (const value of values) {
    low = Math.min(low, value);
    high = Math.max(high, value);
  }
  const pad = Math.max((high - low) * 0.12, percentages ? 0.0001 : 0.25);
  low -= pad;
  high += pad;
  if (title === "Burn rate") low = Math.max(0, low);
  if (percentages) high = Math.min(1, high);
  if (title === "SLI and SLO target") {
    low = Math.max(0, low);
    high = Math.min(1, high);
  }
  const left = width < 400 ? 80 : 92,
    right = Math.max(left + 1, width - 12),
    top = 16,
    bottom = Math.max(top + 1, height - 36);
  const start = Date.parse(history.from),
    end = Date.parse(history.to),
    span = Math.max(1, end - start);
  const xTime = (time: number) =>
    left + ((time - start) / span) * (right - left);
  const x = (i: number) => xTime(Date.parse(history.points[i].at));
  const y = (value: number) =>
    bottom -
    ((value - low) / Math.max(Number.EPSILON, high - low)) * (bottom - top);
  const precision = Math.min(
    4,
    Math.max(
      title === "SLI and SLO target" ? 2 : 0,
      Math.ceil(-Math.log10(Math.max(1e-9, ((high - low) * 100) / 4))),
    ),
  );
  const format = (value: number) =>
    percentages
      ? `${(value * 100).toFixed(precision)}%`
      : `${value.toFixed(1)}×`;
  const inspectValue = (value: number) =>
    percentages
      ? `${(value * 100).toFixed(title === "SLI and SLO target" ? 3 : 1)}%`
      : `${value.toFixed(2)}×`;
  const timeLabel = (time: number) => {
    const iso = new Date(time).toISOString();
    return span >= 86400000
      ? width < 480
        ? iso.slice(5, 10)
        : `${iso.slice(5, 10)} ${iso.slice(11, 16)}`
      : iso.slice(11, 16);
  };
  // An edge touching incomplete history is dashed, including the transition
  // to a complete window. Missing observations never acquire connecting edges.
  function path(line: Line, partial = false) {
    let pen = false;
    const segments: string[] = [];
    history.points.forEach((point, i) => {
      const value = line.get(point);
      const previous = i > 0 ? history.points[i - 1] : undefined;
      const before = previous ? line.get(previous) : null;
      if (!finite(value)) {
        pen = false;
        return;
      }
      const edgePartial =
        !!line.partial &&
        (line.partial(point) || (!!previous && line.partial(previous)));
      if (finite(before) && edgePartial === partial) {
        if (!pen)
          segments.push(`M${x(i - 1).toFixed(2)},${y(before).toFixed(2)}`);
        segments.push(`L${x(i).toFixed(2)},${y(value).toFixed(2)}`);
        pen = true;
      } else {
        pen = false;
        if (!!line.partial?.(point) === partial) {
          segments.push(`M${x(i).toFixed(2)},${y(value).toFixed(2)}`);
        }
      }
    });
    return segments.join(" ");
  }
  const selectedIndex = Math.max(0, Math.min(index, history.points.length - 1));
  const selected = history.points[selectedIndex];
  const hasPartial = lines.some((line) =>
    history.points.some(
      (point) => finite(line.get(point)) && line.partial?.(point),
    ),
  );
  const valueText = (line: Line, point: HistoryPoint) => {
    const value = line.get(point);
    return finite(value)
      ? `${inspectValue(value)}${line.partial?.(point) ? ` (incomplete window, ${point.coverage < 0.001 ? "<0.1" : (point.coverage * 100).toFixed(1)}% coverage)` : ""}`
      : "No data";
  };
  const selectedText = selected
    ? `${new Date(selected.at).toUTCString()}. ${lines.map((line) => `${line.name}: ${valueText(line, selected)}`).join(". ")}. ${thresholdLabel}`
    : "No measurements available";
  const ticks = Math.max(2, Math.min(6, Math.floor((right - left) / 120) + 1));
  function inspect(clientX: number, bounds: DOMRect) {
    if (!history.points.length || bounds.width <= 0) return;
    const ratio = Math.max(
      0,
      Math.min(
        1,
        (((clientX - bounds.left) * width) / bounds.width - left) /
          (right - left),
      ),
    );
    onIndex(nearestPointIndex(history.points, start + ratio * span));
  }
  return (
    <Card className="chart-panel">
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-base font-semibold">
            <ReplayLabel>{title}</ReplayLabel>
          </h3>
        </div>
        <div className="flex flex-wrap gap-x-3 gap-y-2 text-sm text-foreground">
          {lines.map((line) => (
            <span key={line.name} className="inline-flex items-center gap-1.5">
              <span
                aria-hidden="true"
                className="inline-block w-6"
                style={{
                  borderTop: `2px ${line.dashed ? "dashed" : "solid"} ${line.color}`,
                }}
              />
              <ReplayLabel>{line.name}</ReplayLabel>
            </span>
          ))}
          <span className="inline-flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className="w-6 border-t-2 border-dotted border-muted-foreground"
            />
            {thresholdLabel}
          </span>
        </div>
        {hasPartial ? (
          <p className="text-sm text-muted-foreground">
            Dashed SLI segments have incomplete{" "}
            {duration(history.windowSeconds)} history. They do not establish
            full-window attainment.
          </p>
        ) : null}
      </CardHeader>
      <CardContent className="flex min-h-0 flex-col">
        <div
          ref={ref}
          className="relative min-h-56 w-full flex-1"
          data-chart-host
        >
          <svg
            viewBox={`0 0 ${width} ${height}`}
            className="chart-inspection-plot absolute inset-0 block size-full touch-pan-y rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            ref={plot}
            role="slider"
            tabIndex={0}
            aria-orientation="horizontal"
            aria-valuemin={0}
            aria-valuemax={Math.max(0, history.points.length - 1)}
            aria-valuenow={selectedIndex}
            aria-valuetext={selectedText}
            aria-disabled={!history.points.length}
            onKeyDown={(event) => {
              if (!history.points.length) return;
              const moves: Record<string, number> = {
                ArrowLeft: selectedIndex - 1,
                ArrowDown: selectedIndex - 1,
                ArrowRight: selectedIndex + 1,
                ArrowUp: selectedIndex + 1,
                Home: 0,
                End: history.points.length - 1,
                PageDown:
                  selectedIndex -
                  Math.max(1, Math.ceil(history.points.length / 10)),
                PageUp:
                  selectedIndex +
                  Math.max(1, Math.ceil(history.points.length / 10)),
              };
              if (Object.hasOwn(moves, event.key)) {
                event.preventDefault();
                onIndex(
                  Math.max(
                    0,
                    Math.min(history.points.length - 1, moves[event.key]),
                  ),
                );
              }
            }}
            aria-labelledby={`${id}-title`}
            aria-describedby={`${id}-description`}
            onPointerMove={(event) =>
              inspect(
                event.clientX,
                event.currentTarget.getBoundingClientRect(),
              )
            }
            onPointerDown={(event) => {
              plot.current?.focus({ preventScroll: true });
              inspect(
                event.clientX,
                event.currentTarget.getBoundingClientRect(),
              );
            }}
          >
            <title id={`${id}-title`}>{title}</title>
            <desc id={`${id}-description`}>
              {thresholdLabel}. Hover or tap the plot to inspect values. Focus
              the chart and use arrow keys to select adjacent observations, Home
              or End for the first or last observation, and Page Up or Page Down
              to move through the range. Times are UTC. Gaps indicate missing
              measurements or no traffic.{" "}
              {hasPartial
                ? "Dashed SLI segments indicate incomplete history."
                : ""}
            </desc>
            <defs>
              <clipPath id={`${id}-clip`}>
                <rect
                  x={left}
                  y={top}
                  width={right - left}
                  height={bottom - top}
                />
              </clipPath>
            </defs>
            {[0, 1, 2, 3, 4].map((i) => {
              const value = low + ((high - low) * i) / 4;
              return (
                <g key={i}>
                  <line
                    x1={left}
                    x2={right}
                    y1={y(value)}
                    y2={y(value)}
                    stroke="var(--border)"
                  />
                  <text
                    x={left - 8}
                    y={y(value) + 4}
                    textAnchor="end"
                    fill="var(--muted-foreground)"
                    fontSize={13}
                  >
                    {format(value)}
                  </text>
                </g>
              );
            })}
            <g clipPath={`url(#${id}-clip)`}>
              <line
                x1={left}
                x2={right}
                y1={y(threshold)}
                y2={y(threshold)}
                stroke="var(--muted-foreground)"
                strokeDasharray="2 4"
                strokeWidth={2}
              />
              {title === "SLI and SLO target"
                ? history.alertEvents
                    .filter(
                      (event) =>
                        (event.kind === "firing" ||
                          event.kind === "resolved") &&
                        Date.parse(event.time) >= start &&
                        Date.parse(event.time) <= end,
                    )
                    .map((event) => (
                      <g key={event.id}>
                        <title>
                          {event.kind} · {event.time}
                          {event.reason ? ` · ${event.reason}` : ""}
                        </title>
                        <line
                          x1={xTime(Date.parse(event.time))}
                          x2={xTime(Date.parse(event.time))}
                          y1={top}
                          y2={bottom}
                          stroke={
                            event.kind === "firing"
                              ? "var(--status-degraded)"
                              : "var(--status-healthy)"
                          }
                          strokeDasharray={
                            event.kind === "firing" ? "2 4" : "6 3"
                          }
                          strokeWidth={1.5}
                        />
                      </g>
                    ))
                : null}
              {lines.map((line) => (
                <g key={line.name} data-series={line.name}>
                  <path
                    d={path(line)}
                    fill="none"
                    stroke={line.color}
                    strokeWidth={2}
                    strokeDasharray={line.dashed ? "4 3" : undefined}
                    vectorEffect="non-scaling-stroke"
                  />
                  {line.partial ? (
                    <path
                      d={path(line, true)}
                      fill="none"
                      stroke={line.color}
                      strokeWidth={2}
                      strokeDasharray="6 4"
                      vectorEffect="non-scaling-stroke"
                    />
                  ) : null}
                  {history.points.map((point, i) => {
                    const value = line.get(point);
                    if (!finite(value)) return null;
                    const before =
                      i > 0 && finite(line.get(history.points[i - 1]));
                    const after =
                      i < history.points.length - 1 &&
                      finite(line.get(history.points[i + 1]));
                    return !before && !after ? (
                      <circle
                        key={point.at}
                        cx={x(i)}
                        cy={y(value)}
                        r={2.5}
                        fill={line.color}
                      />
                    ) : null;
                  })}
                </g>
              ))}
              {title === "SLI and SLO target"
                ? history.points.map((point, i) =>
                    point.targetViolated &&
                    finite(point.reliability) &&
                    point.coverage === 1 ? (
                      <path
                        key={point.at}
                        d={`M${x(i)},${y(point.reliability!) + 4} l-4,-7 h8 Z`}
                        fill="var(--status-degraded)"
                      >
                        <title>SLI below target at {utcStamp(point.at)}</title>
                      </path>
                    ) : null,
                  )
                : null}
              {selected ? (
                <g>
                  <line
                    x1={x(selectedIndex)}
                    x2={x(selectedIndex)}
                    y1={top}
                    y2={bottom}
                    stroke="var(--muted-foreground)"
                    strokeDasharray="3 3"
                  />
                  {lines.map((line) => {
                    const value = line.get(selected);
                    return finite(value) ? (
                      <circle
                        key={line.name}
                        cx={x(selectedIndex)}
                        cy={y(value)}
                        r={3}
                        fill={line.color}
                        stroke="var(--card)"
                        strokeWidth={1.5}
                      />
                    ) : null;
                  })}
                </g>
              ) : null}
            </g>
            {Array.from({ length: ticks }, (_, i) => {
              const time = start + (span * i) / (ticks - 1);
              return (
                <text
                  key={i}
                  x={xTime(time)}
                  y={height - 9}
                  fill="var(--muted-foreground)"
                  fontSize={13}
                  textAnchor={
                    i === 0 ? "start" : i === ticks - 1 ? "end" : "middle"
                  }
                >
                  {timeLabel(time)}
                </text>
              );
            })}
          </svg>
          {!values.length ? (
            <p className="pointer-events-none absolute inset-x-0 top-1/2 text-center text-sm text-muted-foreground">
              No measurements available in this range
            </p>
          ) : null}
        </div>
        <div
          className="mt-3 flex flex-wrap items-baseline justify-between gap-x-5 gap-y-2 border-t pt-3 text-xs"
          aria-live="off"
        >
          {selected ? (
            <time
              dateTime={selected.at}
              title={selected.at}
              className="tabular-nums text-muted-foreground"
            >
              {utcStamp(selected.at, history.stepSeconds < 60)}
            </time>
          ) : (
            <span className="text-muted-foreground">No selected interval</span>
          )}
          {selected ? (
            <dl className="flex flex-wrap gap-x-5 gap-y-2">
              {lines.map((line) => (
                <div
                  key={line.name}
                  className="flex min-w-0 flex-wrap items-baseline gap-x-2"
                >
                  <dt className="text-muted-foreground">{line.name}</dt>
                  <dd className="break-words font-medium tabular-nums">
                    {valueText(line, selected)}
                  </dd>
                </div>
              ))}
            </dl>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
const compactTime = new Intl.DateTimeFormat("en-GB", {
  timeZone: "UTC",
  day: "numeric",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});
const preciseTime = new Intl.DateTimeFormat("en-GB", {
  timeZone: "UTC",
  day: "numeric",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});
function utcStamp(iso: string, seconds = false) {
  return (seconds ? preciseTime : compactTime).format(new Date(iso)) + " UTC";
}
