import type { BadIntervalPolicy } from "./api";

const LABEL_KEY = /^[a-z][a-z0-9_.-]{0,31}$/;
export const MAX_LABELS = 8;
export const MAX_CUSTOMERS = 20;

// Rows keep drafts the user is still typing; conversion refuses anything the
// server would reject so a bad value is explained here, not after review.
export type LabelRow = { key: string; value: string };
export const labelRows = (labels?: Record<string, string>): LabelRow[] =>
  Object.entries(labels ?? {}).map(([key, value]) => ({ key, value }));
export function labelsFromRows(
  rows: LabelRow[],
): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const { key, value } of rows) {
    if (!key && !value) continue;
    if (!LABEL_KEY.test(key))
      throw new Error(
        "Label names start with a lowercase letter and use lowercase letters, digits, dots, hyphens or underscores (up to 32 characters).",
      );
    if (!value.trim() || value.length > 63)
      throw new Error("Label values are 1 to 63 characters.");
    if (Object.hasOwn(out, key)) throw new Error(`Label ${key} is repeated.`);
    out[key] = value;
  }
  if (Object.keys(out).length > MAX_LABELS)
    throw new Error(`Use at most ${MAX_LABELS} labels.`);
  return Object.keys(out).length ? out : undefined;
}

export type IntervalDraft = {
  enabled: boolean;
  /** "errorRate" judges good-minutes; "throughput" judges minimum volume. */
  mode: "errorRate" | "throughput";
  errorRate: string;
  minEvents: string;
  minGoodEvents: string;
};
export const intervalDraft = (p?: BadIntervalPolicy): IntervalDraft => ({
  enabled: !!p,
  mode: p?.minGoodEvents ? "throughput" : "errorRate",
  errorRate: p?.errorRate ? String(p.errorRate) : "0.05",
  minEvents: p?.minEvents ? String(p.minEvents) : "",
  minGoodEvents: p?.minGoodEvents ? String(p.minGoodEvents) : "",
});
export function badIntervalFromDraft(
  d: IntervalDraft,
): BadIntervalPolicy | undefined {
  if (!d.enabled) return undefined;
  if (d.mode === "throughput") {
    const least = Number(d.minGoodEvents);
    if (!d.minGoodEvents.trim() || !Number.isFinite(least) || least <= 0)
      throw new Error(
        "Enter the fewest good events a healthy interval records, above 0.",
      );
    return { minGoodEvents: least };
  }
  const errorRate = Number(d.errorRate);
  if (!Number.isFinite(errorRate) || errorRate <= 0 || errorRate >= 1)
    throw new Error("The interval error rate must be above 0 and below 1.");
  const min = d.minEvents.trim() ? Number(d.minEvents) : 0;
  if (!Number.isInteger(min) || min < 0)
    throw new Error("Minimum events per interval must be a whole number.");
  return { errorRate, ...(min > 0 ? { minEvents: min } : {}) };
}

/** Throws the first problem the server would also refuse. */
export function validateExtras(x: {
  labels?: Record<string, string>;
  customers?: {
    label: string;
    customers: { value: string; target?: number }[];
  };
}) {
  labelsFromRows(labelRows(x.labels));
  const c = x.customers;
  if (!c) return;
  if (
    !/^[a-zA-Z_][a-zA-Z0-9_]{0,99}$/.test(c.label) ||
    c.label.startsWith("__")
  )
    throw new Error("Use a valid metric label name for the customer label.");
  if (!c.customers.length || c.customers.length > MAX_CUSTOMERS)
    throw new Error(`List 1 to ${MAX_CUSTOMERS} customers.`);
  const seen = new Set<string>();
  for (const row of c.customers) {
    if (!row.value.trim())
      throw new Error("Every customer needs a label value.");
    if (seen.has(row.value))
      throw new Error(`Customer ${row.value} is repeated.`);
    seen.add(row.value);
    if (row.target !== undefined && !(row.target > 0 && row.target < 1))
      throw new Error("A customer target must be above 0% and below 100%.");
  }
}
