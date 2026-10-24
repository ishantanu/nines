import { documentationLink, repository } from "./navigation";
const origin = "https://documentation.invalid";
export function resolveDocumentLink(
  href: string,
  source: string,
  root: string,
): string {
  if (!href || href.startsWith("#")) return href;
  let target: URL;
  try {
    target = new URL(href, `${origin}/${source}`);
  } catch {
    return "";
  }
  if (!["http:", "https:", "mailto:"].includes(target.protocol)) return "";
  if (target.origin !== origin) {
    const repoDocs = `${repository}/blob/main/docs/`;
    if (href.startsWith(repoDocs)) {
      target = new URL(
        href.slice(`${repository}/blob/main/`.length),
        `${origin}/`,
      );
    } else return href;
  }
  const path = decodeURIComponent(target.pathname).replace(/^\//, "");
  const suffix = target.search + target.hash;
  if (path.startsWith("docs/") && path.endsWith(".md"))
    return documentationLink(path, root) + suffix;
  if (path.startsWith("docs/public/brand/"))
    return `${root}docs/assets/${path.slice("docs/public/brand/".length)}${suffix}`;
  if (path.startsWith("examples/")) return `${root}${path}${suffix}`;
  return `${repository}/${path.endsWith("/") ? "tree" : "blob"}/main/${path}${suffix}`;
}
