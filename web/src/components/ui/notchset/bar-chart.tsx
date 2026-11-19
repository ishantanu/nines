"use client";

/**
 * Adapted from MIT Beautiful UI Notchset v1.0.0 (Chimpy).
 * https://beautiful-ui.dev/components/bar-chart
 * Nines: shared responsive chart geometry, semantic colors and accessible
 * service inspection. See /beautiful-ui-license.txt.
 */
import { useId, useState } from "react";
import { ChartTable } from "@/lib/beautiful-ui/notchset/chart-table";
import { useChartSize } from "@/lib/use-chart-size";

export type BarDatum = { label: string; value: number };
export function BarChart({
  data,
  yMax,
  reference = 1,
  format = (v) => `${v.toFixed(2)}×`,
  active: activeProp,
  onActiveChange,
  "aria-label": ariaLabel = "Budget burn by service",
}: {
  data: readonly BarDatum[];
  yMax: number;
  reference?: number;
  format?: (v: number) => string;
  active?: number | null;
  onActiveChange?: (i: number | null) => void;
  "aria-label"?: string;
}) {
  const { ref, width, height } = useChartSize();
  const id = useId();
  const [own, setOwn] = useState<number | null>(null);
  const active = activeProp !== undefined ? activeProp : own;
  const selected = active !== null ? data[active] : undefined;
  const setActive = (next: number | null) => {
    setOwn(next);
    onActiveChange?.(next);
  };
  const left = width < 400 ? 80 : 92;
  const right = Math.max(left + 1, width - 12);
  const top = 16,
    bottom = Math.max(top + 1, height - 36);
  const max = Math.max(reference, Number.isFinite(yMax) ? yMax : 1, 1);
  const y = (value: number) =>
    bottom - (Math.max(0, Math.min(max, value)) / max) * (bottom - top);
  const slot = (right - left) / Math.max(1, data.length);
  const x = (index: number) => left + slot * (index + 0.5);
  const barWidth = Math.min(64, slot * 0.6);
  const labelLimit = Math.max(3, Math.floor(slot / 7));
  return (
    <div
      data-notchset=""
      data-slot="bar-chart"
      className="flex min-h-0 flex-1 flex-col"
    >
      <div
        ref={ref}
        className="relative min-h-56 w-full flex-1"
        data-chart-host
      >
        <svg
          role="group"
          aria-label={ariaLabel}
          aria-describedby={`${id}-description`}
          viewBox={`0 0 ${width} ${height}`}
          className="absolute inset-0 block size-full select-none"
          onPointerLeave={() => setActive(null)}
        >
          <desc id={`${id}-description`}>
            Highest complete-window budget burn per service. Focus a service to
            inspect its value; press Escape to dismiss. The dotted 1× reference
            is the sustainable budget rate, not an alert threshold.
          </desc>
          {[0, 1, 2, 3, 4].map((i) => {
            const value = (max * i) / 4;
            return (
              <g key={i}>
                <line
                  x1={left}
                  x2={right}
                  y1={y(value)}
                  y2={y(value)}
                  stroke="var(--color-border)"
                />
                <text
                  x={left - 8}
                  y={y(value) + 4}
                  textAnchor="end"
                  fill="var(--muted-foreground)"
                  style={{ fontSize: "var(--font-size-sm, 12px)" }}
                >
                  {value.toFixed(1)}×
                </text>
              </g>
            );
          })}
          <line
            x1={left}
            x2={right}
            y1={y(reference)}
            y2={y(reference)}
            stroke="var(--muted-foreground)"
            strokeDasharray="2 4"
          />
          {data.map((datum, index) => (
            <g key={datum.label}>
              {datum.value > 0 ? (
                <rect
                  x={x(index) - barWidth / 2}
                  y={y(datum.value)}
                  width={barWidth}
                  height={bottom - y(datum.value)}
                  fill="var(--chart-sli)"
                  fillOpacity={active === index ? 0.24 : 0.16}
                  stroke="var(--chart-sli)"
                  strokeWidth={1.5}
                />
              ) : (
                <circle
                  cx={x(index)}
                  cy={bottom}
                  r={3}
                  fill="var(--chart-sli)"
                />
              )}
              <text
                x={x(index)}
                y={bottom + 22}
                textAnchor="middle"
                fill="var(--muted-foreground)"
                style={{ fontSize: "var(--font-size-sm, 12px)" }}
              >
                {datum.label.length > labelLimit
                  ? datum.label.slice(0, labelLimit - 1) + "…"
                  : datum.label}
              </text>
              <rect
                x={left + index * slot}
                y={top}
                width={slot}
                height={bottom - top}
                fill="transparent"
                role="button"
                tabIndex={0}
                aria-label={`${datum.label}: ${format(datum.value)}`}
                aria-describedby={
                  active === index ? `${id}-tooltip` : undefined
                }
                className="rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                onPointerEnter={() => setActive(index)}
                onFocus={() => setActive(index)}
                onBlur={() => setActive(null)}
                onClick={() => setActive(index)}
                onKeyDown={(event) => {
                  if (event.key === "Escape") {
                    event.preventDefault();
                    setActive(null);
                  }
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    setActive(index);
                  }
                }}
              />
            </g>
          ))}
        </svg>
        {selected && active !== null ? (
          <div
            id={`${id}-tooltip`}
            role="tooltip"
            className="chart-tooltip pointer-events-none absolute top-2 z-10 w-60 max-w-[calc(100%-1rem)] rounded-sm border p-3 text-sm shadow-md"
            style={{
              left: Math.max(
                8,
                Math.min(
                  width - 248,
                  x(active) > width / 2 ? x(active) - 252 : x(active) + 12,
                ),
              ),
            }}
          >
            <p className="break-words text-xs text-muted-foreground">
              {selected.label}
            </p>
            <dl className="mt-2">
              <dt className="text-xs text-muted-foreground">
                Highest complete-window burn
              </dt>
              <dd className="mt-0.5 font-semibold tabular-nums">
                {format(selected.value)}
              </dd>
            </dl>
            <p className="mt-2 border-t pt-2 text-xs text-muted-foreground">
              1× sustainable budget rate
            </p>
          </div>
        ) : null}
      </div>
      <ChartTable
        caption={ariaLabel}
        head={["Service", "Burn rate"]}
        rows={data.map((datum) => [datum.label, format(datum.value)])}
      />
    </div>
  );
}
