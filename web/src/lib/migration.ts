import { api, message, type RequestScope } from "./api";

export interface MigrationNote {
  level: "info" | "review" | "lossy";
  field?: string;
  message: string;
}
export interface MigrationDefinition {
  name: string;
  origin: string;
  yaml: string;
  notes: MigrationNote[];
}
export interface MigrationResult {
  tool: string;
  definitions: MigrationDefinition[];
  skipped: { origin: string; reason: string }[];
  notes: MigrationNote[];
  limitations: string[];
}
export interface MigrationOptions {
  project: string;
  source: string;
  lifecycle: "experimental" | "aspirational" | "committed";
  windowDays: string;
}

export type ImportState =
  | { step: "ready" }
  | {
      step: "planned";
      action: string;
      expectedRevision: number;
      warnings: string[];
    }
  | { step: "applied" }
  | { step: "failed"; error: string };

export const levelLabel: Record<MigrationNote["level"], string> = {
  info: "Nothing lost",
  review: "Review",
  lossy: "Not carried over",
};

export function noteCounts(notes: MigrationNote[]) {
  return {
    lossy: notes.filter((n) => n.level === "lossy").length,
    review: notes.filter((n) => n.level === "review").length,
  };
}

export async function convert(
  scope: RequestScope,
  tool: "sloth" | "pyrra",
  content: string,
  o: MigrationOptions,
  signal: AbortSignal,
): Promise<MigrationResult> {
  const days = Number(o.windowDays);
  return api<MigrationResult>("/api/migrations/convert", scope, {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tool,
      content,
      ...(o.project.trim() ? { project: o.project.trim() } : {}),
      ...(o.source.trim() ? { source: o.source.trim() } : {}),
      lifecycle: o.lifecycle,
      ...(tool === "sloth" && Number.isFinite(days) && days > 0
        ? { windowSeconds: Math.round(days * 86400) }
        : {}),
    }),
  });
}

/** Plans one converted definition without saving it. */
export async function planOne(
  scope: RequestScope,
  d: MigrationDefinition,
  signal: AbortSignal,
): Promise<ImportState> {
  try {
    const p = await api<{
      action: string;
      expectedRevision: number;
      warnings?: string[];
    }>("/api/resources/plan", scope, {
      method: "POST",
      signal,
      headers: { "Content-Type": "application/yaml" },
      body: d.yaml,
    });
    return {
      step: "planned",
      action: p.action,
      expectedRevision: p.expectedRevision,
      warnings: p.warnings ?? [],
    };
  } catch (e) {
    return { step: "failed", error: message(e) };
  }
}

/** Applies one planned definition with its reviewed expected revision. */
export async function applyOne(
  scope: RequestScope,
  d: MigrationDefinition,
  expectedRevision: number,
  signal: AbortSignal,
): Promise<ImportState> {
  try {
    await api("/api/resources/apply", scope, {
      method: "POST",
      signal,
      headers: {
        "Content-Type": "application/yaml",
        "If-Match": `"${expectedRevision}"`,
      },
      body: d.yaml,
    });
    return { step: "applied" };
  } catch (e) {
    return { step: "failed", error: message(e) };
  }
}
