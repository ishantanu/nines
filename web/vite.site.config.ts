import { fileURLToPath, URL } from "node:url";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { siteNotices } from "./site-notices";
import { siteBrand } from "./site-brand";
import { siteDocs } from "./site-docs";

// Budget the actual emitted bootstrap plus every static docs dependency. Dynamic
// landing-page imports are excluded because documentation never requests them.
function documentationBudget(): Plugin {
  return {
    name: "documentation-script-budget",
    generateBundle(_options, bundle) {
      const chunks = Object.values(bundle).filter(
        (item) => item.type === "chunk",
      );
      const docs = chunks.find((item) =>
        item.facadeModuleId?.endsWith("/site/docs/main.tsx"),
      );
      if (!docs) this.error("Missing independent documentation entry");
      const files = new Set<string>();
      const visit = (name: string) => {
        if (files.has(name)) return;
        const item = bundle[name];
        if (item?.type !== "chunk") return;
        files.add(name);
        item.imports.forEach(visit);
      };
      chunks
        .filter((item) => item.isEntry || item === docs)
        .forEach((item) => visit(item.fileName));
      const bytes = [...files].reduce((total, name) => {
        const item = bundle[name];
        return (
          total + (item.type === "chunk" ? Buffer.byteLength(item.code) : 0)
        );
      }, 0);
      if (bytes > 16_000)
        this.error(`Documentation scripts exceed 16 KB: ${bytes} bytes`);
      this.info(`Documentation scripts: ${bytes} bytes (16 KB budget)`);
    },
  };
}

// Relative assets work both at /nines/ on Pages and at a custom domain root.
export default defineConfig({
  root: fileURLToPath(new URL("./site", import.meta.url)),
  base: "./",
  plugins: [
    react(),
    tailwindcss(),
    siteNotices(),
    siteBrand(),
    siteDocs(),
    documentationBudget(),
  ],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    host: "127.0.0.1",
    fs: { allow: [fileURLToPath(new URL("..", import.meta.url))] },
  },
  build: {
    outDir: fileURLToPath(new URL("../dist/site", import.meta.url)),
    emptyOutDir: true,
    assetsInlineLimit: 0,
  },
});
