import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";

const assets = new Map([
  [
    "nines-architecture.svg",
    new URL("../docs/public/brand/nines-architecture.svg", import.meta.url),
  ],
  ["nines.svg", new URL("../docs/public/brand/nines.svg", import.meta.url)],
  [
    "nines-dark.svg",
    new URL("../docs/public/brand/nines-dark.svg", import.meta.url),
  ],
  [
    "nines-mark.svg",
    new URL("../docs/public/brand/nines-mark.svg", import.meta.url),
  ],
]);
export function siteBrand(): Plugin {
  return {
    name: "nines-site-brand",
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const asset = assets.get(
          (req.url ?? "").split("?")[0].replace(/^\//, ""),
        );
        if (!asset) return next();
        try {
          res.setHeader("Content-Type", "image/svg+xml");
          res.end(await readFile(fileURLToPath(asset)));
        } catch (error) {
          next(error);
        }
      });
    },
    async generateBundle() {
      for (const [name, url] of assets)
        this.emitFile({
          type: "asset",
          fileName: name,
          source: await readFile(fileURLToPath(url)),
        });
    },
  };
}
