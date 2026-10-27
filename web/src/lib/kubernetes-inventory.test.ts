import { readFileSync } from "node:fs";
import { expect, test } from "vitest";
import { projectKubernetes, kubernetesHost } from "./kubernetes-inventory";
const fixture = readFileSync(
  new URL(
    "../../../internal/kubediscovery/testdata/resources.yaml",
    import.meta.url,
  ),
  "utf8",
);
test("browser projection matches the native inventory contract and excludes private workload fields", () => {
  const input = fixture.replace(
    "postgres://database",
    "postgres://fixture-user:private-password@database",
  );
  const inventory = projectKubernetes(input, "default");
  expect(inventory.resources).toHaveLength(4);
  expect(inventory.ignoredResources).toBe(1);
  expect(inventory.unresolvedReferences).toBe(1);
  const workload = inventory.resources.find((r) => r.kind === "Deployment")!;
  expect(workload.hosts).toEqual(["database.commerce.svc.cluster.local"]);
  const serialized = JSON.stringify(inventory);
  for (const secret of [
    "fixture-user",
    "private-password",
    "private-cache-reference",
    "postgres://",
    "DEBUG_TOKEN",
    "not-for-discovery",
    "example.invalid",
    "checkout",
  ])
    expect(serialized).not.toContain(secret);
});
test("unsafe dependency annotations and aliases cannot enter transmitted inventory", () => {
  const bad = fixture.replace(
    "annotations: {nines.dev/service: api}",
    "annotations: {nines.dev/service: 'https://user:secret@example.invalid', nines.dev/depends-on: 'https://user:secret@example.invalid'}",
  );
  expect(JSON.stringify(projectKubernetes(bad, "default"))).not.toContain(
    "user:secret",
  );
  for (const yaml of [
    "kind: Service\nmetadata: &m {name: api}\nother: *m",
    "kind: Service\nkind: Service\nmetadata: {name: api}",
    " ".repeat((2 << 20) + 1),
  ])
    expect(() => projectKubernetes(yaml, "default")).toThrow();
});
test("host extraction removes URL credentials, paths and ports without resolving variables", () => {
  expect(
    kubernetesHost(
      "postgres://user:pass@database.commerce.svc.cluster.local:5432/private?token=private",
    ),
  ).toBe("database.commerce.svc.cluster.local");
  expect(kubernetesHost("database:5432")).toBe("database");
  expect(kubernetesHost("$(DATABASE_HOST)")).toBe("");
  expect(kubernetesHost("database:5432?token=private")).toBe("");
});

test("typed Kubernetes lists supply item kinds omitted by API serialization", () => {
  const inventory = projectKubernetes(
    JSON.stringify({
      kind: "ServiceList",
      items: [
        {
          metadata: { name: "api", namespace: "commerce" },
          spec: { selector: { app: "api" } },
        },
      ],
    }),
    "default",
  );
  expect(inventory.resources[0]).toMatchObject({
    kind: "Service",
    name: "api",
    namespace: "commerce",
  });
});
