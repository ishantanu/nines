import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";

// Some published packages ship no license file. Their upstream notice is taken
// from the same-repository sibling or the reviewed copy in third_party/licenses
// (see third_party/license-sources.json for provenance).
const fallbackNotices: Record<string, string> = {
  "@astryxdesign/core": "./node_modules/@astryxdesign/theme-neutral/LICENSE",
  "@stylexjs/stylex": "../third_party/licenses/stylex.LICENSE",
  "@stylexjs/shared": "../third_party/licenses/stylex.LICENSE",
  "@stylexjs/babel-plugin": "../third_party/licenses/stylex.LICENSE",
  "flow-estree": "../third_party/licenses/flow.LICENSE",
  "flow-parser": "../third_party/licenses/flow.LICENSE",
  imurmurhash: "../third_party/licenses/imurmurhash.LICENSE",
};

// Preserve notices for packages whose code Rollup actually includes in the site.
export function siteNotices(): Plugin {
  return {
    name: "nines-site-notices",
    async generateBundle(_, bundle) {
      const packages = new Set<string>();
      for (const chunk of Object.values(bundle)) {
        if (chunk.type !== "chunk") continue;
        for (const [id, module] of Object.entries(chunk.modules)) {
          if (!module.renderedLength || !id.includes("/node_modules/"))
            continue;
          let directory = path.dirname(id.split("?")[0]);
          while (directory.includes("/node_modules/")) {
            try {
              await readFile(path.join(directory, "package.json"));
              packages.add(directory);
              break;
            } catch {
              directory = path.dirname(directory);
            }
          }
        }
      }
      const notices: string[] = [
        "Nines product site\nApplication: Apache-2.0. Bundled third-party software retains the following terms.\n",
      ];
      const records: { name: string; version: string; notice: string }[] = [];
      for (const directory of packages) {
        const metadata = JSON.parse(
          await readFile(path.join(directory, "package.json"), "utf8"),
        );
        const files = (await readdir(directory))
          .filter((name) =>
            /^(?:licen[cs]e|notice|copying|copyright)(?:[._-].*)?$/i.test(name),
          )
          .sort();
        const fallback = fallbackNotices[metadata.name];
        if (!files.length && !fallback)
          throw new Error(
            `Missing site runtime notice: ${metadata.name}@${metadata.version}`,
          );
        records.push({
          name: metadata.name,
          version: metadata.version,
          notice: files.length
            ? (
                await Promise.all(
                  files.map((file) =>
                    readFile(path.join(directory, file), "utf8"),
                  ),
                )
              ).join("\n")
            : await readFile(
                fileURLToPath(new URL(fallback, import.meta.url)),
                "utf8",
              ),
        });
      }
      for (const record of records.sort((a, b) =>
        a.name.localeCompare(b.name),
      )) {
        notices.push(
          `\n${"=".repeat(72)}\n${record.name}@${record.version}\n${"=".repeat(72)}\n${record.notice}`,
        );
      }
      notices.push(
        "\nIncorporated shadcn/ui components\n" +
          (await readFile(
            fileURLToPath(
              new URL("./public/shadcn-license.txt", import.meta.url),
            ),
            "utf8",
          )),
      );
      notices.push(
        "\nBeautiful UI Notchset Feature Bento (MIT)\n" +
          (await readFile(
            fileURLToPath(
              new URL("./public/beautiful-ui-license.txt", import.meta.url),
            ),
            "utf8",
          )),
      );
      notices.push(
        "\nStageshow retained-text motion (adapted)\n" +
          (await readFile(
            fileURLToPath(new URL("./site/workflow/LICENSE", import.meta.url)),
            "utf8",
          )),
      );
      notices.push(
        "\nSelf-hosted typefaces\n" +
          (await readFile(
            fileURLToPath(
              new URL(
                "./site/fonts/OFL-Atkinson-Hyperlegible-Next.txt",
                import.meta.url,
              ),
            ),
            "utf8",
          )) +
          "\n" +
          (await readFile(
            fileURLToPath(
              new URL("./site/fonts/OFL-Barlow-Condensed.txt", import.meta.url),
            ),
            "utf8",
          )),
      );
      notices.push(
        "\nFigtree (SIL OFL 1.1)\n" +
          (await readFile(
            fileURLToPath(
              new URL("./site/fonts/OFL-Figtree.txt", import.meta.url),
            ),
            "utf8",
          )),
      );
      notices.push(
        "\nNines application license\n" +
          (await readFile(
            fileURLToPath(new URL("../LICENSE", import.meta.url)),
            "utf8",
          )),
      );
      this.emitFile({
        type: "asset",
        fileName: "third-party-notices.txt",
        source: notices.join("\n"),
      });
      this.emitFile({ type: "asset", fileName: ".nojekyll", source: "" });
    },
  };
}
