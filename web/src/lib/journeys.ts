import { parse } from "yaml";
import { RESOURCE_API_VERSION } from "./definitions";
import type { Manifest } from "./api";

export const journeyDimensions = [
  "availability",
  "latency",
  "correctness",
  "safety",
  "cost",
] as const;
export function newJourney(): Manifest {
  return {
    apiVersion: RESOURCE_API_VERSION,
    kind: "CriticalUserJourney",
    metadata: { name: "", project: "production" },
    spec: {
      displayName: "",
      description: "",
      owner: "",
      criticality: "high",
      slos: [],
      steps: [{ id: "step-1", name: "", actor: "user", sloRefs: [] }],
    },
  };
}
// Reject shapes the form cannot represent. Semantic validation stays server-owned.
export function parseJourney(text: string): Manifest {
  const m = parse(text) as Manifest;
  if (
    !m ||
    m.apiVersion !== RESOURCE_API_VERSION ||
    m.kind !== "CriticalUserJourney"
  )
    throw Error(
      `Expected apiVersion ${RESOURCE_API_VERSION} and kind CriticalUserJourney.`,
    );
  if (
    !m.metadata ||
    !m.spec ||
    [
      m.metadata.name,
      m.metadata.project,
      m.spec.displayName,
      m.spec.description,
      m.spec.owner,
      m.spec.criticality,
    ].some((v) => typeof v !== "string") ||
    !Array.isArray(m.spec.slos) ||
    !Array.isArray(m.spec.steps) ||
    m.spec.slos.some(
      (l) =>
        !l ||
        typeof l.id !== "string" ||
        typeof l.dimension !== "string" ||
        typeof l.required !== "boolean",
    ) ||
    m.spec.steps.some(
      (s) =>
        !s ||
        typeof s.id !== "string" ||
        typeof s.name !== "string" ||
        typeof s.actor !== "string" ||
        !Array.isArray(s.sloRefs) ||
        s.sloRefs.some((v) => typeof v !== "string") ||
        (s.dependsOn !== undefined &&
          (!Array.isArray(s.dependsOn) ||
            s.dependsOn.some((v) => typeof v !== "string"))),
    )
  )
    throw Error(
      "The journey needs metadata, details, SLO links and steps in the expected shape. Correct the YAML before opening the form.",
    );
  const a = m.spec.agentic;
  if (
    a !== undefined &&
    a !== null &&
    (typeof a !== "object" ||
      typeof a.successDefinition !== "string" ||
      typeof a.approvalRequired !== "boolean" ||
      [a.maxDurationSeconds, a.maxCostUSD, a.maxSteps].some(
        (v) =>
          v !== undefined && (typeof v !== "number" || !Number.isFinite(v)),
      ))
  )
    throw Error(
      "Correct the agent contract fields in YAML before opening the form.",
    );
  return m;
}
export function removeJourneyLink(m: Manifest, id: string): Manifest {
  return {
    ...m,
    spec: {
      ...m.spec,
      slos: m.spec.slos.filter((l) => l.id !== id),
      steps: m.spec.steps.map((s) => ({
        ...s,
        sloRefs: s.sloRefs.filter((ref) => ref !== id),
      })),
    },
  };
}
export function renameJourneyStep(
  m: Manifest,
  index: number,
  id: string,
): Manifest {
  const old = m.spec.steps[index].id;
  // Duplicate identifiers are left to validation; never rewrite ambiguous edges.
  const unique =
    old !== id &&
    !m.spec.steps.some((s) => s.id === id) &&
    m.spec.steps.filter((s) => s.id === old).length === 1;
  return {
    ...m,
    spec: {
      ...m.spec,
      steps: m.spec.steps.map((s, i) => ({
        ...s,
        ...(i === index ? { id } : {}),
        ...(unique && s.dependsOn
          ? { dependsOn: s.dependsOn.map((dep) => (dep === old ? id : dep)) }
          : {}),
      })),
    },
  };
}
export function removeJourneyStep(m: Manifest, index: number): Manifest {
  const id = m.spec.steps[index].id;
  return {
    ...m,
    spec: {
      ...m.spec,
      steps: m.spec.steps
        .filter((_, i) => i !== index)
        .map((s) => ({
          ...s,
          dependsOn: s.dependsOn?.filter((dep) => dep !== id),
        })),
    },
  };
}
