import { RESOURCE_API_VERSION } from "./definitions";
export const bindingKind = "MeasurementBinding";
export const mapKind = "ServiceDependencyMap";
export interface TopologyItem {
  kind: string;
  id: string;
  displayName: string;
  revision: number;
  digest: string;
  appliedAt: string;
}
export interface TopologyCatalog {
  items: TopologyItem[];
  nextCursor?: string;
}
export interface ObjectiveLink {
  id: string;
  displayName: string;
  revision: number;
  health: string;
  reliability: number | null;
  target: number;
  dataStatus: string;
  freshness: string;
  collectionState: string;
}
export interface DependencyNode {
  potentialImpact: { id: string; displayName: string; revision: number }[];
  impactTruncated: boolean;
  name: string;
  displayName: string;
  declared: boolean;
  health: string;
  objectives: ObjectiveLink[];
  journeys: { id: string; displayName: string; revision: number }[];
  journeyLinksTruncated: boolean;
}
export interface DependencyEdge {
  caller: string;
  callee: string;
  provenance: "declared" | "observed";
  critical: boolean;
  description?: string;
  connectionType?: string;
  requests?: number;
}
export interface DependencyView {
  id: string;
  revision: number;
  observedAt: string;
  nodes: DependencyNode[];
  edges: DependencyEdge[];
  observationState: string;
  source?: string;
  from?: string;
  to?: string;
  truncated: boolean;
  warnings: string[];
}
export interface Discovery {
  metric: string;
  metadata: { type: string; unit: string; help: string }[];
  series: Record<string, string>[];
  from: string;
  to: string;
  truncated: boolean;
  fingerprint: string;
  warnings: string[];
}
export interface BindingManifest {
  apiVersion: string;
  kind: string;
  metadata: { name: string; project: string };
  spec: {
    displayName: string;
    service: string;
    source: string;
    measurement: "availability" | "latency";
    countFrom?: "counter" | "histogram";
    metric: string;
    labels: Record<string, string>;
    successLabels?: Record<string, string>;
    thresholdSeconds?: number;
    scrapeIntervalSeconds?: number;
    stepSeconds: number;
    observationPoint: string;
  };
}
export function bindingStarter(
  source = "primary",
  metric = "http_requests_total",
  labels: Record<string, string> = {},
): BindingManifest {
  return {
    apiVersion: RESOURCE_API_VERSION,
    kind: bindingKind,
    metadata: { name: "request-success", project: "production" },
    spec: {
      displayName: "Successful requests",
      service: "api",
      source,
      measurement: "availability",
      metric,
      labels,
      successLabels: { status: "200" },
      stepSeconds: 60,
      scrapeIntervalSeconds: 15,
      observationPoint: "Server requests completed at the API boundary",
    },
  };
}
export const mapStarter = {
  apiVersion: RESOURCE_API_VERSION,
  kind: mapKind,
  metadata: { name: "service-map", project: "production" },
  spec: {
    displayName: "Service dependencies",
    services: [
      { name: "api", displayName: "API", sloRefs: [] },
      { name: "database", displayName: "Database", sloRefs: [] },
    ],
    dependencies: [
      {
        caller: "api",
        callee: "database",
        critical: true,
        description: "Reads and writes",
      },
    ],
  },
};
// Fixed layers keep cycles and disconnected services visible without treating the graph as a DAG.
export function layoutGraph(nodes: DependencyNode[], edges: DependencyEdge[]) {
  const sorted = [...nodes].sort((a, b) => a.name.localeCompare(b.name));
  const depth = new Map<string, number>();
  const targets = new Set(edges.map((e) => e.callee));
  const walk = (root: string) => {
    const queue = [root];
    depth.set(root, 0);
    for (let i = 0; i < queue.length; i++) {
      const name = queue[i];
      for (const e of edges) {
        if (e.caller === name && !depth.has(e.callee)) {
          depth.set(e.callee, Math.min(5, (depth.get(name) ?? 0) + 1));
          queue.push(e.callee);
        }
      }
    }
  };
  for (const n of sorted)
    if (!targets.has(n.name) && !depth.has(n.name)) walk(n.name);
  for (const n of sorted) if (!depth.has(n.name)) walk(n.name);
  const rows = new Map<number, number>();
  const positions = new Map<string, { x: number; y: number }>();
  for (const n of sorted) {
    const col = depth.get(n.name) ?? 0;
    const row = rows.get(col) ?? 0;
    rows.set(col, row + 1);
    positions.set(n.name, { x: 36 + col * 252, y: 36 + row * 116 });
  }
  return {
    positions,
    width: Math.max(720, Math.max(0, ...depth.values()) * 252 + 284),
    height: Math.max(420, Math.max(0, ...rows.values()) * 116 + 72),
  };
}
