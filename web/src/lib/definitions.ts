import type { SLO } from "./api";
export const RESOURCE_API_VERSION = "nines.dev/v1alpha1" as const;
export interface SLODefinition {
  apiVersion: typeof RESOURCE_API_VERSION;
  kind: "ServiceLevelObjective";
  metadata: { name: string; project: string };
  spec: {
    lifecycle?: SLO["lifecycle"];
    quality?: SLO["quality"];
    errorBudgetPolicy?: SLO["errorBudgetPolicy"];
    measurement?: SLO["measurement"];
    intent?: SLO["intent"];
    providerWindow?: SLO["providerWindow"];
    measurementBinding?: SLO["measurementBinding"];
    source?: string;
    displayName: string;
    service: string;
    target: number;
    windowSeconds: number;
    stepSeconds: number;
    goodQuery?: string;
    totalQuery?: string;
    policy?: SLO["policy"];
    policies?: SLO["policies"];
  };
}
export function definitionFromSLO(slo: SLO): SLODefinition {
  return {
    apiVersion: RESOURCE_API_VERSION,
    kind: "ServiceLevelObjective",
    metadata: { name: slo.id, project: slo.project },
    spec: {
      ...(slo.intent ? { intent: slo.intent } : {}),
      ...(slo.measurement
        ? { measurement: slo.measurement }
        : slo.providerWindow
          ? { providerWindow: slo.providerWindow }
          : {}),
      ...(slo.measurementBinding
        ? { measurementBinding: slo.measurementBinding }
        : {}),
      ...(slo.source ? { source: slo.source } : {}),
      ...(slo.lifecycle ? { lifecycle: slo.lifecycle } : {}),
      ...(slo.quality ? { quality: slo.quality } : {}),
      ...(slo.errorBudgetPolicy
        ? { errorBudgetPolicy: slo.errorBudgetPolicy }
        : {}),
      displayName: slo.name,
      service: slo.service,
      target: slo.target,
      windowSeconds: slo.windowSeconds,
      stepSeconds: slo.stepSeconds,
      ...(slo.goodQuery
        ? { goodQuery: slo.goodQuery, totalQuery: slo.totalQuery }
        : {}),
      ...(slo.policy ? { policy: slo.policy } : {}),
      ...(slo.policies?.length ? { policies: slo.policies } : {}),
    },
  };
}
