import { describe, expect, it } from "vitest";
import {
  layoutGraph,
  type DependencyNode,
  type DependencyEdge,
} from "./topology";
const node = (name: string) =>
  ({
    name,
    displayName: name,
    declared: true,
    health: "unknown",
    objectives: [],
    journeys: [],
    potentialImpact: [],
    impactTruncated: false,
    journeyLinksTruncated: false,
  }) satisfies DependencyNode;
const edge = (caller: string, callee: string) =>
  ({
    caller,
    callee,
    provenance: "declared",
    critical: true,
  }) satisfies DependencyEdge;
describe("dependency graph layout", () => {
  it("keeps cycles, self references and disconnected services visible without overlapping nodes", () => {
    const nodes = [node("a"), node("b"), node("c"), node("d")];
    const graph = layoutGraph(nodes, [
      edge("a", "b"),
      edge("b", "a"),
      edge("c", "c"),
    ]);
    expect(graph.positions.size).toBe(4);
    const positions = [...graph.positions.values()];
    expect(new Set(positions.map((p) => `${p.x},${p.y}`)).size).toBe(4);
    for (const p of positions) {
      expect(p.x + 192).toBeLessThanOrEqual(graph.width);
      expect(p.y + 80).toBeLessThanOrEqual(graph.height);
    }
  });
  it("places dependencies after callers", () => {
    const graph = layoutGraph([node("api"), node("db")], [edge("api", "db")]);
    expect(graph.positions.get("db")!.x).toBeGreaterThan(
      graph.positions.get("api")!.x,
    );
  });
});
