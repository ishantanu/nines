export interface RetirementState {
  archivedAt: string;
  cleanup: string;
  attempts: number;
  message: string;
  audit: {
    id: string;
    action: string;
    at: string;
    actor: string;
    revision: number;
    digest: string;
  }[];
}
export interface RetirementPlan {
  tenant: string;
  slo: string;
  action: "archive" | "delete";
  expectedRevision: number;
  digest: string;
  references: {
    kind: string;
    name: string;
    revision: number;
    required: boolean;
  }[];
  blockers: string[];
  group?: string;
  routeDigest?: string;
}
