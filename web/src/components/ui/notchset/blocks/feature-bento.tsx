// Beautiful UI Notchset Feature Bento v1.0.0, MIT (c) 2026 Chimpy.
// Source: https://beautiful-ui.dev/r/radix-nova/feature-bento.json
// Nines changes: pause/visibility controls, documentation destinations, generic
// value formatting, accessible sample chart, and source-backed illustration copy.
// Full MIT notice: /beautiful-ui-license.txt.
"use client";

import * as React from "react";
import { cn } from "cn";
import { buttonVariants } from "@/lib/beautiful-ui/notchset/button-variants";
import {
  NOTCHSET_ROOT,
  OWN_SOUND,
  RollingNumber,
  motionOff,
  playCue,
} from "@/lib/beautiful-ui/notchset/showcase-instrument";

/*
 * Notchset Feature Bento (a block, one file for both builds). Four features, each shown working: two people
 * reviewing a trace (cursors glide, a line is selected, a comment types itself and posts); a command
 * palette that types its own queries until you click into it, then filters, moves and runs for real; a
 * cost tile whose weekly bars roll the number when you point at or focus them; and an automation chain
 * that lights step by step, with REPLAY. One 250ms tick drives it all, and it stops when the tab is
 * hidden, the block is off screen or motion is reduced (everything then shows its finished state).
 */

export type BentoCommand = {
  glyph: string;
  label: string;
  kind: string;
  result: string;
  href?: string;
};
export type BentoTile = { title: string; body: string };

export type FeatureBentoProps = Omit<
  React.ComponentProps<"section">,
  "children" | "title"
> & {
  eyebrow?: string;
  title: React.ReactNode;
  description?: React.ReactNode;
  review: BentoTile & {
    trace: string;
    lines: readonly { text: string; time: string; signal?: boolean }[];
    people: readonly [string, string];
    initials: readonly [string, string];
    note: string;
  };
  palette: BentoTile & {
    commands: readonly BentoCommand[];
    autoQueries?: readonly string[];
  };
  cost: BentoTile & {
    label: string;
    weeks: readonly { label: string; value: number }[];
    marker?: { index: number; label: string };
    baseline?: number;
    formatValue?: (value: number) => string;
    comparisonLabel?: string;
  };
  automation: BentoTile & {
    label: string;
    steps: readonly { kind: string; text: string }[];
    footnote?: string;
  };
};

const small = "font-mono text-[12px] tracking-[0.04em] whitespace-nowrap";
// Subsequence match: "mod" finds "Change model…".
const subsequence = (q: string, s: string) => {
  let i = 0;
  for (const ch of s) if (ch === q[i]) i++;
  return i === q.length;
};
// Cursor positions per phase (x in %, y in px within the trace card): Priya selects line 3, Maya reads.
const PRIYA = [
  [30, 58],
  [52, 104],
  [62, 104],
  [24, 150],
] as const;
const MAYA = [
  [78, 150],
  [70, 70],
  [44, 104],
  [70, 104],
] as const;

/** The tick: counts every 250ms while the block is on screen, visible and allowed to move. */
function useTick(root: React.RefObject<HTMLElement | null>, paused: boolean) {
  const [t, setT] = React.useState(12);
  const [still, setStill] = React.useState(true);
  React.useEffect(() => {
    const el = root.current;
    if (!el) return;
    let seen = typeof IntersectionObserver !== "function";
    let timer: ReturnType<typeof setInterval> | undefined;
    const media = window.matchMedia?.("(prefers-reduced-motion: reduce)");
    const sync = () => {
      clearInterval(timer);
      const moving = seen && !document.hidden && !paused && !motionOff(el);
      setStill(!moving);
      if (moving) timer = setInterval(() => setT((n) => n + 1), 250);
    };
    const io =
      typeof IntersectionObserver === "function"
        ? new IntersectionObserver(([entry]) => {
            seen = !!entry?.isIntersecting;
            sync();
          })
        : null;
    io?.observe(el);
    document.addEventListener("visibilitychange", sync);
    media?.addEventListener("change", sync);
    sync();
    return () => {
      clearInterval(timer);
      io?.disconnect();
      document.removeEventListener("visibilitychange", sync);
      media?.removeEventListener("change", sync);
    };
  }, [root, paused]);
  return { t, still };
}

function Tile({
  tile,
  className,
  children,
  onEnter,
  onLeave,
}: {
  tile: BentoTile;
  className?: string;
  children: React.ReactNode;
  onEnter?: () => void;
  onLeave?: () => void;
}) {
  return (
    <div
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      className={cn(
        "flex min-w-0 flex-col gap-3.5 overflow-hidden p-5 transition-colors duration-[160ms] ease-linear hover:bg-[var(--notchset-field,var(--muted))]",
        className,
      )}
    >
      {children}
      <div className="flex flex-col gap-1">
        <h3 className="m-0 text-[16px] font-semibold">{tile.title}</h3>
        <p className="m-0 text-[14px] leading-[1.5] text-muted-foreground">
          {tile.body}
        </p>
      </div>
    </div>
  );
}

function Review({
  review,
  t,
  still,
}: {
  review: FeatureBentoProps["review"];
  t: number;
  still: boolean;
}) {
  const phase = still ? 2 : Math.floor(t / 7) % 4;
  const selected = phase === 1 || phase === 2;
  const cycle = t % 28;
  const typed = still
    ? review.note.length
    : Math.max(0, Math.min(review.note.length, (cycle - 6) * 3));
  const posted = still || cycle >= 18;
  const showNote = still || cycle >= 5;
  const cursor = (
    p: readonly (readonly [number, number])[],
    name: string,
    ink: boolean,
  ) => (
    <div
      aria-hidden
      data-slot="review-cursor"
      className="pointer-events-none absolute z-[3] transition-[left,top] duration-[900ms] ease-(--notchset-ease-glide)"
      style={{ left: `${p[phase]![0]}%`, top: p[phase]![1] }}
    >
      <svg width="12" height="15" viewBox="0 0 12 15">
        <path
          d="M1 1 L1 12.5 L4 9.6 L6.4 14 L8.4 13 L6.1 8.7 L10.5 8.4 Z"
          strokeWidth="1"
          className={
            ink
              ? "fill-foreground stroke-background"
              : "fill-background stroke-foreground"
          }
        />
      </svg>
      <span
        className={cn(
          small,
          "absolute top-3 left-[11px] flex h-4 items-center border border-solid border-foreground px-[5px] font-medium tracking-[0.08em]",
          ink
            ? "bg-foreground text-background"
            : "bg-background text-foreground",
        )}
      >
        {name}
      </span>
    </div>
  );
  return (
    <div className="relative box-border min-h-[200px] flex-1 border border-solid border-foreground bg-background px-3.5 py-3">
      <div
        className={cn(
          small,
          "mb-3 flex items-center justify-between tracking-[0.12em] text-muted-foreground",
        )}
      >
        <span>{review.trace}</span>
        <span aria-hidden className="flex">
          <span className="flex size-5 items-center justify-center rounded-full border-[1.5px] border-solid border-background bg-foreground font-mono text-[10px] text-background">
            {review.initials[0]}
          </span>
          <span className="-ms-1.5 box-border flex size-5 items-center justify-center rounded-full border-[1.5px] border-solid border-foreground bg-background font-mono text-[10px] text-foreground">
            {review.initials[1]}
          </span>
        </span>
      </div>
      {review.lines.map((l, i) => (
        <div
          key={l.text}
          className="relative grid h-[30px] grid-cols-[22px_minmax(0,1fr)_64px] items-center gap-2.5 font-mono text-[12px] whitespace-nowrap"
        >
          <span
            aria-hidden
            className={cn(
              "absolute inset-y-1 start-7 end-[72px] origin-left bg-[var(--notchset-field,var(--muted))] transition-[scale] duration-[360ms] ease-(--notchset-ease-unfold)",
              i === 2 && selected ? "scale-x-100" : "scale-x-0",
            )}
          />
          <span className="relative text-muted-foreground">
            {String(i + 1).padStart(2, "0")}
          </span>
          <span
            className={cn(
              "relative truncate",
              l.signal &&
                "text-[var(--notchset-signal-text,var(--destructive))]",
            )}
          >
            {l.text}
          </span>
          <span
            className={cn(
              "relative text-end",
              l.signal
                ? "text-[var(--notchset-signal-text,var(--destructive))]"
                : "text-muted-foreground",
            )}
          >
            {l.time}
          </span>
        </div>
      ))}
      <div
        className={cn(
          "absolute end-3.5 bottom-3 box-border flex w-[236px] max-w-[calc(100%-28px)] flex-col gap-1 border border-solid bg-background px-2.5 py-2 transition-[opacity,translate] duration-[360ms] ease-(--notchset-ease-settle)",
          posted
            ? "border-foreground"
            : "border-[var(--notchset-control-edge,var(--input))]",
          showNote ? "translate-y-0 opacity-100" : "translate-y-1.5 opacity-0",
        )}
      >
        <span className={cn(small, "font-medium")}>
          {review.people[1].toUpperCase()} · {posted ? "POSTED" : "TYPING"}
        </span>
        <span className="min-h-[18px] text-[13px] leading-[1.4]">
          <span className="sr-only">{review.note}</span>
          <span aria-hidden>{review.note.slice(0, typed)}</span>
          {!posted && (
            <span
              aria-hidden
              className="ms-px inline-block h-[13px] w-px bg-foreground align-[-2px] motion-safe:animate-[notchset-blink_1s_steps(1)_infinite]"
            />
          )}
        </span>
      </div>
      {!still && cursor(PRIYA, review.people[0].toUpperCase(), true)}
      {!still && cursor(MAYA, review.people[1].toUpperCase(), false)}
    </div>
  );
}

function Palette({
  palette,
  t,
  still,
}: {
  palette: FeatureBentoProps["palette"];
  t: number;
  still: boolean;
}) {
  const [manual, setManual] = React.useState(false);
  const [query, setQuery] = React.useState("");
  const [sel, setSel] = React.useState(0);
  const [toast, setToast] = React.useState("");
  const toastTimer = React.useRef<ReturnType<typeof setTimeout>>(undefined);
  React.useEffect(() => () => clearTimeout(toastTimer.current), []);
  const listId = React.useId();
  const q = query;
  const list = palette.commands.filter(
    (c) => !q || subsequence(q.toLowerCase(), c.label.toLowerCase()),
  );
  const index = Math.min(manual ? sel : 0, Math.max(0, list.length - 1));
  const shown = toast;
  const run = (c: BentoCommand, el: Element | null) => {
    playCue(el, "done");
    if (c.href) {
      window.location.assign(c.href);
      return;
    }
    setToast(c.result);
    clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(""), 1600);
  };
  return (
    <div className="flex flex-1 flex-col justify-center">
      <div className="border border-solid border-foreground bg-background">
        <label className="flex h-11 items-center gap-2.5 border-0 border-b border-solid border-[var(--notchset-rule,var(--border))] px-3">
          <svg
            aria-hidden
            width="12"
            height="12"
            viewBox="0 0 13 13"
            className="flex-none"
          >
            <circle
              cx="5.5"
              cy="5.5"
              r="4.2"
              fill="none"
              className="stroke-muted-foreground"
              strokeWidth="1.2"
            />
            <path
              d="M8.6 8.6 L12 12"
              className="stroke-muted-foreground"
              strokeWidth="1.2"
            />
          </svg>
          <input
            role="combobox"
            aria-expanded
            aria-controls={listId}
            aria-activedescendant={
              list.length ? `${listId}-${index}` : undefined
            }
            aria-label="Find a workflow guide"
            value={q}
            onFocus={() => {
              if (!manual) {
                setManual(true);
                setQuery("");
                setSel(0);
              }
            }}
            onBlur={() => {
              if (!query) setManual(false);
            }}
            onChange={(e) => {
              setManual(true);
              setQuery(e.target.value);
              setSel(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault();
                if (!list.length) return;
                playCue(e.currentTarget, "tick");
                setSel(
                  (index + (e.key === "ArrowDown" ? 1 : -1) + list.length) %
                    list.length,
                );
              } else if (e.key === "Enter") {
                e.preventDefault();
                const c = list[index];
                if (c) run(c, e.currentTarget);
                else playCue(e.currentTarget, "signal");
              } else if (e.key === "Escape") {
                setQuery("");
                setSel(0);
              }
            }}
            placeholder="Find a guide"
            spellCheck={false}
            className="min-w-0 flex-1 border-0 bg-transparent font-mono text-[12px] text-foreground outline-none placeholder:text-muted-foreground"
          />
        </label>
        <div
          id={listId}
          role="listbox"
          aria-label="Documentation guides"
          className="relative h-[144px] py-1"
        >
          <span
            aria-hidden
            className={cn(
              "absolute inset-x-0 h-11 border-0 border-s border-solid border-foreground bg-[var(--notchset-field,var(--muted))] transition-[top] duration-[160ms] ease-(--notchset-ease-glide)",
              list.length ? "opacity-100" : "opacity-0",
            )}
            style={{ top: 4 + index * 44 }}
          />
          {list.map((c, i) => (
            <a
              href={c.href}
              key={c.label}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === index}
              onMouseEnter={() => manual && setSel(i)}
              onMouseDown={(e) => {
                if (c.href) {
                  setManual(true);
                  setSel(i);
                  return;
                }
                e.preventDefault();
                setManual(true);
                setSel(i);
                run(c, e.currentTarget);
              }}
              className="relative grid h-11 cursor-pointer grid-cols-[22px_minmax(0,1fr)_auto] items-center gap-2.5 px-3"
            >
              <span
                className={cn(
                  "box-border flex size-5 items-center justify-center border border-solid font-mono text-[10px] font-medium",
                  i === index
                    ? "border-foreground bg-foreground text-background"
                    : "border-[var(--notchset-rule,var(--border))] text-muted-foreground",
                )}
              >
                {c.glyph}
              </span>
              <span className="truncate text-[13px]">{c.label}</span>
              <span className={cn(small, "text-muted-foreground")}>
                {c.kind}
              </span>
            </a>
          ))}
          {!list.length && (
            <span
              className={cn(
                small,
                "absolute start-3 top-3.5 tracking-[0.06em] whitespace-normal text-muted-foreground",
              )}
            >
              No matching guide. Try “CLI”, “MCP” or “intent”.
            </span>
          )}
        </div>
        <div
          className={cn(
            small,
            "flex h-9 items-center justify-between gap-2 border-0 border-t border-solid border-[var(--notchset-rule,var(--border))] px-3 tracking-[0.08em] text-muted-foreground",
          )}
        >
          <span className="@max-[360px]:hidden">
            Arrow keys to browse · Enter to open
          </span>
          <span
            aria-live={manual ? "polite" : "off"}
            className={cn(
              "truncate text-foreground transition-opacity duration-200 ease-linear",
              shown ? "opacity-100" : "opacity-0",
            )}
          >
            {shown}
          </span>
        </div>
      </div>
    </div>
  );
}

function Cost({ cost }: { cost: FeatureBentoProps["cost"] }) {
  const [hover, setHover] = React.useState(-1);
  const formatValue =
    cost.formatValue ?? ((value: number) => `$${value.toFixed(2)}`);
  const last = cost.weeks.length - 1;
  const at = hover >= 0 ? hover : last;
  const week = cost.weeks[at]!;
  const top = Math.max(...cost.weeks.map((w) => w.value)) * 1.05;
  const base = cost.baseline ?? cost.weeks[0]?.value ?? 1;
  const after = cost.marker && at >= cost.marker.index + 1;
  return (
    <div
      className="flex flex-1 flex-col gap-3"
      onMouseLeave={(event) => {
        if (!event.currentTarget.contains(document.activeElement)) setHover(-1);
      }}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setHover(-1);
      }}
    >
      <div className="flex flex-col gap-1">
        <span className={cn(small, "tracking-[0.12em] text-muted-foreground")}>
          {cost.label} · {week.label}
        </span>
        <span className="flex items-baseline gap-2">
          <RollingNumber
            value={formatValue(week.value)}
            height={34}
            className="font-mono text-[30px] font-light tracking-[-0.04em]"
          />
          <span
            className={cn(
              small,
              "flex h-5 items-center bg-foreground px-1.5 font-medium tracking-[0.06em] text-background",
            )}
          >
            {cost.comparisonLabel ??
              (after
                ? `−${Math.round((1 - week.value / base) * 100)}% VS ${cost.weeks[0]?.label.split(" ")[0]}`
                : "BEFORE THE SWITCH")}
          </span>
        </span>
      </div>
      <div
        className="relative grid min-h-[90px] flex-1 grid-cols-[repeat(var(--n),minmax(0,1fr))] items-end gap-1.5 border-0 border-b border-solid border-foreground"
        style={{ "--n": cost.weeks.length } as React.CSSProperties}
      >
        {cost.weeks.map((w, i) => (
          <div
            key={w.label}
            tabIndex={0}
            role="button"
            aria-pressed={i === at}
            onClick={() => setHover(i)}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                setHover(i);
              }
            }}
            aria-label={`${w.label}: ${formatValue(w.value)}`}
            onMouseEnter={() => setHover(i)}
            onFocus={() => setHover(i)}
            className={cn(
              `notchset-focus`,
              "relative flex h-full items-end [--notchset-focus-inset:-3px]",
            )}
          >
            <span
              className={cn(
                "block w-full transition-colors duration-[140ms] ease-linear",
                i === at
                  ? "bg-foreground"
                  : cost.marker && i > cost.marker.index
                    ? "bg-muted-foreground"
                    : "bg-[var(--notchset-control-edge,var(--input))]",
              )}
              style={{ height: `${(w.value / top) * 100}%` }}
            />
            {cost.marker?.index === i && (
              <span
                aria-hidden
                className="absolute inset-y-0 -end-1 border-0 border-e border-dashed border-[var(--notchset-signal,var(--destructive))]"
              />
            )}
          </div>
        ))}
      </div>
      <div
        aria-hidden
        className={cn(
          small,
          "flex justify-between gap-2 text-muted-foreground",
        )}
      >
        <span>{cost.weeks[0]?.label}</span>
        {cost.marker && (
          <span className="truncate text-[var(--notchset-signal-text,var(--destructive))]">
            {cost.marker.label}
          </span>
        )}
        <span>{cost.weeks[last]?.label}</span>
      </div>
    </div>
  );
}

function Automation({
  automation,
  t,
  still,
}: {
  automation: FeatureBentoProps["automation"];
  t: number;
  still: boolean;
}) {
  const [start, setStart] = React.useState(0);
  const n = automation.steps.length;
  const period = (n + 5) * 3;
  const lit = still
    ? n
    : Math.min(n, Math.floor(((((t - start) % period) + period) % period) / 3));
  return (
    <div className="flex flex-1 flex-col gap-3">
      <div
        className={cn(
          small,
          "flex items-center justify-between tracking-[0.12em] text-muted-foreground",
        )}
      >
        <span className="truncate">{automation.label}</span>
        <button
          {...OWN_SOUND}
          type="button"
          onClick={(e) => {
            playCue(e.currentTarget, "tick");
            setStart(t);
          }}
          className={cn(
            `notchset-focus`,
            buttonVariants({ variant: "outline", size: "xs" }),
            "min-h-11 flex-none [--notchset-focus-inset:-3px]",
          )}
        >
          REPLAY
        </button>
      </div>
      <ol className="m-0 flex flex-1 list-none flex-col justify-center p-0">
        {automation.steps.map((s, i) => (
          <li
            key={s.text}
            className={cn(
              "relative grid grid-cols-[24px_minmax(0,1fr)] items-start gap-3",
              i < n - 1 && "pb-[22px]",
            )}
          >
            {i < n - 1 && (
              <>
                <span
                  aria-hidden
                  className="absolute start-[11.5px] top-6 bottom-0 w-px bg-[var(--notchset-rule,var(--border))]"
                />
                <span
                  aria-hidden
                  className={cn(
                    "absolute start-[11px] top-6 bottom-0 w-0.5 origin-top bg-foreground transition-[scale] duration-[420ms] ease-(--notchset-ease-fold)",
                    i < lit - 1 || (i < lit && lit === n)
                      ? "scale-y-100"
                      : "scale-y-0",
                  )}
                />
              </>
            )}
            <svg
              aria-hidden
              width="24"
              height="24"
              viewBox="0 0 24 24"
              className="relative"
            >
              <circle
                cx="12"
                cy="12"
                r="10.5"
                strokeWidth="1.3"
                className={cn(
                  "transition-[fill,stroke] duration-[160ms] ease-linear",
                  i < lit ? "fill-foreground" : "fill-background",
                  i <= lit
                    ? "stroke-foreground"
                    : "stroke-[var(--notchset-rule,var(--border))]",
                )}
              />
              <path
                d="M7.5 12.2 L10.6 15.2 L16.5 9"
                fill="none"
                strokeWidth="1.6"
                pathLength={1}
                strokeDasharray={1}
                className="stroke-background transition-[stroke-dashoffset] delay-[80ms] duration-[260ms] ease-(--notchset-ease-check)"
                style={{ strokeDashoffset: i < lit ? 0 : 1 }}
              />
            </svg>
            <span className="flex min-w-0 flex-col gap-[3px] pt-0.5">
              <span
                className={cn(
                  small,
                  "font-medium tracking-[0.12em] text-muted-foreground",
                )}
              >
                {s.kind}
              </span>
              <span className="text-[13px] font-medium">{s.text}</span>
            </span>
          </li>
        ))}
      </ol>
      {automation.footnote && (
        <span
          className={cn(
            small,
            "whitespace-normal leading-relaxed tracking-[0.04em] text-muted-foreground",
          )}
        >
          {automation.footnote}
        </span>
      )}
    </div>
  );
}

function FeatureBento({
  eyebrow,
  title,
  description,
  review,
  palette,
  cost,
  automation,
  className,
  ...props
}: FeatureBentoProps) {
  const root = React.useRef<HTMLElement>(null);
  const [paused, setPaused] = React.useState(false);
  const { t, still } = useTick(root, paused);
  return (
    <section
      {...NOTCHSET_ROOT}
      ref={root}
      data-slot="feature-bento"
      data-still={still}
      data-paused={paused}
      aria-label={typeof title === "string" ? title : "Features"}
      className={cn(
        "@container box-border border border-solid border-foreground bg-background text-foreground",
        className,
      )}
      {...props}
    >
      <div className="bento-heading flex max-w-[600px] flex-col gap-3 px-7 pt-7 pb-[22px] @max-[640px]:px-5">
        {eyebrow && <span className="sr-only">{eyebrow}</span>}
        <h2 className="m-0 text-[clamp(32px,8cqi,46px)] leading-[0.9] font-semibold">
          {title}
        </h2>
        {description && (
          <p className="m-0 text-[15px] leading-[1.5] text-muted-foreground [text-wrap:pretty]">
            {description}
          </p>
        )}
        <button
          type="button"
          aria-pressed={paused}
          onClick={() => setPaused((value) => !value)}
          className="bento-motion-control notchset-focus self-start border border-foreground px-3 py-2 text-sm"
        >
          {paused ? "Play illustrations" : "Pause illustrations"}
        </button>
      </div>
      {/* Three columns and two 300px rows; one column, tiles at their own height, under 760px. */}
      <div className="grid grid-cols-3 grid-rows-[minmax(260px,auto)_minmax(260px,auto)] border-0 border-t border-solid border-foreground @max-[760px]:grid-cols-1 @max-[760px]:grid-rows-none">
        <Tile
          tile={review}
          className="col-span-2 row-start-1 border-0 border-e border-b border-solid border-[var(--notchset-rule,var(--border))] @max-[760px]:col-span-1 @max-[760px]:row-start-auto @max-[760px]:border-e-0"
        >
          <Review review={review} t={t} still={still} />
        </Tile>
        <Tile
          tile={palette}
          className="col-start-3 row-span-2 row-start-1 border-0 border-b border-solid border-[var(--notchset-rule,var(--border))] @max-[760px]:col-start-auto @max-[760px]:row-span-1 @max-[760px]:row-start-auto"
        >
          <Palette palette={palette} t={t} still={still} />
        </Tile>
        <Tile
          tile={cost}
          className="col-start-1 row-start-2 border-0 border-e border-solid border-[var(--notchset-rule,var(--border))] @max-[760px]:col-start-auto @max-[760px]:row-start-auto @max-[760px]:min-h-[300px] @max-[760px]:border-e-0 @max-[760px]:border-b"
        >
          <Cost cost={cost} />
        </Tile>
        <Tile
          tile={automation}
          className="col-start-2 row-start-2 border-0 border-e border-solid border-[var(--notchset-rule,var(--border))] @max-[760px]:col-start-auto @max-[760px]:row-start-auto @max-[760px]:border-e-0"
        >
          <Automation automation={automation} t={t} still={still} />
        </Tile>
      </div>
    </section>
  );
}

export { FeatureBento };
