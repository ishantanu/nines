import { api, type RequestScope } from "./api";
export const visualNotice = "visual-sharing-v1";
export type VisualReceipt = {
  id: string;
  tenant: string;
  journeyId: string;
  journeyRevision: number;
  imageDigest: string;
  criterionDigest: string;
  requestDigest: string;
  createdAt: string;
  expiresAt: string;
  route: { provider: string; model: string; contractDigest: string };
  consent: { noticeVersion: string; at: string; actor: { name: string } };
  assessment: {
    status: string;
    resolvedModel?: string;
    judgments?: {
      successVisible: number;
      errorVisible: number;
      evidenceSufficient: number;
    };
  };
  review?: {
    outcome: string;
    reason: string;
    at: string;
    actor: { name: string };
  };
};
const path = (journey: string) =>
  `/api/journeys/${encodeURIComponent(journey)}/visual-evaluations`;
export const visualCapabilities = (scope: RequestScope) =>
  api<{
    tenant: string;
    enabled: boolean;
    routeDigest: string;
    noticeVersion: string;
    route?: { provider: string; model: string };
  }>("/api/visual-evaluations/capabilities", scope).then((value) => {
    if (
      value.tenant !== scope.tenant ||
      value.noticeVersion !== visualNotice ||
      (value.enabled &&
        (!value.route || !/^[a-f0-9]{64}$/.test(value.routeDigest)))
    )
      throw Error("Visual capability scope or contract mismatch");
    return value;
  });
export const listVisual = (scope: RequestScope, journey: string) =>
  api<{ tenant: string; items: VisualReceipt[] }>(path(journey), scope).then(
    (result) => {
      if (result.tenant !== scope.tenant || !Array.isArray(result.items))
        throw Error("Visual evidence scope mismatch");
      return result.items;
    },
  );
export const createVisual = (
  scope: RequestScope,
  journey: string,
  input: {
    id: string;
    routeDigest: string;
    journeyRevision: number;
    successCriterion: string;
    imageBase64: string;
    consent: boolean;
    noticeVersion: string;
  },
) =>
  api<VisualReceipt>(path(journey), scope, {
    method: "POST",
    body: JSON.stringify(input),
  });
export const reviewVisual = (
  scope: RequestScope,
  receipt: VisualReceipt,
  outcome: string,
  reason: string,
) =>
  api<VisualReceipt>(
    `${path(receipt.journeyId)}/${encodeURIComponent(receipt.id)}/review`,
    scope,
    {
      method: "POST",
      body: JSON.stringify({
        requestDigest: receipt.requestDigest,
        outcome,
        reason,
      }),
    },
  );
export async function readVisualFile(file: File): Promise<string> {
  if (
    !["image/png", "image/jpeg"].includes(file.type) ||
    file.size === 0 ||
    file.size > 2 * 1024 * 1024
  )
    throw Error("Choose a PNG or JPEG of at most 2 MiB.");
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(Error("The image could not be read."));
    reader.onload = () => {
      const value = String(reader.result);
      resolve(value.slice(value.indexOf(",") + 1));
    };
    reader.readAsDataURL(file);
  });
}
