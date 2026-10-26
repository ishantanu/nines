import { observedFetch } from "./observability";
export type Health = "healthy" | "at_risk" | "unknown" | "degraded";
export type SLOLifecycle =
  "experimental" | "aspirational" | "committed" | "deprecated";
export interface BurnRatePolicy {
  minEvents?: number;
  name?: string;
  severity?: "critical" | "warning" | "info";
  shortWindowSeconds: number;
  longWindowSeconds: number;
  burnRate: number;
  forSeconds: number;
  minCoverage: number;
}
export type MeasurementSpec = {
  type: "availability" | "latency";
  metric: string;
  labels: Record<string, string>;
  successLabels?: Record<string, string>;
  countFrom?: "counter" | "histogram";
  thresholdSeconds?: number;
  scrapeIntervalSeconds: number;
  observationPoint: string;
};
export interface MeasurementQualityPolicy {
  minEvents?: number;
  maxSampleAgeSeconds?: number;
  cohortLabels?: string[];
}
export interface MeasurementQuality {
  collectedSourceDigest?: string;
  sourceFrom?: string;
  sourceTo?: string;
  apiVersion: string;
  tenant: string;
  sloId: string;
  revision: number;
  definitionDigest: string;
  sourceDigest: string;
  observedAt: string;
  evaluatedAt: string;
  state: "usable" | "limited" | "unavailable";
  good: number;
  total: number;
  coverage: number;
  minimumEvents: number;
  observationPoint?: string;
  freshness: {
    state: string;
    lastSampleAt?: string;
    ageAtObservationSeconds?: number;
    staleAfterSeconds: number;
  };
  checks: { code: string; state: string; message: string }[];
  limitations: string[];
  cohorts?: {
    state: string;
    message: string;
    at: string;
    windowSeconds: number;
    labels: string[];
    matchesAggregate: boolean;
    items: {
      labels: Record<string, string>;
      good: number | null;
      total: number;
      reliability: number | null;
      burnRate: number | null;
      state: string;
    }[];
  };
}
export interface ErrorBudgetPolicy {
  owner: string;
  runbookURL: string;
  reviewBelow: number;
  onExhausted?: "blocked" | "review_required";
}
export interface ErrorBudgetDecision {
  evidence?: {
    coverage: number;
    total: number;
    minimumEvents: number;
    freshness: MeasurementQuality["freshness"];
    checks: MeasurementQuality["checks"];
  };
  apiVersion: string;
  tenant: string;
  sloId: string;
  revision: number;
  definitionDigest: string;
  sourceDigest: string;
  policyDigest?: string;
  observedAt: string;
  evaluatedAt: string;
  state:
    | "not_configured"
    | "insufficient_evidence"
    | "review_required"
    | "blocked"
    | "proceed";
  reasonCodes: string[];
  message: string;
  owner?: string;
  runbookURL?: string;
  budgetRemainingFraction: number | null;
  qualityState: string;
  advisory: boolean;
}
export interface SLO {
  errorBudgetPolicy?: ErrorBudgetPolicy;
  quality?: MeasurementQualityPolicy;
  lifecycle?: SLOLifecycle;
  intent?: import("./intent").IntentSpec;
  measurement?: MeasurementSpec;
  providerWindow?: {
    goodSelector: string;
    totalSelector: string;
    scrapeIntervalSeconds: number;
  };
  measurementBinding?: { name: string; revision: number; digest: string };
  source?: string;
  revision?: number;
  id: string;
  project: string;
  service: string;
  name: string;
  target: number;
  windowSeconds: number;
  stepSeconds: number;
  goodQuery?: string;
  totalQuery?: string;
  policies?: BurnRatePolicy[];
  policy?: BurnRatePolicy;
}
export interface Snapshot {
  sloId: string;
  at: string;
  from: string;
  good: number;
  total: number;
  bad: number;
  coverage: number;
  reliability: number | null;
  budgetRemainingFraction: number | null;
  burnRate: number | null;
  dataStatus: string;
  alert: {
    suppressedReason?: string;
    phase: string;
    policies?: Record<
      string,
      { phase: string; severity: string; policy: string }
    >;
  };
}
export interface Objective {
  budgetDecision?: ErrorBudgetDecision;
  quality?: MeasurementQuality;
  collection?: {
    state: string;
    points: number;
    completedAt?: string;
    sloRevision?: number;
  };
  measurementHealth?: {
    state: string;
    message: string;
    checkedAt: string;
    evaluationBlocked: boolean;
  };
  slo: SLO;
  status: Snapshot;
}
export interface Link {
  id: string;
  dimension: string;
  required: boolean;
}
export interface Step {
  id: string;
  name: string;
  actor: "user" | "service" | "agent" | "human";
  dependsOn?: string[];
  sloRefs: string[];
}
export interface Manifest {
  apiVersion: string;
  kind: string;
  metadata: { name: string; project: string };
  spec: {
    displayName: string;
    description: string;
    owner: string;
    criticality: string;
    slos: Link[];
    steps: Step[];
    agentic?: {
      successDefinition: string;
      maxDurationSeconds?: number;
      maxCostUSD?: number;
      maxSteps?: number;
      approvalRequired: boolean;
    };
  };
}
export interface Revision {
  manifest: Manifest;
  revision: number;
  digest: string;
  appliedAt: string;
}
export interface Journey {
  resource: Revision;
  health: Health;
  links: { link: Link; slo: SLO; status: Snapshot; health: Health }[];
  reasons: string[];
}
export interface Run {
  evidence: {
    id: string;
    journeyRevision: number;
    startedAt: string;
    finishedAt: string;
    outcome: string;
    costUSD?: number;
    steps?: number;
    approvalObserved?: boolean;
    policyViolations?: string[];
  };
  journeyId: string;
  contractDigest: string;
  verdict: string;
  findings: string[];
  receivedAt: string;
}
export interface Delivery {
  event: {
    configRevision?: number;
    id: string;
    sloId: string;
    kind: string;
    time: string;
    shortBurn: number;
    longBurn: number;
  };
  status: string;
  attempts: number;
  lastError?: string;
}
export interface Workspace {
  mode: string;
  prometheusConfigured: boolean;
  journeysConfigured: boolean;
  tenant: { id: string; name: string };
  source: {
    kind: "manual" | "prometheus" | "mimir" | "multiple";
    configured: boolean;
    connectionCount?: number;
  };
  role: Role;
  workflowsConfigured: boolean;
}
export type Role = "owner" | "editor" | "agent" | "viewer";
export interface Tenant {
  id: string;
  name: string;
  role: Role;
}
export interface RequestScope {
  token: string;
  tenant: string;
  role: Role;
  signal: AbortSignal;
  csrfToken?: string;
}
export interface TenantSession extends RequestScope {
  close: () => void;
}
export function createTenantSession(
  token: string,
  tenant: string,
  role: Role,
  csrfToken?: string,
): TenantSession {
  if (!tenant) throw new Error("Select a tenant before making a request");
  const controller = new AbortController();
  return {
    token,
    tenant,
    role,
    csrfToken,
    signal: controller.signal,
    close: () => controller.abort(),
  };
}
export interface Data {
  journeys: Journey[];
  objectives: Objective[];
  events: Delivery[];
  workspace: Workspace;
}
export interface Plan {
  action: string;
  expectedRevision: number;
  digest: string;
  manifest: Manifest;
  previous?: Revision;
}
export interface Point {
  time: string;
  good: number;
  total: number;
}
export interface Skill {
  id: string;
  version: string;
  name: string;
  description: string;
  digest: string;
  instructions: string;
}
export interface WorkflowRecord {
  id: string;
  kind: "investigate" | "propose_journey";
  state:
    | "completed"
    | "awaiting_approval"
    | "approved"
    | "applied"
    | "rejected"
    | "conflicted";
  version: number;
  skill: { id: string; version: string; digest: string };
  journeyId?: string;
  note: string;
  createdAt: string;
  updatedAt: string;
  findings: {
    code: string;
    severity: "info" | "warning" | "critical";
    resource: string;
    message: string;
  }[];
  proposal?: Plan;
  applied?: Revision;
  decision?: { action: string; reason: string; at: string };
  error?: string;
}
export function workflowDecision(
  workflow: WorkflowRecord,
  action: "approve" | "reject",
  reason: string,
) {
  return {
    expectedVersion: workflow.version,
    reason:
      workflow.state === "approved"
        ? (workflow.decision?.reason ?? "")
        : reason,
    ...(action === "approve" ? { digest: workflow.proposal?.digest } : {}),
  };
}
export class APIError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}
async function request<T>(
  path: string,
  token: string,
  options: RequestInit = {},
): Promise<T> {
  options.signal?.throwIfAborted();
  const headers = new Headers(options.headers);
  if (options.body && !headers.has("Content-Type"))
    headers.set("Content-Type", "application/json");
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await observedFetch(path, {
    ...options,
    credentials: "same-origin",
    headers,
  });
  options.signal?.throwIfAborted();
  if (!response.ok) {
    if (
      (response.status === 401 || response.status === 403) &&
      typeof window !== "undefined" &&
      path.startsWith("/api/") &&
      path !== "/api/tenants"
    )
      window.dispatchEvent(new CustomEvent("nines-access-changed"));
    let message = `Request failed (${response.status})`;
    try {
      const body = await response.json();
      if (typeof body.error === "string") message = body.error;
      else if (typeof body.error?.message === "string")
        message = body.error.message;
    } catch {
      /* Non-JSON error response. */
    }
    options.signal?.throwIfAborted();
    throw new APIError(response.status, message);
  }
  const result = (await response.json()) as T;
  options.signal?.throwIfAborted();
  return result;
}
export function discoverTenants(
  token: string,
  signal: AbortSignal,
): Promise<Tenant[]> {
  return request<Tenant[]>("/api/tenants", token, { signal });
}
export function api<T>(
  path: string,
  scope: RequestScope,
  options: RequestInit = {},
): Promise<T> {
  const headers = new Headers(options.headers);
  // A scope is immutable for its lifetime. Per-request options cannot override it.
  headers.set("X-Nines-Tenant", scope.tenant);
  if (!scope.token && scope.csrfToken)
    headers.set("X-CSRF-Token", scope.csrfToken);
  const signal = options.signal
    ? AbortSignal.any([scope.signal, options.signal])
    : scope.signal;
  return request<T>(path, scope.token, { ...options, headers, signal });
}
export async function loadData(
  scope: RequestScope,
  signal: AbortSignal,
): Promise<Data> {
  const [journeys, objectives, events, workspace] = await Promise.all([
    api<Journey[]>("/api/journeys", scope, { signal }),
    api<Objective[]>("/api/slos", scope, { signal }),
    api<Delivery[]>("/api/events", scope, { signal }),
    api<Workspace>("/api/workspace", scope, { signal }),
  ]);
  return { journeys, objectives, events, workspace };
}
export function healthOf(s: Snapshot): Health {
  if (
    s.alert.phase === "firing" ||
    (s.budgetRemainingFraction !== null && s.budgetRemainingFraction <= 0)
  )
    return "degraded";
  if (s.reliability === null || s.coverage < 1) return "unknown";
  if (s.alert.phase === "pending" || (s.burnRate !== null && s.burnRate > 1))
    return "at_risk";
  return "healthy";
}
export function percent(n: number | null | undefined, digits = 2) {
  return n == null ? "N/A" : `${(n * 100).toFixed(digits)}%`;
}
export function date(s: string) {
  return new Date(s).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
export function message(e: unknown) {
  return e instanceof Error ? e.message : "Something went wrong";
}
// Stable topological layers preserve parallel branches, independent of input order.
export function stepLayers(steps: Step[]): Step[][] {
  const done = new Set<string>();
  const layers: Step[][] = [];
  while (done.size < steps.length) {
    const next = steps.filter(
      (s) => !done.has(s.id) && (s.dependsOn ?? []).every((d) => done.has(d)),
    );
    if (!next.length) return [...layers, steps.filter((s) => !done.has(s.id))];
    layers.push(next);
    next.forEach((s) => done.add(s.id));
  }
  return layers;
}

export interface AuthInfo {
  enabled: boolean;
  authenticated: boolean;
  csrfToken?: string;
  actor?: {
    id: string;
    name: string;
    kind: string;
    provider?: string;
    identitySubject?: string;
    platformAdmin?: boolean;
    expiresAt?: string;
  };
  providers: { id: string; name: string; type: "oidc" | "ldap" | "local" }[];
}
export function authSession(signal?: AbortSignal): Promise<AuthInfo> {
  return request<AuthInfo>("/auth/session", "", { signal });
}
export function authAction<T>(
  path: string,
  csrfToken: string,
  body?: unknown,
): Promise<T> {
  return request<T>(path, "", {
    method: "POST",
    headers: { "X-CSRF-Token": csrfToken },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}
export function canEdit(scope: RequestScope): boolean {
  return scope.role === "owner" || scope.role === "editor";
}
