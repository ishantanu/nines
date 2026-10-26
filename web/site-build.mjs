import { build } from "esbuild";
import {
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// Ship readable HTML to crawlers and readers without JavaScript. Hydration only
// adds the shadcn tabs and clipboard interaction; no server runtime is required.
const web = fileURLToPath(new URL(".", import.meta.url));
// Public Go examples are downloadable website assets, not packages in this module.
// Keep the generated artifact tree outside recursive Go tooling across rebuilds.
const artifacts = path.resolve(web, "../dist");
await mkdir(artifacts, { recursive: true });
try {
  await writeFile(path.join(artifacts, "go.mod"), "module nines.invalid/build-artifacts\n", { flag: "wx" });
} catch (error) {
  if (error.code !== "EEXIST") throw error;
}
const temporary = await mkdtemp(path.join(tmpdir(), "nines-site-render-"));
try {
  const output = path.join(temporary, "render.cjs");
  await build({
    entryPoints: [path.join(web, "site/render.tsx")],
    bundle: true,
    outfile: output,
    platform: "node",
    format: "cjs",
    jsx: "automatic",
    define: { "process.env.NODE_ENV": '"production"' },
    plugins: [
      {
        name: "raw-source-examples",
        setup(bundler) {
          bundler.onResolve({ filter: /\.(?:yaml|go)\?raw$/ }, (args) => ({
            path: path.resolve(args.resolveDir, args.path.slice(0, -4)),
            namespace: "raw-definition",
          }));
          bundler.onLoad(
            { filter: /.*/, namespace: "raw-definition" },
            async (args) => ({
              contents: `export default ${JSON.stringify(await readFile(args.path, "utf8"))}`,
              loader: "js",
            }),
          );
        },
      },
    ],
  });
  const { renderSite, docPages, documentationHTML, loadDocumentationSource } =
    await import(pathToFileURL(output).href);
  const index = path.join(web, "../dist/site/index.html");
  const html = await readFile(index, "utf8");
  if (!html.includes('<div id="root"></div>'))
    throw new Error("Missing static render mount");
  await writeFile(
    index,
    html.replace(
      '<div id="root"></div>',
      () => `<div id="root">${renderSite()}</div>`,
    ),
  );
  const site = path.dirname(index);
  const root = path.resolve(web, "..");
  const search = [];
  const corpus = [];
  for (const page of docPages) {
    const markdown = await loadDocumentationSource(root, page);
    const destination = path.join(site, "docs", page.slug);
    await mkdir(destination, { recursive: true });
    await writeFile(
      path.join(destination, "index.html"),
      documentationHTML(html, page, markdown),
    );
    search.push({ slug: page.slug, title: page.title, text: markdown });
    corpus.push(`# ${page.title}\nSource: ${page.source}\n\n${markdown}`);
  }
  await writeFile(path.join(site, "docs/search.json"), JSON.stringify(search));
  await writeFile(path.join(site, "llms-full.txt"), corpus.join("\n\n---\n\n"));
  // Only public example definitions and canonical SVG artwork are copied.
  const copyPublicFiles = async (source, destination, extensions) => {
    await mkdir(destination, { recursive: true });
    for (const item of await readdir(source, { withFileTypes: true })) {
      if (item.name.startsWith(".")) continue;
      if (item.isSymbolicLink())
        throw new Error("Refusing documentation asset symlink");
      if (item.isDirectory())
        await copyPublicFiles(
          path.join(source, item.name),
          path.join(destination, item.name),
          extensions,
        );
      else if (item.isFile() && extensions.includes(path.extname(item.name)))
        await writeFile(
          path.join(destination, item.name),
          await readFile(path.join(source, item.name)),
        );
    }
  };
  await copyPublicFiles(
    path.join(root, "examples"),
    path.join(site, "examples"),
    [".yaml", ".yml", ".go"],
  );
  await copyPublicFiles(
    path.join(root, "docs/public/brand"),
    path.join(site, "docs/assets"),
    [".svg"],
  );
  const canonical = "https://ishantanu.github.io/nines/";
  const locations = [
    "",
    ...docPages.map((page) => `docs/${page.slug ? page.slug + "/" : ""}`),
  ];
  await writeFile(
    path.join(site, "sitemap.xml"),
    `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${locations.map((location) => `<url><loc>${canonical}${location}</loc></url>`).join("")}</urlset>`,
  );
  console.log(
    `Product site and ${docPages.length} documentation pages prerendered to static HTML.`,
  );
} finally {
  await rm(temporary, { recursive: true, force: true });
}
