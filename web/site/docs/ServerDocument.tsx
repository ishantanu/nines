// Build/dev-server only. Markdown and guide contents never enter browser bundles.
import { readFile, realpath } from "node:fs/promises";
import path from "node:path";
import { isValidElement, type ReactNode } from "react";
import { renderToString } from "react-dom/server";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSlug from "rehype-slug";
import { Button } from "../../src/components/ui/button";
import { DocsLayout, type DocumentationData, type Heading } from "./DocsLayout";
import { resolveDocumentLink } from "./links";
import { rootForPage, type DocPage } from "./navigation";
function headingText(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(headingText).join("");
  if (isValidElement<{ children?: ReactNode; alt?: string }>(node))
    return node.props.alt ?? headingText(node.props.children);
  return "";
}
export function renderDocumentation(page: DocPage, markdown: string) {
  const root = rootForPage(page);
  const headings: Heading[] = [];
  function heading(level: 2 | 3, id: string | undefined, children: ReactNode) {
    if (id) headings.push({ level, id, text: headingText(children) });
    return level === 2 ? (
      <h2 id={id}>{children}</h2>
    ) : (
      <h3 id={id}>{children}</h3>
    );
  }
  const content = renderToString(
    <Markdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeSlug]}
      skipHtml
      components={{
        h2: ({ id, children }) => heading(2, id, children),
        h3: ({ id, children }) => heading(3, id, children),
        a: ({ href, children }) => {
          const target = resolveDocumentLink(href ?? "", page.source, root);
          return (
            <a
              href={target}
              download={target.startsWith(root + "examples/") ? "" : undefined}
            >
              {children}
            </a>
          );
        },
        img: ({ src, alt }) => (
          <img
            src={resolveDocumentLink(
              typeof src === "string" ? src : "",
              page.source,
              root,
            )}
            alt={alt ?? ""}
            loading="lazy"
          />
        ),
        table: ({ children }) => (
          <div className="docs-table">
            <table>{children}</table>
          </div>
        ),
        pre: ({ children }) => {
          const code = isValidElement<{
            className?: string;
            children?: ReactNode;
          }>(children)
            ? children
            : null;
          const language =
            code?.props.className?.replace(/^language-/, "") ?? "text";
          return (
            <div className="docs-code-block">
              <div className="docs-code-toolbar">
                <span>{language}</span>
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  data-copy-code
                  aria-label={`Copy ${language} code`}
                >
                  Copy
                </Button>
              </div>
              <pre tabIndex={0}>{children}</pre>
            </div>
          );
        },
      }}
    >
      {markdown}
    </Markdown>,
  );
  const data: DocumentationData = { page, root, headings };
  return {
    markup: renderToString(<DocsLayout {...data} content={content} />),
    data,
  };
}
function attribute(text: string) {
  return text
    .replace(/&/g, "&amp;")
    .replace(/"/g, "&quot;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}
export function documentationHTML(
  template: string,
  page: DocPage,
  markdown: string,
) {
  const { markup, data } = renderDocumentation(page, markdown);
  const title = attribute(`${page.title} | Nines documentation`);
  const description = attribute(page.description);
  return template
    .replace(/(?:src|href)="\.\//g, (match) => match.replace("./", data.root))
    .replace(/<title>[\s\S]*?<\/title>/, `<title>${title}</title>`)
    .replace(
      /<link rel="canonical" href="[^"]*"\s*\/>/,
      `<link rel="canonical" href="https://ishantanu.github.io/nines/docs/${attribute(page.slug ? page.slug + "/" : "")}" />`,
    )
    .replace(
      /(<meta\s+(?:name="description"|property="og:description")\s+content=")[^"]*("\s*\/?>)/g,
      `$1${description}$2`,
    )
    .replace(
      /(<meta\s+property="og:title"\s+content=")[^"]*("\s*\/?>)/g,
      `$1${title}$2`,
    )
    .replace(
      '<div id="root"></div>',
      () => `<div id="root" data-doc="${attribute(page.slug)}">${markup}</div>`,
    );
}

export async function loadDocumentationSource(root: string, page: DocPage) {
  const source = await readFile(path.join(root, page.source), "utf8");
  const matches = [...source.matchAll(/^<!-- example: ([^\r\n]+) -->$/gm)];
  let result = source;
  for (const match of matches) {
    const name = match[1];
    if (!/^examples\/[a-zA-Z0-9_./-]+\.(yaml|yml|go)$/.test(name))
      throw new Error("Unsupported documentation include");
    const actual = await realpath(path.resolve(root, name));
    const examples = await realpath(path.join(root, "examples"));
    if (!actual.startsWith(examples + path.sep))
      throw new Error("Documentation include escapes examples");
    const language = name.endsWith(".go") ? "go" : "yaml";
    result = result.replace(
      match[0],
      `\n\`\`\`${language}\n${(await readFile(actual, "utf8")).trimEnd()}\n\`\`\`\n`,
    );
  }
  return result;
}
