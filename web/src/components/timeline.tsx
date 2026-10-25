import { useId, useState } from "react";
import type { HistoryPoint, SLOHistory } from "@/lib/history";
import { nearestPointIndex } from "@/lib/chart-range";
import { useChartSize } from "@/lib/use-chart-size";
import { ReplayLabel } from "@/components/replay-label";
import { Card, CardHeader, CardContent } from "@/components/ui/card";
interface Line {
  name: string;
  color: string;
  get: (p: HistoryPoint) => number | null;
  dashed?: boolean;
}
export const sliTimelineLines: Line[] = [
  {
    name: "Interval SLI",
    color: "var(--chart-interval)",
    get: (p) => (p.intervalCoverage === 1 ? p.intervalReliability : null),
  },
  {
    name: "Rolling SLI",
    color: "var(--chart-sli)",
    get: (p) => (p.coverage === 1 ? p.reliability : null),
  },
  {
    name: "Rolling SLI (partial window)",
    color: "var(--chart-partial)",
    dashed: true,
    get: (p) => (p.coverage > 0 && p.coverage < 1 ? p.reliability : null),
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
  const [keyboardInspection, setKeyboardInspection] = useState(false);
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
  const left = width < 400 ? 64 : 74,
    right = Math.max(left + 1, width - 12),
    top = 14,
    bottom = Math.max(top + 1, height - 32);
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
      ? `${iso.slice(5, 10)} ${iso.slice(11, 16)}`
      : iso.slice(11, 16);
  };
  function path(line: Line) {
    let pen = false;
    return history.points
      .map((point, i) => {
        const value = line.get(point);
        if (!finite(value)) {
          pen = false;
          return "";
        }
        const segment = `${pen ? "L" : "M"}${x(i).toFixed(2)},${y(value).toFixed(2)}`;
        pen = true;
        return segment;
      })
      .join(" ");
  }
  const selected = history.points[index];
  const ticks = Math.max(2, Math.min(6, Math.floor((right - left) / 120) + 1));
  function inspect(clientX: number, bounds: DOMRect) {
    setKeyboardInspection(false);
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
        <h3 className="text-sm font-semibold">
          <ReplayLabel>{title}</ReplayLabel>
        </h3>
        <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {lines.map((line) => (
            <span key={line.name} className="inline-flex items-center gap-1.5">
              <span
                className="inline-block w-4"
                style={{
                  borderTop: `2px ${line.dashed ? "dashed" : "solid"} ${line.color}`,
                }}
              />
              <ReplayLabel>{line.name}</ReplayLabel>
            </span>
          ))}
          <span className="inline-flex items-center gap-1.5">
            <span className="w-4 border-t border-dashed border-slate-500" />
            {thresholdLabel}
          </span>
        </div>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-col">
        <div
          ref={ref}
          className="relative min-h-0 w-full flex-1"
          data-chart-host
        >
          <svg
            viewBox={`0 0 ${width} ${height}`}
            className="block size-full touch-pan-y outline-none focus-visible:ring-2 focus-visible:ring-ring"
            role="img"
            aria-labelledby={`${id}-title`}
            aria-describedby={`${id}-description`}
            tabIndex={0}
            onKeyDown={(event) => {
              if (!history.points.length) return;
              if (
                ["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)
              ) {
                event.preventDefault();
                setKeyboardInspection(true);
                onIndex(
                  event.key === "Home"
                    ? 0
                    : event.key === "End"
                      ? history.points.length - 1
                      : Math.max(
                          0,
                          Math.min(
                            history.points.length - 1,
                            index + (event.key === "ArrowRight" ? 1 : -1),
                          ),
                        ),
                );
              }
            }}
            onPointerMove={(event) =>
              inspect(
                event.clientX,
                event.currentTarget.getBoundingClientRect(),
              )
            }
            onPointerDown={(event) => {
              event.currentTarget.focus();
              inspect(
                event.clientX,
                event.currentTarget.getBoundingClientRect(),
              );
            }}
          >
            <title id={`${id}-title`}>{title}</title>
            <desc id={`${id}-description`}>
              Use pointer, touch or Left/Right, Home and End keys to inspect
              shared time points. Time axis is UTC. Gaps mean no eligible
              measurements.
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
                    fontSize={11}
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
                strokeDasharray="5 4"
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
                            event.kind === "firing" ? "#be123c" : "#15803d"
                          }
                          strokeDasharray="2 4"
                          opacity={0.55}
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
                    strokeWidth={1.8}
                    strokeDasharray={line.dashed ? "4 3" : undefined}
                    vectorEffect="non-scaling-stroke"
                  />
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
                    point.belowTarget &&
                    finite(point.intervalReliability) &&
                    point.intervalCoverage === 1 ? (
                      <circle
                        key={point.at}
                        cx={x(i)}
                        cy={y(point.intervalReliability)}
                        r={2.3}
                        fill="#be123c"
                      />
                    ) : null,
                  )
                : null}
              {selected ? (
                <g>
                  <line
                    x1={x(index)}
                    x2={x(index)}
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
                        cx={x(index)}
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
                  fontSize={11}
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
            <p className="pointer-events-none absolute inset-x-0 top-1/2 text-center text-xs text-muted-foreground">
              No measurements available in this range
            </p>
          ) : null}
        </div>
        <div
          className="flex h-14 shrink-0 flex-wrap content-center items-center gap-x-3 gap-y-1 overflow-y-auto border-t pt-2 text-[11px] text-muted-foreground"
          aria-live={keyboardInspection ? "polite" : "off"}
        >
          <span className="font-mono">
            {selected ? utcStamp(selected.at) : "No selected interval"}
          </span>
          {selected
            ? lines.map((line) => {
                const value = line.get(selected);
                return (
                  <span key={line.name}>
                    {line.name}:{" "}
                    <span className="font-mono text-foreground">
                      {finite(value) ? inspectValue(value) : "No data"}
                    </span>
                  </span>
                );
              })
            : null}
        </div>
      </CardContent>
    </Card>
  );
}
function utcStamp(iso: string) {
  return new Date(iso).toISOString().slice(0, 19).replace("T", " ") + " UTC";
}
