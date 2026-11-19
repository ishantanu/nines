"use client";

/**
 * Donut Chart (Notchset) v1.0.0 · Beautiful UI
 * Docs: https://beautiful-ui.dev/components/donut-chart
 * MIT licensed: free to use, change and share.
 * Nines adaptation: visible without mount animation, keyboard inspection,
 * bounded labels and precise shares. See /beautiful-ui-license.txt.
 */

import * as React from "react";
import { cn } from "@/lib/utils";
import { NOTCHSET_ROOT, GLIDE } from "@/lib/beautiful-ui/notchset/instrument";
import { RollingLabel } from "@/lib/beautiful-ui/notchset/instrument";

/*
 * Notchset Donut Chart (hand-built SVG, one file for both builds). A segmented ring
 * (r80, 18px, butt caps, 0.9-unit gaps) on a ruler of 40 ticks, in ink tones and patterns that each
 * reach 3:1: ink, muted, edge, 45° hatch, dots, −45° muted hatch. Shares use the largest remainder,
 * so they total 100%. Segments sweep in, and morph when the data changes (760ms, 50ms apart).
 * Selection leaves geometry and the total stable; a separate readout describes the category.
 */

const GAP = 0.9;

/** Whole percentages that total exactly 100 (largest remainder). */
export function shares(values: readonly number[]) {
  const sum = values.reduce((a, b) => a + b, 0);
  if (sum === 0) return { raw: values.map(() => 0), pct: values.map(() => 0) };
  const raw = values.map((v) => (v / sum) * 100);
  const out = raw.map(Math.floor);
  let rem = 100 - out.reduce((a, b) => a + b, 0);
  raw
    .map((v, i) => [v - Math.floor(v), i] as const)
    .sort((a, b) => b[0] - a[0])
    .forEach(([, i]) => {
      if (rem > 0) {
        out[i]!++;
        rem--;
      }
    });
  return { raw, pct: out };
}

function DonutChart({
  data,
  unit = "",
  digits = 1,
  label = "ALL",
  caption = "TOTAL",
  className,
  "aria-label": ariaLabel = "Donut chart",
}: {
  data: readonly { label: string; value: number }[];
  /** After the centre value (M, K). */
  unit?: string;
  digits?: number;
  /** The centre's label at rest. */
  label?: string;
  /** The centre's caption at rest; hovering shows the share. */
  caption?: string;
  className?: string;
  "aria-label"?: string;
}) {
  const root = React.useRef<HTMLDivElement>(null);
  const [hoverAt, setHover] = React.useState<number | null>(null);
  // A hovered segment that the data no longer has reads as no hover.
  const hover = hoverAt != null && hoverAt < data.length ? hoverAt : null;
  const places = Number.isInteger(digits)
    ? Math.min(6, Math.max(0, digits))
    : 1;
  const pre = false;
  const values = data.map((d) => d.value);
  const sum = values.reduce((a, b) => a + b, 0);
  const { raw, pct } = shares(values);
  const fills = [
    "var(--color-data-categorical-green)",
    "var(--color-data-categorical-orange)",
    "var(--color-data-categorical-purple)",
    "var(--color-text-secondary)",
    "var(--color-text-primary)",
    "var(--color-border-emphasized)",
  ];
  // A zero is drawn as nothing (never as a sliver of share it doesn't have).
  const segs = raw.map((r, i) => ({
    st: raw.slice(0, i).reduce((a, b) => a + b, 0),
    len: r > 0 ? r - Math.min(GAP, r * 0.1) : 0,
  }));

  return (
    <div
      {...NOTCHSET_ROOT}
      ref={root}
      data-slot="donut-chart"
      className={cn("text-foreground", className)}
      onMouseLeave={() => setHover(null)}
    >
      <div className="flex justify-center pt-[18px] pb-2">
        <div className="relative size-[180px] sm:size-[220px]">
          <svg
            role="img"
            aria-label={`${ariaLabel}: ${data.map((d, i) => `${d.label} ${pct[i]}%`).join(", ")}`}
            viewBox="0 0 220 220"
            width="220"
            height="220"
            className="block size-full overflow-visible"
          >
            <circle
              cx="110"
              cy="110"
              r="80"
              fill="none"
              strokeWidth="18"
              className="stroke-[var(--color-background-muted)]"
            />
            <g transform="rotate(-90 110 110)">
              {segs.map((g, i) => (
                <circle
                  key={i}
                  cx="110"
                  cy="110"
                  r="80"
                  fill="none"
                  pathLength={100}
                  strokeLinecap="butt"
                  stroke={fills[i % fills.length]}
                  onMouseEnter={() => setHover(i)}
                  style={{
                    strokeDasharray: pre
                      ? "0 100"
                      : `${g.len.toFixed(2)} ${(100 - g.len).toFixed(2)}`,
                    strokeDashoffset: pre ? 0 : -g.st - GAP / 2,
                    strokeWidth: 18,
                    opacity: 1,
                    transition: pre
                      ? "none"
                      : `stroke-dasharray 760ms ${GLIDE} ${i * 50}ms, stroke-dashoffset 760ms ${GLIDE} ${i * 50}ms`,
                  }}
                />
              ))}
            </g>
          </svg>
          <div
            aria-hidden
            data-slot="donut-total"
            className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center"
          >
            <span className="max-w-[100px] break-words text-xs font-medium leading-tight sm:max-w-[120px]">
              {label}
            </span>
            <span className="mt-1 flex items-baseline gap-0.5">
              <RollingLabel
                text={sum.toFixed(places)}
                height={34}
                className="text-[30px] font-semibold tracking-[-0.02em]"
              />
              {unit && (
                <span className="font-mono text-[13px] text-muted-foreground">
                  {unit}
                </span>
              )}
            </span>
            <span className="max-w-[100px] break-words text-xs leading-tight text-muted-foreground sm:max-w-[120px]">
              {caption}
            </span>
          </div>
        </div>
      </div>
      <p
        data-slot="donut-selection"
        className="flex min-h-14 items-center justify-center px-4 text-center text-sm leading-5"
        aria-live="polite"
        aria-atomic="true"
      >
        {hover === null
          ? "Hover or focus a category to inspect"
          : `${data[hover]!.label}: ${values[hover]!.toFixed(places)}${unit} · ${pct[hover]}% of ${caption}`}
      </p>
      <div className="px-2 pt-1 pb-2.5">
        {data.map((d, i) => (
          <div
            key={d.label}
            onMouseEnter={() => setHover(i)}
            role="button"
            tabIndex={0}
            aria-label={`${d.label}: ${d.value}, ${pct[i]}%`}
            aria-pressed={hover === i}
            onFocus={() => setHover(i)}
            onBlur={() => setHover(null)}
            onClick={() => setHover(i)}
            onKeyDown={(event) => {
              if (event.key === "Escape") setHover(null);
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                setHover(i);
              }
            }}
            className={cn(
              "grid min-h-[44px] grid-cols-[12px_minmax(0,1fr)_auto_auto] items-center gap-2.5 px-2 transition-[background-color] duration-(--notchset-fade) ease-(--notchset-ease-fade)",
              hover === i && "bg-[var(--notchset-faceplate,var(--muted))]",
            )}
          >
            <svg aria-hidden width="12" height="12" viewBox="0 0 12 12">
              <rect
                width="12"
                height="12"
                rx="3"
                fill={fills[i % fills.length]}
              />
            </svg>
            <span className="break-words font-sans text-[13px]">{d.label}</span>
            <span className="flex justify-end text-muted-foreground">
              <RollingLabel
                text={`${d.value.toFixed(places)}${unit}`}
                height={12}
                delay={i * 60}
                className="text-[11px] font-normal"
              />
            </span>
            <span className="flex justify-end">
              <RollingLabel
                text={`${pct[i]}%`}
                height={12}
                delay={i * 60}
                className="text-[11px]"
              />
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

export { DonutChart };
