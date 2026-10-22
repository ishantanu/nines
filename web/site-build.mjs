import { build } from "esbuild";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// Ship readable HTML to crawlers and readers without JavaScript. Hydration only
// adds the shadcn tabs and clipboard interaction; no server runtime is required.
const web = fileURLToPath(new URL(".", import.meta.url));
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
        name: "raw-definition-examples",
        setup(bundler) {
          bundler.onResolve({ filter: /\.yaml\?raw$/ }, (args) => ({
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
  const { renderSite } = await import(pathToFileURL(output).href);
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
  console.log("Product site prerendered to static HTML.");
} finally {
  await rm(temporary, { recursive: true, force: true });
}
