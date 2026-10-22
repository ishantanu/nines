import type { SLO } from "./api";
export interface HistoryPoint {
  shortCoverage: number;
  longCoverage: number;
  at: string;
  intervalReliability: number | null;
  reliability: number | null;
  budgetRemainingFraction: number | null;
  burnRate: number | null;
  shortBurnRate: number | null;
  longBurnRate: number | null;
  coverage: number;
  intervalCoverage: number;
  belowTarget: boolean;
  targetViolated: boolean;
  burnThresholdBreached: boolean;
}
export interface SLOHistory {
  measurementMode?: "interval_counts" | "provider_window";
  coverageBasis?: "interval_presence" | "scrape_density";
  source: string;
  sloId: string;
  configRevision: number;
  target: number;
  windowSeconds: number;
  stepSeconds: number;
  resolutionSeconds: number;
  from: string;
  to: string;
  policy?: SLO["policy"];
  policies?: SLO["policies"];
  points: HistoryPoint[];
  violations: { kind: string; from: string; to: string; buckets: number }[];
  alertEvents: {
    id: string;
    kind: string;
    time: string;
    reason?: string;
    policy?: string;
    severity?: string;
    shortBurn: number;
    longBurn: number;
  }[];
  alertEventsBounded: boolean;
}
export function duration(seconds: number): string {
  if (seconds === 0) return "0s";
  if (seconds % 86400 === 0) return `${seconds / 86400}d`;
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}
export function durationSeconds(text: string): number {
  let total = 0,
    used = "";
  for (const m of text.matchAll(/(\d+)(w|d|h|m|s)/g)) {
    total +=
      Number(m[1]) * { w: 604800, d: 86400, h: 3600, m: 60, s: 1 }[m[2] as "s"];
    used += m[0];
  }
  if (
    !used ||
    used !== text ||
    !Number.isSafeInteger(total) ||
    total > 365 * 86400
  )
    throw new Error("Use whole durations such as 28d, 1h30m or 60s.");
  return total;
}
