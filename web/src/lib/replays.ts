import { workspacePath, parseRoute } from "./navigation";
import { api, type RequestScope } from "./api";
import type { ReplayEvent } from "./replay-capture";
export type ReplayRef = {
  kind: string;
  id: string;
  revision: number;
  sloId?: string;
  at?: string;
};
export type SessionReplay = {
  apiVersion: string;
  tenant: string;
  id: string;
  title: string;
  version: number;
  createdAt: string;
  expiresAt: string;
  startedAt: string;
  finishedAt: string;
  durationMillis: number;
  eventCount: number;
  sizeBytes: number;
  digest: string;
  policy: string;
  recordedBy: { name: string };
  references: ReplayRef[];
  archived: boolean;
};
export type ReplayList = {
  apiVersion: string;
  tenant: string;
  items: SessionReplay[];
  retentionDays: number;
  maxSessions: number;
};
export type ReplayEventSet = {
  apiVersion: string;
  tenant: string;
  id: string;
  digest: string;
  events: ReplayEvent[];
};
export const listReplays = (s: RequestScope, archived = false) =>
  api<ReplayList>(
    "/api/session-replays" + (archived ? "?archived=true" : ""),
    s,
  );
export const replayEvents = (s: RequestScope, id: string) =>
  api<ReplayEventSet>(
    `/api/session-replays/${encodeURIComponent(id)}/events`,
    s,
  );
export const createReplay = (
  s: RequestScope,
  incoming: {
    id: string;
    title: string;
    consent: boolean;
    sharingConsent: boolean;
    noticeVersion: string;
    policy: string;
    references: ReplayRef[];
    events: ReplayEvent[];
  },
) =>
  api<SessionReplay>("/api/session-replays", s, {
    method: "POST",
    body: JSON.stringify(incoming),
  });
export const linkReplay = (
  s: RequestScope,
  replay: SessionReplay,
  references: ReplayRef[],
) =>
  api<SessionReplay>(
    `/api/session-replays/${encodeURIComponent(replay.id)}/links`,
    s,
    {
      method: "POST",
      body: JSON.stringify({ expectedVersion: replay.version, references }),
    },
  );
export const archiveReplay = (s: RequestScope, replay: SessionReplay) =>
  api<SessionReplay>(
    `/api/session-replays/${encodeURIComponent(replay.id)}/archive`,
    s,
    {
      method: "POST",
      body: JSON.stringify({
        expectedVersion: replay.version,
        archived: !replay.archived,
      }),
    },
  );

export function replayLink(tenant: string, id = "") {
  return workspacePath(tenant, "replays", id);
}
export function replayFromHash(path: string, tenant: string): string | null {
  const route = parseRoute(path.startsWith("#") ? path.slice(1) : path);
  return route.kind === "workspace" &&
    route.tenant === tenant &&
    route.page === "replays"
    ? route.id
    : null;
}

export const replayConsentNotice = "replay-sharing-v1";
export type ReplayPrivacy = {
  apiVersion: string;
  tenant: string;
  enabled: boolean;
  noticeVersion: string;
  policyURL: string;
  contact: string;
};
export const replayPrivacy = (scope: RequestScope) =>
  api<ReplayPrivacy>("/api/session-replays/privacy", scope).then((notice) => {
    if (
      notice.apiVersion !== "nines.dev/v1alpha1" ||
      notice.tenant !== scope.tenant ||
      notice.noticeVersion !== replayConsentNotice ||
      typeof notice.enabled !== "boolean"
    )
      throw Error(
        "Replay privacy notice or tenant mismatch; reload the portal.",
      );
    return notice;
  });
export const deleteReplay = (scope: RequestScope, replay: SessionReplay) =>
  api<{ deleted: boolean }>(
    `/api/session-replays/${encodeURIComponent(replay.id)}`,
    scope,
    {
      method: "DELETE",
      body: JSON.stringify({
        expectedVersion: replay.version,
        confirmId: replay.id,
      }),
    },
  );
