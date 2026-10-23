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
  progress?: {
    modelCall: number;
    state: "waiting" | "receiving" | "received";
    bytes: number;
    at: string;
  };
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
    measurementDigest?: string;
    binding: { name: string; expectedRevision: number };
    actor: { name: string };
    at: string;
  };
  plan?: IntentPlan;
  backtest?: IntentBacktest;
  runs: FlowRun[];
  audit?: IntentAuditEvent[];
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
  state: "proposed" | "approved" | "applied" | "rejected" | "superseded";
  recommendation?: IntentRecommendation;
  approvalReason?: string;
  rejectedBy?: { name: string };
  rejectionReason?: string;
  rejectedAt?: string;
  rollback?: IntentRollback;
  rollbackHistory?: IntentRollback[];
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
  operation?: "apply" | "rollback";
  rollbackDigest?: string;
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

export type IntentRecommendation = {
  measurementDigest?: string;
  requestedBy?: { subject: string; name: string; kind: string };
  compilerVersion?: string;
  rollbackStatus?:
    "no_definition_change" | "creation_has_no_baseline" | "restorable_updates";
  summary: string;
  explanation?: string;
  basis: string[];
  limitations: string[];
  runId?: string;
  mode: string;
  modelRoute?: {
    provider: string;
    model: string;
    protocol: string;
    routeDigest: string;
    local: boolean;
  };
  draftRoute?: {
    provider: string;
    model: string;
    protocol: string;
    routeDigest: string;
    local: boolean;
  };
  skill?: { id: string; version: string; digest: string };
  binding: { name: string; revision: number; digest: string };
  sourceDigest: string;
  evidenceDigest: string;
  observedFrom: string;
  observedTo: string;
  fields: {
    resource: string;
    field: string;
    before?: unknown;
    after?: unknown;
    impact: string;
  }[];
  backtest?: {
    from: string;
    to: string;
    resolutionSeconds: number;
    complete: number;
    unknown: number;
    noTraffic: number;
    targetViolations: number;
    burnBreaches: number;
    digest: string;
  };
};
export type IntentAuditEvent = {
  sequence: number;
  at: string;
  actor: { subject: string; name: string; kind: string };
  action: string;
  outcome: string;
  reason: string;
  changeSetDigest?: string;
  version: number;
  previousHash?: string;
  hash: string;
};
export type IntentRollback = {
  id: string;
  digest: string;
  originalDigest: string;
  state: "proposed" | "approved" | "applied" | "blocked" | "rejected";
  reason: string;
  createdAt: string;
  createdBy: { name: string };
  changes: IntentChangeSet["changes"];
  blockers?: string[];
  approvedBy?: { name: string };
  approvedAt?: string;
  approvalReason?: string;
  appliedBy?: { name: string };
  appliedAt?: string;
  applied?: { name: string; revision: number; digest: string }[];
};
