import { hydrateRoot } from "react-dom/client";
import { DocsLayout, type DocumentationData } from "./DocsLayout";
export function mountDocumentation(root: HTMLElement) {
  const payload = document.getElementById("docs-page-data");
  const content = document.getElementById("doc-content");
  if (!payload?.textContent || !content)
    throw new Error("Missing prerendered documentation");
  const data = JSON.parse(payload.textContent) as DocumentationData;
  // This HTML was rendered from allowlisted repository Markdown by React with
  // raw HTML disabled. Never pass fetched or user-supplied HTML into this view.
  hydrateRoot(root, <DocsLayout {...data} content={content.innerHTML} />);
}
