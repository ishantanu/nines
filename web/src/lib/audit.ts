import { api, type RequestScope } from "./api";
import type { IntentAuditEvent, IntentChangeSet } from "./intent-conversations";
export type AuditRecord = {
  recoveredFromConversation?: boolean;
  id: string;
  tenant: string;
  conversation: string;
  event: IntentAuditEvent;
  resources: { kind: string; name: string; revision: number; digest: string }[];
  proposal?: IntentChangeSet;
};
export type AuditPage = {
  tenant: string;
  items: AuditRecord[];
  nextCursor?: string;
  from: string;
  to: string;
};
export async function readAudit(
  scope: RequestScope,
  query: URLSearchParams,
  signal: AbortSignal,
) {
  const result = await api<AuditPage>("/api/audit?" + query, scope, { signal });
  if (
    result.tenant !== scope.tenant ||
    !Array.isArray(result.items) ||
    result.items.some((item) => item.tenant !== scope.tenant)
  )
    throw new Error("Audit tenant mismatch");
  return result;
}
export async function readAuditRecord(
  scope: RequestScope,
  id: string,
  signal: AbortSignal,
) {
  const result = await api<AuditRecord>(
    "/api/audit/" + encodeURIComponent(id),
    scope,
    { signal },
  );
  if (result.tenant !== scope.tenant || result.id !== id)
    throw new Error("Audit record scope mismatch");
  return result;
}

export type ArchiveStatus = {
  recordId?: string;
  tenant: string;
  enabled: boolean;
  state: string;
  pending: number;
  failed: number;
  verified: number;
  retained: number;
  expired: number;
  lastError?: string;
  receipt?: {
    bucket: string;
    key: string;
    versionId: string;
    sha256: string;
    mode: string;
    retainUntil: string;
    verifiedAt: string;
  };
};
export async function readArchiveStatus(
  scope: RequestScope,
  signal: AbortSignal,
  id = "",
) {
  const result = await api<ArchiveStatus>(
    "/api/audit/archive" + (id ? "?record=" + encodeURIComponent(id) : ""),
    scope,
    { signal },
  );
  if (result.tenant !== scope.tenant || (result.recordId ?? "") !== id)
    throw new Error("Archive status scope mismatch");
  return result;
}
