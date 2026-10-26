import { expect, it } from "vitest";
import { build } from "esbuild";
import { fileURLToPath } from "node:url";

it("keeps documentation enhancements below 12 KB without React or any runtime dependency", async () => {
  const result = await build({
    entryPoints: [fileURLToPath(new URL("./main.tsx", import.meta.url))],
    bundle: true,
    write: false,
    minify: true,
    format: "esm",
    platform: "browser",
    target: "es2022",
    metafile: true,
  });
  expect(result.outputFiles[0].contents.byteLength).toBeLessThan(12_000);
  expect(
    Object.keys(result.metafile.inputs).some((name) =>
      name.includes("node_modules"),
    ),
  ).toBe(false);
});
