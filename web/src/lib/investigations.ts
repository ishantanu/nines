import { workspacePath, parseRoute } from "./navigation";
import type { RequestScope } from "./api";
import { api } from "./api";
export type InvestigationScope = {
  kind: "ServiceLevelObjective" | "CriticalUserJourney";
  resourceId: string;
  expectedRevision: number;
  at: string;
};
export type InvestigationActor = {
  subject: string;
  name: string;
  kind: string;
};
export type EvidenceContext = {
  tenant: string;
  observedAt: string;
  health: string;
  warnings: string[];
  resource: { id: string; revision: number; displayName: string };
  measurement?: {
    reliability: number | null;
    target: number;
    coverage: number;
    burnRate: number | null;
    dataStatus: string;
    evaluatedAt: string;
    windowStart: string;
  };
  definition?: { measurementMode: string };
  healthScope?: string;
  totalLinks?: number;
  nextCursor?: string;
  links?: {
    link: { id: string };
    context?: EvidenceContext;
    error?: { message: string };
  }[];
};
export type InvestigationEntry = {
  id: string;
  kind: "message" | "evidence";
  createdAt: string;
  actor: InvestigationActor;
  text?: string;
  evidence?: { capturedAt: string; digest: string; context: EvidenceContext };
};
export type InvestigationSummary = {
  id: string;
  title: string;
  version: number;
  scope: InvestigationScope;
  createdAt: string;
  updatedAt: string;
  createdBy: InvestigationActor;
};
export type Investigation = InvestigationSummary & {
  apiVersion: string;
  tenant: string;
  entries: InvestigationEntry[];
};
export type InvestigationList = {
  apiVersion: string;
  tenant: string;
  items: InvestigationSummary[];
  nextCursor?: string;
};
export function investigationLink(tenant: string, id = "") {
  return workspacePath(tenant, "investigations", id);
}
export function investigationFromHash(
  path: string,
  tenant: string,
): string | null {
  const route = parseRoute(path.startsWith("#") ? path.slice(1) : path);
  return route.kind === "workspace" &&
    route.tenant === tenant &&
    route.page === "investigations"
    ? route.id
    : null;
}
export function listInvestigations(scope: RequestScope, cursor = "") {
  return api<InvestigationList>(
    `/api/investigations?limit=25${cursor ? "&cursor=" + encodeURIComponent(cursor) : ""}`,
    scope,
  );
}
export function readInvestigation(scope: RequestScope, id: string) {
  return api<Investigation>(
    "/api/investigations/" + encodeURIComponent(id),
    scope,
  );
}
export function createInvestigation(
  scope: RequestScope,
  input: { id: string; title: string; scope: InvestigationScope },
) {
  return api<Investigation>("/api/investigations", scope, {
    method: "POST",
    body: JSON.stringify(input),
  });
}
export function saveInvestigationMessage(
  scope: RequestScope,
  id: string,
  input: { id: string; expectedVersion: number; text: string },
) {
  return api<Investigation>(
    `/api/investigations/${encodeURIComponent(id)}/messages`,
    scope,
    { method: "POST", body: JSON.stringify(input) },
  );
}
export function captureInvestigationEvidence(
  scope: RequestScope,
  id: string,
  input: { id: string; expectedVersion: number },
) {
  return api<Investigation>(
    `/api/investigations/${encodeURIComponent(id)}/evidence`,
    scope,
    { method: "POST", body: JSON.stringify(input) },
  );
}

export type ReviewSkillRef = { id: string; version: string; digest: string };
export type ReviewSkill = {
  apiVersion: string;
  kind: "ReviewSkill";
  metadata: { name: string; version: string };
  digest: string;
  spec: {
    displayName: string;
    description: string;
    mode: string;
    scopeKinds: string[];
    toolAllowlist: string[];
    requiredTools: string[];
    evidenceObligations: string[];
    inputSchema: Record<string, unknown>;
    outputSchema: Record<string, unknown>;
    instructions: string;
  };
};
export function listReviewSkills(scope: RequestScope) {
  return api<{ apiVersion: string; tenant: string; items: ReviewSkill[] }>(
    "/api/review-skills",
    scope,
  );
}
export type ReviewRun = {
  skill?: ReviewSkillRef;
  skillInputDigest?: string;
  skillResult?: {
    inputDigest: string;
    evidenceDigests: string[];
    completedTools: string[];
    outputDigest: string;
  };
  mode?: "deterministic" | "model";
  question?: string;
  modelRoute?: {
    provider: string;
    model: string;
    protocol: string;
    local: boolean;
  };
  modelCalls?: number;
  toolCalls?: number;
  receivedBytes?: number;
  outputTokensReserved?: number;
  reportedOutputTokens?: number;
  usageMissing?: number;
  modelReview?: {
    summary: string;
    evidenceDigests: string[];
    limitations: string[];
  };
  apiVersion: string;
  tenant: string;
  id: string;
  investigationId: string;
  version: number;
  reviewVersion: number;
  scope: InvestigationScope;
  requestedBy: InvestigationActor;
  cancelledBy?: InvestigationActor;
  status: "queued" | "running" | "completed" | "failed" | "cancelled";
  errorCode?: string;
  createdAt: string;
  updatedAt: string;
  deadlineAt: string;
  startedAt?: string;
  finishedAt?: string;
  attempts: number;
  steps: {
    tool: string;
    callId?: string;
    status: string;
    attempts: number;
    completedAt?: string;
    evidence?: NonNullable<InvestigationEntry["evidence"]>;
    assessment?: {
      evidenceDigest: string;
      state: string;
      findings: { resourceId: string; state: string; message: string }[];
      limitations: string[];
    };
  }[];
};
const runsPath = (id: string) =>
  `/api/investigations/${encodeURIComponent(id)}/runs`;
export function listReviewRuns(scope: RequestScope, id: string) {
  return api<{ apiVersion: string; tenant: string; items: ReviewRun[] }>(
    runsPath(id),
    scope,
  );
}
export function readReviewRun(scope: RequestScope, id: string, run: string) {
  return api<ReviewRun>(`${runsPath(id)}/${encodeURIComponent(run)}`, scope);
}
export function createReviewRun(
  scope: RequestScope,
  id: string,
  input: {
    id: string;
    expectedVersion: number;
    mode?: string;
    question?: string;
    skill?: ReviewSkillRef;
  },
) {
  return api<ReviewRun>(runsPath(id), scope, {
    method: "POST",
    body: JSON.stringify(input),
  });
}
export function cancelReviewRun(
  scope: RequestScope,
  id: string,
  run: string,
  expectedVersion: number,
) {
  return api<ReviewRun>(
    `${runsPath(id)}/${encodeURIComponent(run)}/cancel`,
    scope,
    { method: "POST", body: JSON.stringify({ expectedVersion }) },
  );
}

export type ReviewCapabilities = {
  apiVersion: string;
  tenant: string;
  model?: { provider: string; model: string; local: boolean };
  maxModelCalls: number;
  maxToolCalls: number;
  maxOutputTokens: number;
  maxInputBytes: number;
};
export function reviewCapabilities(scope: RequestScope) {
  return api<ReviewCapabilities>("/api/investigations/capabilities", scope);
}
