import { api, type RequestScope } from "./api";

export type IncidentState =
  "detected" | "mitigated" | "monitoring" | "resolved";
export type IncidentSLO = {
  id: string;
  configRevision: number;
  policy?: string;
  firingSince?: string;
  clearedAt?: string;
  firing: boolean;
};
export type IncidentSignal = {
  source: string;
  key: string;
  title: string;
  url?: string;
  firingSince?: string;
  clearedAt?: string;
  firing: boolean;
};
export type IncidentEvent = {
  time: string;
  kind: string;
  actor?: string;
  sloId?: string;
  message: string;
};
export type IncidentImpact = {
  sloId: string;
  impactStartedAt?: string;
  from: string;
  to: string;
  badEvents: number | null;
  badEventsAtLeast?: boolean;
  budgetConsumedFraction: number | null;
  coverage: number;
  reason?: string;
};
export type Incident = {
  id: string;
  revision: number;
  title: string;
  source: "alert" | "manual" | "external";
  state: IncidentState;
  declaredBy?: string;
  openedAt: string;
  mitigatedAt?: string;
  resolvedAt?: string;
  slos: IncidentSLO[];
  signals?: IncidentSignal[];
  detectedBySLO: boolean;
  journeys: { id: string; revision: number }[];
  timeline: IncidentEvent[];
  impact: IncidentImpact[];
  postmortemRequirement?: {
    required: boolean;
    reasons: { sloId: string; consumed: number; threshold: number }[];
    unknown?: string[];
    published: boolean;
  };
};

export const stateLabels: Record<IncidentState, string> = {
  detected: "Detected",
  mitigated: "Mitigated",
  monitoring: "Monitoring",
  resolved: "Resolved",
};

// The states a responder may move an incident to, as the server allows.
export const nextStates: Record<IncidentState, IncidentState[]> = {
  detected: ["mitigated", "monitoring", "resolved"],
  mitigated: ["detected", "monitoring", "resolved"],
  monitoring: ["detected", "mitigated", "resolved"],
  resolved: [],
};

export function listIncidents(scope: RequestScope, signal?: AbortSignal) {
  return api<{ incidents: Incident[] }>("/api/incidents?limit=100", scope, {
    signal,
  }).then((r) => r.incidents);
}
export function readIncident(scope: RequestScope, id: string) {
  return api<Incident>(`/api/incidents/${encodeURIComponent(id)}`, scope);
}
export function declareIncident(
  scope: RequestScope,
  input: { title: string; slos: string[] },
) {
  return api<Incident>("/api/incidents", scope, {
    method: "POST",
    body: JSON.stringify(input),
  });
}
export function transitionIncident(
  scope: RequestScope,
  incident: Incident,
  state: IncidentState,
  note: string,
) {
  return api<Incident>(
    `/api/incidents/${encodeURIComponent(incident.id)}/transition`,
    scope,
    {
      method: "POST",
      body: JSON.stringify({
        state,
        note,
        expectedRevision: incident.revision,
      }),
    },
  );
}

const count = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });

// Bad events stay unknown without data, and are a floor when some intervals
// are missing; never shown as zero when they are not known.
export function badEventsText(i: IncidentImpact): string {
  if (i.badEvents === null) return "Unknown";
  return (i.badEventsAtLeast ? "At least " : "") + count.format(i.badEvents);
}

// The budget consumed, or Unknown; the reason is shown beside it.
export function budgetText(i: IncidentImpact): string {
  if (i.budgetConsumedFraction === null) return "Unknown";
  const pct = i.budgetConsumedFraction * 100;
  return `${pct < 0.1 && pct > 0 ? "<0.1" : pct.toFixed(1)}% of the error budget`;
}

// The incident's largest known budget impact, for the list.
export function worstImpact(incident: Incident): string {
  const known = incident.impact.filter(
    (i) => i.budgetConsumedFraction !== null,
  );
  if (known.length === 0)
    return incident.impact.length ? "Impact unknown" : "No SLOs linked";
  const worst = known.reduce((a, b) =>
    (b.budgetConsumedFraction ?? 0) > (a.budgetConsumedFraction ?? 0) ? b : a,
  );
  return `${budgetText(worst)} on ${worst.sloId}`;
}

// How long before the alert users were affected, if Nines can tell.
export function detectionLag(
  slo: IncidentSLO,
  impact?: IncidentImpact,
): string | null {
  if (!slo.firingSince || !impact?.impactStartedAt) return null;
  const seconds =
    (Date.parse(slo.firingSince) - Date.parse(impact.impactStartedAt)) / 1000;
  if (!(seconds > 0)) return null;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  return m ? `${m} min ${s ? `${s} s` : ""}`.trim() : `${s} s`;
}

const toolNames: Record<string, string> = {
  alertmanager: "Alertmanager",
  pagerduty: "PagerDuty",
  "incident.io": "incident.io",
  rootly: "Rootly",
  opsgenie: "Opsgenie",
  grafana: "Grafana",
};
export function toolName(source: string): string {
  return toolNames[source.toLowerCase()] ?? source;
}

export function openedBy(incident: Incident): string {
  if (incident.source === "alert") return "Opened by a critical burn alert";
  if (incident.source === "external")
    return `Opened by an alert from ${toolName(incident.signals?.[0]?.source ?? "another tool")}`;
  return `Declared by ${incident.declaredBy}`;
}

export function formatTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}
