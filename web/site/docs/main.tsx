// Documentation is rendered at build time. Keep reading and navigation independent
// of React, the landing-page widgets, Markdown parsers and provider SDKs.
interface SearchEntry {
  slug: string;
  title: string;
  text: string;
}
interface PageSnapshot {
  main: HTMLElement;
  outline: HTMLElement;
  title: string;
  description: string;
  slug: string;
}

export function mountDocumentation(root: HTMLElement) {
  const home = root.querySelector<HTMLAnchorElement>("a.brand")!;
  const base = new URL(home.href);
  const routes = new Set(
    Array.from(
      root.querySelectorAll<HTMLAnchorElement>(
        '.docs-sidebar nav[aria-label="Documentation chapters"] a',
      ),
      (link) => new URL(link.href).pathname,
    ),
  );
  const canonicalPath = (url: URL) =>
    url.pathname.replace(/\/index\.html$/, "/");
  const docURL = (link: HTMLAnchorElement) => {
    const url = new URL(link.href);
    return url.origin === base.origin &&
      !url.search &&
      !link.hasAttribute("download") &&
      (!link.target || link.target === "_self") &&
      routes.has(canonicalPath(url))
      ? url
      : null;
  };
  const absoluteLinks = (element: HTMLElement, url: URL) => {
    for (const link of element.querySelectorAll<HTMLElement>("[href], [src]")) {
      for (const attribute of ["href", "src"]) {
        const value = link.getAttribute(attribute);
        if (value) link.setAttribute(attribute, new URL(value, url).href);
      }
    }
  };
  // The persistent sidebar/header must keep working across different path depths.
  absoluteLinks(document.documentElement, new URL(location.href));
  const pages = new Map<string, PageSnapshot>();
  const pending = new Map<string, Promise<PageSnapshot>>();
  const controllers = new Set<AbortController>();
  const timers = new Set<ReturnType<typeof setTimeout>>();
  let disposed = false;
  let navigation = 0;
  let index: Promise<SearchEntry[]> | undefined;
  let searchVersion = 0;
  const previousScrollRestoration = history.scrollRestoration;
  history.scrollRestoration = "manual";

  const readPage = (document: Document): PageSnapshot => {
    const main = document.querySelector<HTMLElement>("#main.docs-main");
    const outline = document.querySelector<HTMLElement>(".docs-outline");
    const marker = document.querySelector<HTMLElement>("#root[data-doc]");
    if (!main || !outline || !marker || !main.querySelector("#doc-content"))
      throw new Error("Not a documentation page");
    return {
      main: main.cloneNode(true) as HTMLElement,
      outline: outline.cloneNode(true) as HTMLElement,
      slug: marker.dataset.doc ?? "",
      title: document.title,
      description:
        document
          .querySelector('meta[name="description"]')
          ?.getAttribute("content") ?? "",
    };
  };
  const remember = (path: string, page: PageSnapshot) => {
    pages.delete(path);
    pages.set(path, page);
    if (pages.size > 8) pages.delete(pages.keys().next().value!);
    return page;
  };
  remember(canonicalPath(new URL(location.href)), readPage(document));

  const load = (url: URL): Promise<PageSnapshot> => {
    const path = canonicalPath(url);
    const cached = pages.get(path);
    if (cached) return Promise.resolve(cached);
    const existing = pending.get(path);
    if (existing) return existing;
    const controller = new AbortController();
    controllers.add(controller);
    const timeout = setTimeout(() => controller.abort(), 8000);
    const request = fetch(new URL(path, base), { signal: controller.signal })
      .then(async (response) => {
        if (
          !response.ok ||
          !response.headers.get("content-type")?.includes("text/html") ||
          (response.url &&
            (new URL(response.url).origin !== base.origin ||
              canonicalPath(new URL(response.url)) !== path))
        )
          throw new Error("Documentation unavailable");
        const html = await response.text();
        if (disposed || html.length > 1_000_000)
          throw new Error("Documentation unavailable");
        const page = readPage(
          new DOMParser().parseFromString(html, "text/html"),
        );
        if (
          new URL(`docs/${page.slug ? page.slug + "/" : ""}`, base).pathname !==
          path
        )
          throw new Error("Documentation route mismatch");
        // Only trusted, same-origin static documentation routes are eligible.
        // Scripts are never imported or executed during chapter navigation.
        for (const element of [page.main, page.outline]) {
          for (const script of element.querySelectorAll("script"))
            script.remove();
          absoluteLinks(element, new URL(path, base));
        }
        return remember(path, page);
      })
      .finally(() => {
        clearTimeout(timeout);
        controllers.delete(controller);
        pending.delete(path);
      });
    pending.set(path, request);
    return request;
  };
  const saveScroll = () =>
    history.replaceState(
      { ...history.state, docsScroll: [scrollX, scrollY] },
      "",
    );
  const position = (url: URL, scroll?: number[]) => {
    const main = root.querySelector<HTMLElement>("#main")!;
    main.focus({ preventScroll: true });
    if (scroll) window.scrollTo(scroll[0], scroll[1]);
    else if (url.hash) {
      let id = url.hash.slice(1);
      try {
        id = decodeURIComponent(id);
      } catch {
        /* Use the literal anchor. */
      }
      const heading = document.getElementById(id);
      if (heading) heading.scrollIntoView();
      else window.scrollTo(0, 0);
    } else window.scrollTo(0, 0);
  };
  const navigate = async (url: URL, pop = false, scroll?: number[]) => {
    const version = ++navigation;
    root.querySelector(".docs-main")?.setAttribute("aria-busy", "true");
    try {
      const page = await load(url);
      if (disposed || version !== navigation) return;
      if (!pop) {
        saveScroll();
        history.pushState({ docsScroll: [0, 0] }, "", url);
      }
      root.querySelector(".docs-main")!.replaceWith(page.main.cloneNode(true));
      root
        .querySelector(".docs-outline")!
        .replaceWith(page.outline.cloneNode(true));
      root.dataset.doc = page.slug;
      root
        .querySelector(".skip-link")
        ?.setAttribute("href", new URL("#main", url).href);
      document.title = page.title;
      for (const selector of [
        'meta[name="description"]',
        'meta[property="og:description"]',
      ])
        document
          .querySelector(selector)
          ?.setAttribute("content", page.description);
      document
        .querySelector('meta[property="og:title"]')
        ?.setAttribute("content", page.title);
      for (const link of root.querySelectorAll<HTMLAnchorElement>(
        'nav[aria-label="Documentation chapters"] a',
      )) {
        if (new URL(link.href).pathname === canonicalPath(url))
          link.setAttribute("aria-current", "page");
        else link.removeAttribute("aria-current");
      }
      const menu = root.querySelector<HTMLDetailsElement>(".docs-mobile-nav");
      if (menu) menu.open = false;
      const search = root.querySelector<HTMLInputElement>("#docs-search");
      if (search) search.value = "";
      root.querySelector<HTMLElement>("#docs-search-results")!.hidden = true;
      search?.setAttribute("aria-expanded", "false");
      search?.removeAttribute("aria-controls");
      searchVersion++;
      position(url, scroll);
    } catch {
      // Failed requests/deploy races retain a normal, directly addressable link.
      if (!disposed && version === navigation) location.assign(url.href);
    } finally {
      if (!disposed && version === navigation)
        root.querySelector(".docs-main")?.removeAttribute("aria-busy");
    }
  };
  const click = (event: MouseEvent) => {
    if (!(event.target instanceof Element)) return;
    const copy = event.target.closest<HTMLButtonElement>(
      "button[data-copy-code]",
    );
    if (copy) {
      const code = copy
        .closest(".docs-code-block")
        ?.querySelector("pre")?.textContent;
      if (code != null) {
        void (async () => {
          try {
            await navigator.clipboard.writeText(code);
            copy.textContent = "Copied";
          } catch {
            copy.textContent = "Select code to copy";
          }
          if (disposed) return;
          const timer = setTimeout(() => {
            copy.textContent = "Copy";
            timers.delete(timer);
          }, 2200);
          timers.add(timer);
        })();
      }
      return;
    }
    const link = event.target.closest<HTMLAnchorElement>("a[href]");
    if (
      !link ||
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.altKey ||
      event.shiftKey
    )
      return;
    const url = docURL(link);
    if (!url) return;
    event.preventDefault();
    if (canonicalPath(url) === canonicalPath(new URL(location.href))) {
      // Anchor history needs the same scroll bookkeeping as chapter history.
      navigation++;
      if (url.href !== location.href) {
        saveScroll();
        history.pushState({}, "", url);
      }
      root.querySelector(".docs-main")?.removeAttribute("aria-busy");
      const menu = root.querySelector<HTMLDetailsElement>(".docs-mobile-nav");
      if (menu) menu.open = false;
      position(url);
      return;
    }
    void navigate(url);
  };
  const dismissMenu = (event: KeyboardEvent) => {
    const menu = root.querySelector<HTMLDetailsElement>(".docs-mobile-nav");
    if (event.key === "Escape" && menu?.open) {
      menu.open = false;
      menu.querySelector("summary")?.focus();
    }
  };
  const prefetch = (event: Event) => {
    const link =
      event.target instanceof Element
        ? event.target.closest<HTMLAnchorElement>("a[href]")
        : null;
    const url = link && docURL(link);
    if (url && pending.size < 2) void load(url).catch(() => {});
  };
  const popstate = (event: PopStateEvent) => {
    const url = new URL(location.href);
    if (!routes.has(canonicalPath(url))) return;
    // Restore cached chapters without adding a new history entry.
    void navigate(url, true, event.state?.docsScroll);
  };
  const search = root.querySelector<HTMLInputElement>("#docs-search")!;
  const results = root.querySelector<HTMLElement>("#docs-search-results")!;
  search.disabled = false;
  search.removeAttribute("aria-describedby");
  const input = async () => {
    const version = ++searchVersion;
    const words = search.value
      .trim()
      .slice(0, 200)
      .toLocaleLowerCase()
      .split(/\s+/)
      .filter(Boolean);
    const active = search.value.trim().length > 1;
    results.hidden = !active;
    search.setAttribute("aria-expanded", String(active));
    if (!active) {
      search.removeAttribute("aria-controls");
      return;
    }
    search.setAttribute("aria-controls", results.id);
    const status = results.querySelector("p")!;
    const list = results.querySelector("ul")!;
    list.replaceChildren();
    status.textContent = "Loading search...";
    if (!index) {
      const controller = new AbortController();
      controllers.add(controller);
      index = fetch(new URL("docs/search.json", base), {
        signal: controller.signal,
      })
        .then((response) => {
          if (!response.ok) throw new Error("Search unavailable");
          return response.json() as Promise<SearchEntry[]>;
        })
        .finally(() => controllers.delete(controller));
    }
    try {
      const entries = await index;
      if (disposed || version !== searchVersion) return;
      const matches = entries
        .filter((entry) =>
          words.every((word) =>
            `${entry.title} ${entry.text}`.toLocaleLowerCase().includes(word),
          ),
        )
        .slice(0, 8);
      status.textContent = matches.length
        ? `${matches.length} matching pages`
        : "No matching pages";
      for (const entry of matches) {
        const url = new URL(`docs/${entry.slug ? entry.slug + "/" : ""}`, base);
        if (!routes.has(url.pathname)) continue;
        const item = document.createElement("li");
        const link = document.createElement("a");
        link.href = url.href;
        link.textContent = entry.title;
        item.append(link);
        list.append(item);
      }
    } catch {
      if (!disposed && version === searchVersion)
        status.textContent =
          "Search is unavailable. Browse the chapters below.";
      index = undefined;
    }
  };
  const dismissSearch = (event: KeyboardEvent) => {
    if (event.key === "Escape") {
      results.hidden = true;
      search.setAttribute("aria-expanded", "false");
      searchVersion++;
    }
  };
  // CSS also provides a no-JavaScript fallback; measure wrapped mobile headers.
  const resize = () => {
    const height =
      root.querySelector(".docs-header")?.getBoundingClientRect().height ?? 77;
    document.documentElement.style.setProperty(
      "--docs-header-height",
      `${height}px`,
    );
  };
  const observer = new ResizeObserver(resize);
  observer.observe(root.querySelector(".docs-header")!);
  resize();
  root.addEventListener("click", click);
  root.addEventListener("keydown", dismissMenu);
  root.addEventListener("pointerover", prefetch);
  root.addEventListener("focusin", prefetch);
  search.addEventListener("input", input);
  search.addEventListener("keydown", dismissSearch);
  window.addEventListener("popstate", popstate);
  return () => {
    disposed = true;
    navigation++;
    for (const controller of controllers) controller.abort();
    for (const timer of timers) clearTimeout(timer);
    observer.disconnect();
    document.documentElement.style.removeProperty("--docs-header-height");
    history.scrollRestoration = previousScrollRestoration;
    root.removeEventListener("click", click);
    root.removeEventListener("keydown", dismissMenu);
    root.removeEventListener("pointerover", prefetch);
    root.removeEventListener("focusin", prefetch);
    search.removeEventListener("input", input);
    search.removeEventListener("keydown", dismissSearch);
    window.removeEventListener("popstate", popstate);
  };
}
