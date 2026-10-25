import { workspacePath, parseRoute } from "./navigation";
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
  return workspacePath(tenant, "intents", id);
}
export function conversationFromHash(
  path: string,
  tenant: string,
): string | null {
  const route = parseRoute(path.startsWith("#") ? path.slice(1) : path);
  return route.kind === "workspace" &&
    route.tenant === tenant &&
    route.page === "intents"
    ? route.id
    : null;
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

export type IntentSafetyCheck = {
  burnState?: string;
  state: string;
  message: string;
  checkedAt: string;
  quality?: import("./api").MeasurementQuality;
  budget?: import("./api").ErrorBudgetDecision;
};
export type IntentOutcome = {
  burnState?: string;
  tenant: string;
  conversation: string;
  changeSetDigest: string;
  operation: string;
  operationDigest: string;
  sloId: string;
  revision: number;
  state: string;
  code: string;
  message: string;
  startedAt: string;
  checkedAt: string;
  warmupUntil: string;
  deadlineAt: string;
  healthySince?: string;
  nextCheckAt?: string;
  reliability?: number;
  baselineReliability?: number;
  comparison: string;
  rollbackStatus: string;
  limitations: string[];
  quality?: import("./api").MeasurementQuality;
  budget?: import("./api").ErrorBudgetDecision;
  deployment?: { state: string; message: string };
  evaluation?: { state: string; message: string };
  ruleOutput?: { state: string; message: string };
};
export type IntentChangeSet = {
  observationPolicy?: {
    warmupSeconds: number;
    observeSeconds: number;
    deadlineSeconds: number;
  };
  safetyException?: {
    reason: string;
    approvedBy: { subject: string; name: string };
    expiresAt: string;
  };
  safetyCheck?: IntentSafetyCheck;
  outcome?: IntentOutcome;
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
  outcome?: IntentOutcome;
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
  budgetDecision?: import("./api").ErrorBudgetDecision;
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
  outcome?: IntentOutcome;
  safetyCheck?: IntentSafetyCheck;
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
