import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ProviderIdentity, SLOProvider } from "./provider-identity";
import type { SLO } from "@/lib/api";

const slo = { source: "payments", queryLanguage: "promql" } as SLO;
test("SLO identity uses the configured connection, not its shared query language", () => {
  const html = renderToStaticMarkup(
    <SLOProvider
      slo={slo}
      connections={[
        { id: "primary", kind: "prometheus" },
        { id: "payments", kind: "dash0" },
      ]}
    />,
  );
  expect(html).toContain("Dash0");
  expect(html).toContain("payments");
  expect(html).toContain("<img");
  expect(html).toContain('alt=""');
  expect(html).not.toContain("mask-image");
  expect(html).not.toContain("Prometheus");
  expect(html).not.toContain("Deployed");
});
test("unresolved connections never invent a provider or publication status", () => {
  expect(renderToStaticMarkup(<SLOProvider slo={slo} />)).toContain(
    "Connection: payments",
  );
});
test("provider text survives without images and escapes untrusted connection names", () => {
  for (const kind of [
    "prometheus",
    "mimir",
    "datadog",
    "newrelic",
    "victoriametrics",
    "dash0",
    "googlecloud",
  ]) {
    const html = renderToStaticMarkup(
      <ProviderIdentity kind={kind} connection="<script>" />,
    );
    expect(html).toContain('aria-hidden="true"');
    expect(html).toContain('width="16"');
    expect(html).toContain('height="16"');
    expect(html.includes("dark:invert")).toBe(kind !== "mimir");
    expect(html).toMatch(/src="[^"<>]+\.svg\?no-inline"/);
    expect(html).not.toContain("background");
    expect(html).not.toContain("mask-image");
    expect(html).toContain("&lt;script&gt;");
    expect(html).not.toContain("<script>");
    expect(html).not.toContain("https://");
  }
});

test("Mimir uses its own product symbol and preserves its official colors", () => {
  const html = renderToStaticMarkup(
    <ProviderIdentity kind="mimir" connection="metrics" />,
  );
  expect(html).toContain("/mimir.svg?no-inline");
  expect(html).not.toContain("grafana.svg");
  expect(html).not.toContain("dark:invert");
  expect(html).toContain("Mimir");
});
