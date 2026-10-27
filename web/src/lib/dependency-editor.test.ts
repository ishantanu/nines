import { expect, test } from "vitest";
import { stringify } from "yaml";
import {
  connectDependency,
  dependencyManifest,
  mergeDependencyDiscovery,
  moveService,
  renameService,
  type DependencyManifest,
} from "./dependency-editor";
import { layoutGraph, mapStarter } from "./topology";
const draft = () => structuredClone(mapStarter) as DependencyManifest;
test("connections and positions survive YAML round trips and keep explicit semantics", () => {
  const d = draft();
  d.spec.dependencies = [];
  const linked = connectDependency(d, "api", "database");
  expect(linked.spec.dependencies).toEqual([
    { caller: "api", callee: "database", critical: false },
  ]);
  expect(() => connectDependency(linked, "api", "api")).toThrow(/itself/);
  expect(() => connectDependency(linked, "api", "database")).toThrow(
    /already exists/,
  );
  expect(() => connectDependency(linked, "api", "foreign")).toThrow(/this map/);
  const graph = layoutGraph(d.spec.services, d.spec.dependencies);
  const moved = moveService(linked, "api", { x: 37, y: 281 }, graph.positions);
  const parsed = dependencyManifest(stringify(moved));
  expect(parsed.spec.services[0].position).toEqual({ x: 40, y: 280 });
  expect(
    layoutGraph(parsed.spec.services, parsed.spec.dependencies).positions.get(
      "api",
    ),
  ).toEqual({ x: 40, y: 280 });
  expect(() =>
    moveService(
      linked,
      "api",
      graph.positions.get("database")!,
      graph.positions,
    ),
  ).toThrow(/overlap/);
  expect(
    renameService(linked, "api", "gateway").spec.dependencies[0].caller,
  ).toBe("gateway");
});
test("discovery merges without losing manual links, SLO references, observation or positions", () => {
  const current = draft();
  current.spec.services[0].sloRefs = ["availability"];
  current.spec.services[0].position = { x: 0, y: 80 };
  current.spec.observed = { source: "primary", labels: {}, windowSeconds: 900 };
  const discovered = draft();
  discovered.spec.services.push({
    name: "worker",
    displayName: "Worker",
    sloRefs: [],
  });
  discovered.spec.dependencies[0].critical = false;
  const merged = mergeDependencyDiscovery(current, discovered);
  expect(merged.spec.services[0]).toEqual(current.spec.services[0]);
  expect(merged.spec.dependencies[0].critical).toBe(true);
  expect(merged.spec.observed).toEqual(current.spec.observed);
  expect(merged.spec.services).toHaveLength(3);
  expect(merged.metadata).toEqual(current.metadata);
});
test("manual coordinates cannot hide unpositioned services under an existing node", () => {
  const d = draft();
  d.spec.services[0].position = { x: 36, y: 152 };
  const graph = layoutGraph(d.spec.services, []);
  expect(graph.positions.get("database")).not.toEqual(
    d.spec.services[0].position,
  );
  for (const position of graph.positions.values())
    expect(position.y + 80).toBeLessThanOrEqual(graph.height);
});
