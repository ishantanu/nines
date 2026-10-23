import { describe, expect, it } from "vitest";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import {
  documentationLink,
  docPages,
  docPath,
  pageForPath,
  rootForPage,
} from "./navigation";
import { resolveDocumentLink } from "./links";
import { documentationHTML, loadDocumentationSource } from "./ServerDocument";

describe("documentation routes and source links", () => {
  it("keeps every guide unique and reachable at root or a GitHub project subpath", () => {
    expect(new Set(docPages.map((page) => page.slug)).size).toBe(
      docPages.length,
    );
    expect(new Set(docPages.map((page) => page.source)).size).toBe(
      docPages.length,
    );
    for (const page of docPages) {
      expect(pageForPath("/" + docPath(page))).toEqual(page);
      expect(pageForPath("/nines/" + docPath(page))).toEqual(page);
      expect(pageForPath("/nines/" + docPath(page) + "index.html")).toEqual(
        page,
      );
      const destination = new URL(
        rootForPage(page) + "assets/site.js",
        "https://example.test/nines/" + docPath(page),
      );
      expect(destination.pathname).toBe("/nines/assets/site.js");
    }
    expect(pageForPath("/docs/missing/")).toBeUndefined();
    expect(documentationLink("docs/cli.md#scripts-and-llm-skills")).toBe(
      "./docs/cli/#scripts-and-llm-skills",
    );
  });
  it("rewrites guide, anchor, artwork and download links without sending readers to GitHub", () => {
    const source = "docs/public/deployment/identity-and-tenants.md";
    const root = "../../../";
    expect(
      resolveDocumentLink(
        "../measurements.md#cli-api-and-agents",
        source,
        root,
      ),
    ).toBe(root + "docs/measurements/#cli-api-and-agents");
    expect(
      resolveDocumentLink("../cli.md", "docs/public/measurements.md", "../../"),
    ).toBe("../../docs/cli/");
    expect(
      resolveDocumentLink("../../../examples/auth-entra.yaml", source, root),
    ).toBe(root + "examples/auth-entra.yaml");
    expect(
      resolveDocumentLink(
        "brand/nines-architecture.svg",
        "docs/public/architecture.md",
        "../../",
      ),
    ).toBe("../../docs/assets/nines-architecture.svg");
    expect(resolveDocumentLink("#providers", source, root)).toBe("#providers");
    expect(
      resolveDocumentLink(
        "https://github.com/ishantanu/nines/blob/main/docs/public/measurements.md",
        source,
        root,
      ),
    ).toBe(root + "docs/measurements/");
    expect(
      resolveDocumentLink("https://prometheus.io/docs/", source, root),
    ).toBe("https://prometheus.io/docs/");
    expect(resolveDocumentLink("javascript:alert(1)", source, root)).toBe("");
    expect(resolveDocumentLink("data:text/html,unsafe", source, root)).toBe("");
  });
});

it("prerenders safe readable guides with anchors, tables and accessible code controls", () => {
  const page = docPages.find((page) => page.slug === "latency-slos")!;
  const template =
    '<html><head><title>Home</title><meta name="description" content="Home"><link href="./assets/site.css"></head><body><div id="root"></div><script src="./assets/site.js"></script></body></html>';
  const markdown =
    '# Latency SLOs\n\n## Ratios\n\n| Good | Total |\n| --- | --- |\n| fast | all |\n\n```promql\ngood / total\n```\n\n[Guide](measurements.md)\n\n<script>alert("unsafe")</script>\n';
  const html = documentationHTML(template, page, markdown);
  expect(html).toContain('<h1 id="latency-slos">Latency SLOs</h1>');
  expect(html).toContain('<h2 id="ratios">Ratios</h2>');
  expect(html).toContain('aria-label="Documentation chapters"');
  expect(html).toContain('aria-label="Copy promql code"');
  expect(html).toContain("<table>");
  expect(html).toContain('href="../../docs/measurements/"');
  expect(html).toContain('src="../../assets/site.js"');
  expect(html).toContain('href="../../assets/site.css"');
  expect(html).toContain('id="docs-page-data" type="application/json"');
  expect(html).not.toContain("<script>alert(");
});

it("includes canonical examples but rejects files outside the public example tree", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "nines-docs-test-"));
  try {
    await mkdir(path.join(root, "docs/public"), { recursive: true });
    await mkdir(path.join(root, "examples"));
    await writeFile(
      path.join(root, "examples/slo.yaml"),
      "kind: ServiceLevelObjective\n",
    );
    const page = { ...docPages[0], source: "docs/public/test.md" };
    await writeFile(
      path.join(root, page.source),
      "# Example\n<!-- example: examples/slo.yaml -->\n",
    );
    expect(await loadDocumentationSource(root, page)).toContain(
      "```yaml\nkind: ServiceLevelObjective\n```",
    );
    await writeFile(path.join(root, "private.yaml"), "private fixture");
    await writeFile(
      path.join(root, page.source),
      "<!-- example: examples/../private.yaml -->\n",
    );
    await expect(loadDocumentationSource(root, page)).rejects.toThrow(
      "escapes examples",
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
