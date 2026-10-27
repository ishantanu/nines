import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";

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
        if (!files.length)
          throw new Error(
            `Missing site runtime notice: ${metadata.name}@${metadata.version}`,
          );
        records.push({
          name: metadata.name,
          version: metadata.version,
          notice: (
            await Promise.all(
              files.map((file) => readFile(path.join(directory, file), "utf8")),
            )
          ).join("\n"),
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
