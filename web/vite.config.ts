import { readFile } from "node:fs/promises";
import { fileURLToPath, URL } from "node:url";
import { defineConfig, loadEnv } from "vite";
import { build as bundleRecorder } from "esbuild";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { siteBrand } from "./site-brand";
export default defineConfig(({ mode, command }) => {
  const env = loadEnv(mode, process.cwd(), "VITE_");
  return {
    // Unset and explicitly empty opt-in telemetry settings must build identically.
    define:
      command === "build"
        ? Object.fromEntries(
            [
              "VITE_POSTHOG_PROJECT_TOKEN",
              "VITE_POSTHOG_HOST",
              "VITE_OTEL_EXPORTER_OTLP_ENDPOINT",
            ].map((key) => [
              `import.meta.env.${key}`,
              JSON.stringify(env[key] ?? ""),
            ]),
          )
        : {},
    plugins: [
      react(),
      tailwindcss(),
      siteBrand(),
      {
        name: "browser-telemetry-policy",
        async generateBundle() {
          const sdk = await bundleRecorder({
            entryPoints: [
              fileURLToPath(
                new URL("./src/replay-browser.ts", import.meta.url),
              ),
            ],
            bundle: true,
            write: false,
            format: "iife",
            globalName: "NinesReplay",
            platform: "browser",
            target: "es2022",
            minify: true,
            logLevel: "silent",
          });
          this.emitFile({
            type: "asset",
            fileName: "replay-recorder.js",
            source: sdk.outputFiles[0].text,
          });
          const player = await bundleRecorder({
            entryPoints: [
              fileURLToPath(new URL("./src/replay-player.ts", import.meta.url)),
            ],
            bundle: true,
            write: false,
            format: "iife",
            platform: "browser",
            target: "es2022",
            minify: true,
            plugins: [
              {
                name: "inline-css",
                setup(build) {
                  build.onResolve(
                    { filter: /\.css\?inline$/ },
                    async (args) => {
                      const result = await build.resolve(
                        args.path.replace(/\?inline$/, ""),
                        { resolveDir: args.resolveDir, kind: args.kind },
                      );
                      return { path: result.path, namespace: "inline-css" };
                    },
                  );
                  build.onLoad(
                    { filter: /.*/, namespace: "inline-css" },
                    async (args) => ({
                      contents: await readFile(args.path, "utf8"),
                      loader: "text",
                    }),
                  );
                },
              },
            ],
          });
          this.emitFile({
            type: "asset",
            fileName: "replay-player-offline.js",
            source: player.outputFiles[0].text,
          });
          this.emitFile({
            type: "asset",
            fileName: "telemetry.json",
            source: JSON.stringify({
              endpoint: env.VITE_OTEL_EXPORTER_OTLP_ENDPOINT ?? "",
              replayHost: env.VITE_POSTHOG_PROJECT_TOKEN?.trim()
                ? (env.VITE_POSTHOG_HOST?.trim() ?? "")
                : "",
            }),
          });
        },
      },
    ],
    resolve: {
      alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
    },
    server: {
      proxy: {
        "/api": "http://127.0.0.1:8189",
        "/auth": "http://127.0.0.1:8189",
        "/healthz": "http://127.0.0.1:8189",
      },
    },
    build: {
      outDir: "../internal/httpapi/web",
      emptyOutDir: true,
      rollupOptions: {
        input: {
          main: fileURLToPath(new URL("./index.html", import.meta.url)),
          replay: fileURLToPath(
            new URL("./replay-player.html", import.meta.url),
          ),
        },
      },
    },
  };
});
