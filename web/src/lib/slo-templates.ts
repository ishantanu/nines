import type { SLODefinition } from "./definitions";
import { bindingStarter } from "./topology";
import {
  measurementDraft,
  filterLabels,
  type LabelFilter,
} from "./measurement";

export interface ExtensionInput {
  field: string;
  type: "string" | "number" | "integer" | "labels";
  description: string;
  required: boolean;
  default?: unknown;
  minimum?: number;
  maximum?: number;
  enum?: string[];
}
export interface ExtensionReference {
  name: string;
  version: string;
  sha256: string;
}
export interface SLOTemplate {
  extension?: ExtensionReference;
  inputs?: Record<string, ExtensionInput>;
  evaluation?: {
    name: string;
    version: string;
    rubric: string;
    sampling: string;
    unknownVerdict: string;
    missingVerdict: string;
    populationLabels?: Record<string, string>;
  };
  id: string;
  revision: number;
  digest: string;
  category: string;
  sli: string;
  title: string;
  description: string;
  eligibleEvents: string;
  goodEvents: string;
  requirements: string[];
  manifest: SLODefinition;
}
export interface SLOTemplateCatalog {
  tenant: string;
  apiVersion: string;
  kind: string;
  digest: string;
  categories: { id: string; title: string }[];
  templates: SLOTemplate[];
}
export function draftFromSLOTemplate(template: SLOTemplate) {
  const { metadata, spec } = template.manifest;
  const m = spec.measurement;
  if (!m) throw new Error("Template must contain an inline measurement.");
  return measurementDraft({
    ...bindingStarter(),
    metadata,
    spec: {
      ...m,
      measurement: m.type,
      displayName: spec.displayName,
      service: spec.service,
      source: spec.source || "primary",
      stepSeconds: spec.stepSeconds,
    },
  });
}

export function extensionLabelRows(value: unknown): LabelFilter[] {
  if (Array.isArray(value)) return value as LabelFilter[];
  if (value && typeof value === "object")
    return Object.entries(value).map(([label, value]) => ({
      label,
      value: String(value),
    }));
  return [];
}
export function extensionRequestInputs(
  template: SLOTemplate,
  values: Record<string, unknown>,
): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const [name, schema] of Object.entries(template.inputs ?? {})) {
    if (!Object.hasOwn(values, name)) continue;
    const value = values[name];
    if (schema.type === "labels")
      result[name] = filterLabels(extensionLabelRows(value));
    else if (schema.type === "number" || schema.type === "integer") {
      if (value === "") throw new Error(`Enter a value for ${name}.`);
      const n = Number(value);
      if (
        !Number.isFinite(n) ||
        (schema.type === "integer" && !Number.isInteger(n))
      )
        throw new Error(`Enter a valid ${schema.type} for ${name}.`);
      result[name] = n;
    } else result[name] = value;
  }
  return result;
}
