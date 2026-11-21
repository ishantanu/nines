// Generated from internal/maturity by its tests; do not edit maturity.json.
import catalog from "./maturity.json";

export type MaturityLevel = "stable" | "beta" | "experimental";
export type MaturityFeature = {
  id: string;
  name: string;
  level: MaturityLevel;
  docs: string;
  page?: string;
  provider?: string;
  remaining?: string;
  gated?: boolean;
};

export const maturityFeatures = catalog.features as MaturityFeature[];
export const maturityLevels = catalog.levels as {
  level: MaturityLevel;
  name: string;
  promise: string;
}[];

const docsBase = "https://ishantanu.github.io/nines/docs/";

export function maturityDocsURL(feature?: MaturityFeature) {
  return feature?.level === "experimental"
    ? `${docsBase}maturity/#experimental`
    : `${docsBase}maturity/`;
}

export function pageMaturity(page: string) {
  return maturityFeatures.find((f) => f.page === page);
}

export function providerMaturity(kind: string) {
  return maturityFeatures.find((f) => f.provider === kind);
}

export function maturityPromise(level: MaturityLevel) {
  return maturityLevels.find((l) => l.level === level)?.promise ?? "";
}

/** Whether a feature is on, given the gated features a server turned on. */
export function featureOn(enabled: readonly string[] | undefined, id: string) {
  const feature = maturityFeatures.find((f) => f.id === id);
  if (!feature?.gated) return true;
  return enabled?.includes(id) ?? false;
}

export function pageOn(enabled: readonly string[] | undefined, page: string) {
  const feature = pageMaturity(page);
  return !feature || featureOn(enabled, feature.id);
}

export function providerOn(
  enabled: readonly string[] | undefined,
  kind: string,
) {
  const feature = providerMaturity(kind);
  return !feature || featureOn(enabled, feature.id);
}
