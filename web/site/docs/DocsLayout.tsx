import { ArrowLeft, ArrowRight, ChevronDown, Search } from "lucide-react";
import { Input } from "../../src/components/ui/input";
import { Button } from "../../src/components/ui/button";
import {
  docGroups,
  docPages,
  docPath,
  repository,
  type DocPage,
} from "./navigation";
export interface Heading {
  id: string;
  text: string;
  level: number;
}
export interface DocumentationData {
  page: DocPage;
  root: string;
  headings: Heading[];
}
function Navigation({ page, root }: { page: DocPage; root: string }) {
  return (
    <nav aria-label="Documentation chapters">
      {docGroups.map((group) => (
        <section key={group.title} className="docs-nav-group">
          <h2>{group.title}</h2>
          <ul>
            {group.pages.map((item) => (
              <li key={item.slug}>
                <a
                  href={root + docPath(item)}
                  aria-current={page.slug === item.slug ? "page" : undefined}
                >
                  {item.title}
                </a>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </nav>
  );
}
function DocumentSearch() {
  return (
    <div className="docs-search">
      <label className="sr-only" htmlFor="docs-search">
        Search documentation
      </label>
      <Search size={15} aria-hidden="true" />
      <Input
        id="docs-search"
        type="search"
        placeholder="Search documentation"
        maxLength={200}
        disabled
        aria-describedby="docs-search-help"
      />
      <span id="docs-search-help" className="sr-only">
        Search becomes available when JavaScript loads. All chapters are listed
        in the navigation.
      </span>
      <div id="docs-search-results" className="docs-search-results" hidden>
        <p role="status" />
        <ul />
      </div>
    </div>
  );
}
export function DocsLayout({
  page,
  root,
  headings,
  content,
}: DocumentationData & { content: string }) {
  const group = docGroups.find((section) =>
    section.pages.some((item) => item.slug === page.slug),
  );
  const position = docPages.findIndex((item) => item.slug === page.slug);
  const previous = docPages[position - 1],
    next = docPages[position + 1];
  return (
    <div className="documentation">
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <header className="site-header docs-header">
        <div className="docs-header-inner">
          <a className="brand" href={root} aria-label="Nines home">
            <img src={root + "nines.svg"} width="112" height="32" alt="Nines" />
          </a>
          <a href={root + "docs/"} className="docs-header-label">
            Documentation
          </a>
          <DocumentSearch />
          <Button asChild variant="outline" size="sm">
            <a href={repository}>GitHub</a>
          </Button>
        </div>
      </header>
      <details className="docs-mobile-nav">
        <summary>
          Browse documentation
          <ChevronDown size={16} aria-hidden="true" />
        </summary>
        <Navigation page={page} root={root} />
      </details>
      <div className="docs-shell">
        <aside className="docs-sidebar">
          <Navigation page={page} root={root} />
        </aside>
        <main id="main" className="docs-main" tabIndex={-1}>
          <nav className="docs-breadcrumb" aria-label="Breadcrumb">
            <a href={root + "docs/"}>Docs</a>
            <span aria-hidden="true">/</span>
            <span>{group?.title}</span>
            <span aria-hidden="true">/</span>
            <span aria-current="page">{page.title}</span>
          </nav>
          <article
            id="doc-content"
            className="doc-content"
            dangerouslySetInnerHTML={{ __html: content }}
          />
          <nav
            className="docs-pagination"
            aria-label="Adjacent documentation pages"
          >
            {previous ? (
              <a href={root + docPath(previous)}>
                <span>
                  <ArrowLeft size={14} aria-hidden="true" /> Previous
                </span>
                <strong>{previous.title}</strong>
              </a>
            ) : (
              <span />
            )}
            {next ? (
              <a href={root + docPath(next)}>
                <span>
                  Next <ArrowRight size={14} aria-hidden="true" />
                </span>
                <strong>{next.title}</strong>
              </a>
            ) : null}
          </nav>
          <footer className="docs-footer">
            <a href={`${repository}/blob/main/${page.source}`} data-doc-source>
              Edit this page on GitHub
            </a>
            <a href={root + "third-party-notices.txt"}>Licenses and notices</a>
          </footer>
        </main>
        <aside className="docs-outline">
          <nav aria-label="On this page">
            <h2>On this page</h2>
            <ul>
              {headings.map((heading) => (
                <li
                  key={heading.id}
                  className={
                    heading.level === 3 ? "docs-outline-nested" : undefined
                  }
                >
                  <a href={"#" + heading.id}>{heading.text}</a>
                </li>
              ))}
            </ul>
          </nav>
        </aside>
      </div>
    </div>
  );
}
