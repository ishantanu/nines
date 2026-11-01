import { fileURLToPath } from "node:url";
import { readFile, realpath } from "node:fs/promises";
import path from "node:path";
import type { Plugin } from "vite";
import { docPages, pageForPath } from "./site/docs/navigation";
export function siteDocs(): Plugin {
  return {
    name: "nines-documentation",
    configureServer(server) {
      server.middlewares.use(async (request, response, next) => {
        const pathname = (request.url ?? "").split("?")[0];
        if (pathname === "/schemas/remote-mcp/v1alpha1.json") {
          try {
            response.setHeader(
              "Content-Type",
              "application/schema+json; charset=utf-8",
            );
            response.setHeader("X-Content-Type-Options", "nosniff");
            response.end(
              await readFile(
                new URL("../api/mcp/v1alpha1.schema.json", import.meta.url),
              ),
            );
          } catch (error) {
            next(error);
          }
          return;
        }
        if (pathname === "/openapi/v1alpha1.yaml") {
          try {
            response.setHeader(
              "Content-Type",
              "application/yaml; charset=utf-8",
            );
            response.setHeader("X-Content-Type-Options", "nosniff");
            response.end(
              await readFile(
                new URL("../api/openapi/v1alpha1.yaml", import.meta.url),
              ),
            );
          } catch (error) {
            next(error);
          }
          return;
        }
        if (
          pathname.startsWith("/examples/") ||
          pathname.startsWith("/docs/assets/")
        ) {
          const example = /^\/examples\/[a-zA-Z0-9_./-]+\.(yaml|yml|go)$/.test(
            pathname,
          );
          const artwork =
            /^\/docs\/assets\/nines(?:-architecture|-dark|-mark)?\.svg$/.test(
              pathname,
            );
          if (!example && !artwork) {
            response.statusCode = 404;
            response.end("Not found");
            return;
          }
          try {
            const base = fileURLToPath(
              new URL(
                example ? "../examples/" : "../docs/public/brand/",
                import.meta.url,
              ),
            );
            const candidate = path.join(
              base,
              example
                ? pathname.slice("/examples/".length)
                : path.basename(pathname),
            );
            const actual = await realpath(candidate);
            if (!actual.startsWith((await realpath(base)) + path.sep))
              throw new Error("Outside public assets");
            response.setHeader(
              "Content-Type",
              artwork ? "image/svg+xml" : "text/plain; charset=utf-8",
            );
            response.setHeader("X-Content-Type-Options", "nosniff");
            response.end(await readFile(actual));
          } catch {
            response.statusCode = 404;
            response.end("Not found");
          }
          return;
        }
        if (pathname === "/docs/search.json") {
          try {
            const entries = await Promise.all(
              docPages.map(async (page) => ({
                slug: page.slug,
                title: page.title,
                text: await readFile(
                  new URL(`../${page.source}`, import.meta.url),
                  "utf8",
                ),
              })),
            );
            response.setHeader("Content-Type", "application/json");
            response.end(JSON.stringify(entries));
          } catch (error) {
            next(error);
          }
          return;
        }
        if (pathname.endsWith("/index.md")) {
          const page = pageForPath(pathname.slice(0, -"index.md".length));
          if (page) {
            try {
              const renderer = await server.ssrLoadModule("/render.tsx");
              const markdown = await renderer.loadDocumentationSource(
                fileURLToPath(new URL("..", import.meta.url)),
                page,
              );
              response.setHeader(
                "Content-Type",
                "text/markdown; charset=utf-8",
              );
              response.setHeader("X-Content-Type-Options", "nosniff");
              response.end(markdown);
            } catch (error) {
              next(error);
            }
            return;
          }
        }
        const page = pageForPath(pathname);
        if (!page) return next();
        if (!pathname.endsWith("/") && !pathname.endsWith("index.html")) {
          response.writeHead(302, { Location: pathname + "/" });
          response.end();
          return;
        }
        try {
          const [template, renderer] = await Promise.all([
            readFile(new URL("./site/index.html", import.meta.url), "utf8"),
            server.ssrLoadModule("/render.tsx"),
          ]);
          const markdown = await renderer.loadDocumentationSource(
            fileURLToPath(new URL("..", import.meta.url)),
            page,
          );
          const html = renderer.documentationHTML(
            template,
            page,
            markdown,
          ) as string;
          response.setHeader("Content-Type", "text/html; charset=utf-8");
          response.end(await server.transformIndexHtml(pathname, html));
        } catch (error) {
          server.ssrFixStacktrace(error as Error);
          next(error);
        }
      });
    },
  };
}
