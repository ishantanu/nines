import { api, type RequestScope } from "./api";
import type { IntentDocument, IntentPlan, IntentBacktest } from "./intent";
export type AuthoringScope = {
  metadata: { name: string; project: string };
  service: string;
  source: string;
  population: Record<string, string>;
  noDataPolicy: "unknown";
  journey?: IntentDocument["spec"]["journey"];
};
export type FlowRun = {
  id: string;
  mode: "guided" | "model";
  status: string;
  modelCalls: number;
  skill: { id: string; version: string; digest: string };
  errorCode?: string;
  steps: { tool: string; at: string; digest: string; result: unknown }[];
};
export type ConversationSummary = {
  id: string;
  title: string;
  version: number;
  state: string;
  scope: AuthoringScope;
  createdAt: string;
  updatedAt: string;
};
export type IntentConversation = ConversationSummary & {
  tenant: string;
  apiVersion: string;
  messages: {
    id: string;
    role: string;
    text: string;
    at: string;
    actor: { name: string; kind: string };
    runId?: string;
  }[];
  goal?: IntentDocument;
  selection?: {
    intentDigest: string;
    binding: { name: string; expectedRevision: number };
    actor: { name: string };
    at: string;
  };
  plan?: IntentPlan;
  backtest?: IntentBacktest;
  runs: FlowRun[];
  changeSet?: IntentChangeSet;
  changeSetHistory?: IntentChangeSet[];
};
export type FlowCapabilities = {
  tenant: string;
  modelEnabled: boolean;
  draftEnabled: boolean;
  skill: { id: string; version: string; digest: string };
};
export const conversationBase = "/api/intent-conversations";
export const conversations = (scope: RequestScope, cursor = "") =>
  api<{ tenant: string; items: ConversationSummary[]; nextCursor?: string }>(
    conversationBase + (cursor ? "?cursor=" + encodeURIComponent(cursor) : ""),
    scope,
  );
export const conversation = (scope: RequestScope, id: string) =>
  api<IntentConversation>(
    conversationBase + "/" + encodeURIComponent(id),
    scope,
  );
export function conversationLink(tenant: string, id = "") {
  return `#/tenants/${encodeURIComponent(tenant)}/intents${id ? "/" + encodeURIComponent(id) : ""}`;
}
export function conversationFromHash(
  hash: string,
  tenant: string,
): string | null {
  const base = conversationLink(tenant);
  if (hash === base) return "";
  if (!hash.startsWith(base + "/")) return null;
  const id = hash.slice(base.length + 1);
  return /^[a-z0-9][a-z0-9-]{0,62}$/.test(id) ? id : null;
}
export function authoringScope(goal: IntentDocument): AuthoringScope {
  return {
    metadata: goal.metadata,
    service: goal.spec.service,
    source: goal.spec.source,
    population: goal.spec.population,
    noDataPolicy: goal.spec.noDataPolicy,
    ...(goal.spec.journey ? { journey: goal.spec.journey } : {}),
  };
}

export type IntentChangeSet = {
  id: string;
  digest: string;
  state: "proposed" | "approved" | "applied";
  createdAt: string;
  approvedAt?: string;
  appliedAt?: string;
  approvedBy?: { name: string };
  appliedBy?: { name: string };
  changes: {
    kind: string;
    name: string;
    action: string;
    expectedRevision: number;
    digest: string;
    manifest: unknown;
    previous?: unknown;
  }[];
  applied?: { kind: string; name: string; revision: number; digest: string }[];
};
export type IntentVerification = {
  sourceState: string;
  tenant: string;
  conversation: string;
  digest: string;
  resources: {
    kind: string;
    name: string;
    revision: number;
    currentRevision: number;
    state: string;
    health: string;
  }[];
};
