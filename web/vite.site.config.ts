import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { siteNotices } from "./site-notices";
import { siteBrand } from "./site-brand";

// Relative assets work both at /nines/ on Pages and at a custom domain root.
export default defineConfig({
  root: fileURLToPath(new URL("./site", import.meta.url)),
  base: "./",
  plugins: [react(), tailwindcss(), siteNotices(), siteBrand()],
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
