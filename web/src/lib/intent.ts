import { parse } from "yaml";
import { RESOURCE_API_VERSION } from "./definitions";
import { durationSeconds } from "./history";
import type { SLO, Manifest } from "./api";
import type { BindingManifest } from "./topology";
export interface IntentSpec {
  description: string;
  service: string;
  source: string;
  outcome: string;
  population: Record<string, string>;
  thresholdSeconds?: number;
  target: number;
  windowSeconds: number;
  noDataPolicy: "unknown";
  policies?: import("./api").BurnRatePolicy[];
  policy?: SLO["policy"];
  journey?: {
    name: string;
    step: string;
    expectedRevision: number;
    required: boolean;
  };
}
export interface IntentDocument {
  apiVersion: typeof RESOURCE_API_VERSION;
  kind: "ServiceLevelObjectiveIntent";
  metadata: { name: string; project: string };
  spec: IntentSpec;
}
export interface IntentPlan {
  status: string;
  expectedRevision: number;
  intentDigest: string;
  candidates: {
    binding: { name: string; revision: number; digest: string };
    definition: BindingManifest;
  }[];
  clarifications: string[];
  truncated: boolean;
  compiled?: {
    binding: { name: string; revision: number };
    definition: BindingManifest;
    yaml: string;
    digest: string;
    warnings: string[];
    evidence: { fingerprint: string; to: string; warnings: string[] };
  };
  journey?: {
    yaml: string;
    digest: string;
    expectedRevision: number;
    manifest: Manifest;
  };
  warnings: string[];
}
export function intentPopulation(text: string): Record<string, string> {
  const value: unknown = parse(text);
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    !Object.keys(value).length ||
    Object.values(value).some((v) => typeof v !== "string")
  )
    throw new Error(
      'Provide a YAML population mapping with quoted string values, for example environment: "production".',
    );
  return value as Record<string, string>;
}
export function intentGoal(target: string, window: string) {
  const n = Number(target);
  if (!Number.isFinite(n) || n <= 0 || n >= 100)
    throw new Error("Target must be greater than 0% and less than 100%.");
  return {
    target: Number((n / 100).toPrecision(12)),
    windowSeconds: durationSeconds(window),
  };
}

export interface IntentBacktest {
  from: string;
  to: string;
  resolutionSeconds: number;
  completeCutoffs: number;
  unknownCutoffs: number;
  noTrafficCutoffs: number;
  targetViolationCutoffs: number;
  burnThresholdCutoffs: number;
  warnings: string[];
  points: {
    at: string;
    reliability: number | null;
    coverage: number;
    dataStatus: string;
    targetViolated: boolean;
    shortBurn: number | null;
    longBurn: number | null;
    shortCoverage: number;
    longCoverage: number;
    burnThresholdExceeded: boolean;
  }[];
}

export function backtestTime(text: string): string {
  const m =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(0+))?(Z|[+-]\d{2}:\d{2})$/.exec(
      text,
    );
  if (!m)
    throw new Error(
      "Use RFC 3339 with whole seconds and an explicit timezone, for example 2026-10-20T06:00:00Z.",
    );
  const parts = m.slice(1, 7).map(Number);
  const calendar = new Date(0);
  calendar.setUTCFullYear(parts[0], parts[1] - 1, parts[2]);
  calendar.setUTCHours(parts[3], parts[4], parts[5], 0);
  const actual = [
    calendar.getUTCFullYear(),
    calendar.getUTCMonth() + 1,
    calendar.getUTCDate(),
    calendar.getUTCHours(),
    calendar.getUTCMinutes(),
    calendar.getUTCSeconds(),
  ];
  const parsed = new Date(text);
  if (
    parts.some((p, i) => p !== actual[i]) ||
    !Number.isFinite(parsed.getTime())
  )
    throw new Error("Enter a valid calendar date and timezone.");
  return parsed.toISOString();
}
