import { parse } from "yaml";
import { RESOURCE_API_VERSION } from "./definitions";
import { mapKind, type GraphPosition } from "./topology";
export interface DependencyManifest {
  apiVersion: string;
  kind: string;
  metadata: { name: string; project: string };
  spec: {
    displayName: string;
    services: {
      name: string;
      displayName: string;
      sloRefs: string[];
      position?: GraphPosition;
    }[];
    dependencies: {
      caller: string;
      callee: string;
      critical: boolean;
      description?: string;
      origin?: "manual" | "kubernetes";
    }[];
    observed?: {
      source: string;
      labels: Record<string, string>;
      windowSeconds: number;
      scrapeIntervalSeconds?: number;
      maxSampleAgeSeconds?: number;
    };
  };
}
export function dependencyManifest(text: string): DependencyManifest {
  const d = parse(text, { maxAliasCount: 0 }) as DependencyManifest;
  if (
    d?.apiVersion !== RESOURCE_API_VERSION ||
    d.kind !== mapKind ||
    !d.metadata ||
    !d.spec ||
    !Array.isArray(d.spec.services) ||
    !Array.isArray(d.spec.dependencies) ||
    d.spec.services.length > 40 ||
    d.spec.dependencies.length > 100 ||
    d.spec.services.some(
      (n) =>
        !n ||
        typeof n.name !== "string" ||
        typeof n.displayName !== "string" ||
        !Array.isArray(n.sloRefs) ||
        n.sloRefs.some((s) => typeof s !== "string"),
    )
  )
    throw new Error(
      "Enter a ServiceDependencyMap with up to 40 services and 100 dependencies.",
    );
  const names = new Set(d.spec.services.map((n) => n.name));
  if (
    names.size !== d.spec.services.length ||
    d.spec.dependencies.some(
      (e) =>
        !e ||
        typeof e.caller !== "string" ||
        typeof e.callee !== "string" ||
        !names.has(e.caller) ||
        !names.has(e.callee) ||
        typeof e.critical !== "boolean",
    )
  )
    throw new Error(
      "Dependencies must reference unique services in this map and declare critical as true or false.",
    );
  if (
    d.spec.services.some(
      (n) =>
        n.position &&
        (!Number.isInteger(n.position.x) ||
          !Number.isInteger(n.position.y) ||
          n.position.x < 0 ||
          n.position.y < 0 ||
          n.position.x > 10000 ||
          n.position.y > 10000),
    )
  )
    throw new Error(
      "Service positions must be integer coordinates from 0 to 10000.",
    );
  return d;
}
export function connectDependency(
  d: DependencyManifest,
  caller: string,
  callee: string,
): DependencyManifest {
  if (
    !d.spec.services.some((n) => n.name === caller) ||
    !d.spec.services.some((n) => n.name === callee)
  )
    throw new Error("Choose a caller and dependency from this map.");
  if (caller === callee) throw new Error("A service cannot depend on itself.");
  if (
    d.spec.dependencies.some((e) => e.caller === caller && e.callee === callee)
  )
    throw new Error("This dependency already exists.");
  if (d.spec.dependencies.length >= 100)
    throw new Error("A map supports at most 100 dependencies.");
  return {
    ...d,
    spec: {
      ...d.spec,
      dependencies: [
        ...d.spec.dependencies,
        { caller, callee, critical: false },
      ],
    },
  };
}
export function moveService(
  d: DependencyManifest,
  name: string,
  position: GraphPosition,
  positions: Map<string, GraphPosition>,
): DependencyManifest {
  const next = {
    x: Math.round(Math.min(10000, Math.max(0, position.x)) / 8) * 8,
    y: Math.round(Math.min(10000, Math.max(0, position.y)) / 8) * 8,
  };
  for (const [other, p] of positions)
    if (
      other !== name &&
      Math.abs(p.x - next.x) < 208 &&
      Math.abs(p.y - next.y) < 96
    )
      throw new Error(
        "Choose a position that does not overlap another service.",
      );
  return {
    ...d,
    spec: {
      ...d.spec,
      services: d.spec.services.map((n) => ({
        ...n,
        position: n.name === name ? next : positions.get(n.name),
      })),
    },
  };
}
export function renameService(
  d: DependencyManifest,
  previous: string,
  name: string,
): DependencyManifest {
  if (
    !name.trim() ||
    name.length > 200 ||
    /[\r\n\0]/.test(name) ||
    d.spec.services.some((n) => n.name === name && n.name !== previous)
  )
    throw new Error("Use a unique service name of 1–200 characters.");
  return {
    ...d,
    spec: {
      ...d.spec,
      services: d.spec.services.map((n) =>
        n.name === previous ? { ...n, name } : n,
      ),
      dependencies: d.spec.dependencies.map((e) => ({
        ...e,
        caller: e.caller === previous ? name : e.caller,
        callee: e.callee === previous ? name : e.callee,
      })),
    },
  };
}

export function mergeDependencyDiscovery(
  current: DependencyManifest,
  discovered: DependencyManifest,
): DependencyManifest {
  const services = [
    ...current.spec.services,
    ...discovered.spec.services.filter(
      (n) => !current.spec.services.some((old) => old.name === n.name),
    ),
  ];
  const dependencies = [
    ...current.spec.dependencies,
    ...discovered.spec.dependencies.filter(
      (e) =>
        !current.spec.dependencies.some(
          (old) => old.caller === e.caller && old.callee === e.callee,
        ),
    ),
  ];
  if (services.length > 40 || dependencies.length > 100)
    throw new Error(
      "Discovery would exceed 40 services or 100 dependencies. Narrow the imported inventory.",
    );
  return { ...current, spec: { ...current.spec, services, dependencies } };
}
