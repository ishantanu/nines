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
