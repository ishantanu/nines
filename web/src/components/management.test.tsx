import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { createTenantSession } from "@/lib/api";
import { PolicyEditor } from "./policy-editor";
import { Users } from "./users";
import { DataSourceForm } from "./data-source-form";
import { definitionFromSLO } from "@/lib/definitions";
const scope = createTenantSession("fixture-key", "alpha", "owner");
test("severity policy editor preserves independent windows and identities", () => {
  const policies = [
    {
      name: "fast",
      severity: "critical" as const,
      shortWindowSeconds: 60,
      longWindowSeconds: 300,
      burnRate: 14.4,
      forSeconds: 60,
      minCoverage: 1,
    },
    {
      name: "slow",
      severity: "warning" as const,
      shortWindowSeconds: 300,
      longWindowSeconds: 3600,
      burnRate: 6,
      forSeconds: 300,
      minCoverage: 1,
    },
  ];
  const html = renderToStaticMarkup(
    <PolicyEditor policies={policies} onChange={() => {}} />,
  );
  expect(html).toContain('value="fast"');
  expect(html).toContain('value="slow"');
  expect(html).toContain("Add alert policy");
  const definition = definitionFromSLO({
    id: "api",
    name: "API",
    project: "p",
    service: "api",
    target: 0.99,
    windowSeconds: 3600,
    stepSeconds: 60,
    policies,
  });
  expect(definition.spec.policies).toEqual(policies);
  expect(definition.spec.policy).toBeUndefined();
});
test("users view escapes identities and keeps external authentication distinct", () => {
  const html = renderToStaticMarkup(
    <Users
      scope={scope}
      providers={[{ id: "local", name: "Local", type: "local" }]}
      members={[
        {
          userId: "u-1",
          role: "viewer",
          effectiveRole: "viewer",
          version: 1,
          user: {
            id: "u-1",
            provider: "local",
            subject: "alice",
            name: "<script>untrusted</script>",
            version: 1,
          },
        },
      ]}
      onRole={() => {}}
      onChange={() => {}}
    />,
  );
  expect(html).toContain("Add user");
  expect(html).toContain("Edit user");
  expect(html).toContain("Remove access");
  expect(html).toContain("&lt;script&gt;");
  expect(html).not.toContain("<script>");
});
test("source management is explicitly disabled until the server enables it", () => {
  const html = renderToStaticMarkup(
    <DataSourceForm scope={scope} enabled={false} onSaved={() => {}} />,
  );
  expect(html).toContain("Add data source");
  expect(html).toContain("disabled");
});
