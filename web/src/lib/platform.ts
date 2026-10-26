import { authAction } from "./api";
export type PlatformTenant = {
  id: string;
  name: string;
  sourceKind: string;
  connections: number;
  systemOne?: { provider: string; model: string; contractDigest: string };
};
export type PlatformOverview = {
  tenants: PlatformTenant[];
  providers: { id: string; name: string; type: string }[];
  administrators: { provider: string; subject: string }[];
  auditRetentionDays: number;
};
export type PlatformUser = {
  user: {
    id: string;
    provider: string;
    subject: string;
    name: string;
    email?: string;
    version: number;
  };
  platformAdmin: boolean;
  local: boolean;
  memberships: {
    tenant: string;
    member: { role: string; effectiveRole: string; version: number };
  }[];
  effectiveGrants: Record<string, string>;
};
export type PlatformEvent = {
  id: string;
  at: string;
  actorId: string;
  actorName?: string;
  actorProvider?: string;
  action: string;
  resource: string;
  reason: string;
  outcome: string;
  details?: unknown;
};
export type PlatformPolicy = {
  tenant: string;
  configured: boolean;
  policy: { version: number; enabled: boolean; dailyRequestLimit: number };
  date: string;
  reservedRequests: number;
  route?: { provider: string; model: string };
};
export async function platformRead<T>(
  path: string,
  signal: AbortSignal,
): Promise<T> {
  const response = await fetch("/api/platform/" + path, {
    credentials: "same-origin",
    signal,
    headers: { Accept: "application/json" },
  });
  const body = await response.json();
  if (!response.ok)
    throw Object.assign(Error(body.error ?? "Platform request failed"), {
      status: response.status,
    });
  return body as T;
}
export function platformWrite<T>(
  path: string,
  csrf: string,
  body: unknown,
): Promise<T> {
  return authAction<T>("/api/platform/" + path, csrf, body);
}
