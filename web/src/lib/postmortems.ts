import { api, type RequestScope } from "./api";
import type { Investigation } from "./investigations";
import phrases from "./hindsight.json";

export type CauseKind = "trigger" | "latent" | "safeguard";
export type ActionKind = "prevent" | "detect" | "mitigate";
export type ActionItem = {
  id: string;
  title: string;
  owner: string;
  due?: string;
  kind: ActionKind;
  slos?: string[];
  journeys?: string[];
  done: boolean;
};
export type Postmortem = {
  summary: string;
  addedEvents: { time: string; text: string }[];
  contributingCauses: { kind: CauseKind; text: string }[];
  lessons: { wentWell: string[]; wentPoorly: string[]; gotLucky: string[] };
  actionItems: ActionItem[];
  published: boolean;
};
export type HindsightFinding = {
  field: string;
  phrase: string;
  suggestion: string;
};
export type PostmortemTemplate = {
  postmortem: Postmortem;
  revision?: string;
  findings: HindsightFinding[];
};
export type OpenActionItem = {
  reviewId: string;
  reviewTitle: string;
  incidentId: string;
  item: ActionItem;
};

export const DEFINE_SLO_ACTION = "define-slo";

export const causeLabels: Record<CauseKind, string> = {
  trigger: "Trigger",
  latent: "Latent condition",
  safeguard: "Missing or weak safeguard",
};
export const actionLabels: Record<ActionKind, string> = {
  prevent: "Prevent",
  detect: "Detect",
  mitigate: "Mitigate",
};

export function readPostmortem(scope: RequestScope, reviewId: string) {
  return api<PostmortemTemplate>(
    `/api/investigations/${encodeURIComponent(reviewId)}/postmortem`,
    scope,
  );
}
export function savePostmortem(
  scope: RequestScope,
  review: Investigation,
  postmortem: Postmortem,
) {
  return api<{ investigation: Investigation; findings: HindsightFinding[] }>(
    `/api/investigations/${encodeURIComponent(review.id)}/postmortem`,
    scope,
    {
      method: "POST",
      body: JSON.stringify({
        id: crypto.randomUUID(),
        expectedVersion: review.version,
        postmortem,
      }),
    },
  );
}
export function listOpenActionItems(
  scope: RequestScope,
  filter: { slo?: string; journey?: string },
  signal?: AbortSignal,
) {
  const q = filter.slo
    ? `?slo=${encodeURIComponent(filter.slo)}`
    : filter.journey
      ? `?journey=${encodeURIComponent(filter.journey)}`
      : "";
  return api<{ items: OpenActionItem[] }>(`/api/action-items${q}`, scope, {
    signal,
  }).then((r) => r.items);
}

const patterns = (phrases as { phrase: string; suggestion: string }[]).map(
  (p) => ({
    ...p,
    re: new RegExp(
      `\\b${p.phrase.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`,
      "i",
    ),
  }),
);

// The same hindsight and blame check the server runs, as people type.
export function hindsight(
  text: string,
): { phrase: string; suggestion: string }[] {
  if (!text.trim()) return [];
  return patterns
    .filter((p) => p.re.test(text))
    .map(({ phrase, suggestion }) => ({ phrase, suggestion }));
}

export function emptyPostmortem(): Postmortem {
  return {
    summary: "",
    addedEvents: [],
    contributingCauses: [],
    lessons: { wentWell: [], wentPoorly: [], gotLucky: [] },
    actionItems: [],
    published: false,
  };
}

// Postmortem revisions on a review, oldest first.
export function postmortemRevisions(review: Investigation) {
  return review.entries.filter((e) => e.kind === "postmortem");
}
