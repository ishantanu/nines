// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mountDocumentation } from "./main";

let cleanup: () => void;
let fetcher: ReturnType<typeof vi.fn>;
const origin = location.origin;
const path = "/nines/docs/";
function html(slug = "", title = "Introduction") {
  const prefix = slug ? "../../../" : "../";
  return `<!doctype html><html><head><title>${title}</title><meta name="description" content="${title} guide"></head><body>
  <div id="root" data-doc="${slug}"><a class="skip-link" href="#main">Skip to content</a><header class="docs-header"><a class="brand" href="${prefix}">Nines</a>
  <input id="docs-search" disabled><div id="docs-search-results" hidden><p role="status"></p><ul></ul></div></header>
  <details class="docs-mobile-nav"><summary>Browse documentation</summary><nav aria-label="Documentation chapters"><a href="${prefix}docs/">Introduction</a><a href="${prefix}docs/deployment/operator/">Operator</a></nav></details>
  <aside class="docs-sidebar"><nav aria-label="Documentation chapters"><a href="${prefix}docs/" aria-current="page">Introduction</a><a href="${prefix}docs/deployment/operator/">Operator</a><a href="${prefix}docs/measurements/">Measurements</a></nav></aside>
  <main id="main" class="docs-main" tabindex="-1"><article id="doc-content"><h1>${title}</h1><h2 id="example">Example</h2>
  <a id="anchor" href="#example">Example</a><a id="external" href="https://example.com/">External</a><a id="download" download href="${prefix}docs/deployment/operator/">Download</a>
  <div class="docs-code-block"><button data-copy-code>Copy</button><pre>nines validate</pre></div></article></main>
  <aside class="docs-outline"><a href="#example">Example</a></aside></div></body></html>`;
}
function mount() {
  const parsed = new DOMParser().parseFromString(html(), "text/html");
  document.head.innerHTML = parsed.head.innerHTML;
  document.body.innerHTML = parsed.body.innerHTML;
  cleanup = mountDocumentation(document.querySelector("#root")!);
}
function click(selector: string, init: MouseEventInit = {}) {
  const event = new MouseEvent("click", {
    bubbles: true,
    cancelable: true,
    ...init,
  });
  let consumed = false;
  const preventNativeNavigation = (event: Event) => {
    consumed = event.defaultPrevented;
    event.preventDefault();
  };
  document.addEventListener("click", preventNativeNavigation, { once: true });
  document.querySelector(selector)!.dispatchEvent(event);
  return { defaultPrevented: consumed };
}
async function settled() {
  for (let i = 0; i < 15; i++) await Promise.resolve();
}
function response(slug: string, title: string) {
  return new Response(html(slug, title), {
    headers: { "Content-Type": "text/html; charset=utf-8" },
  });
}
beforeEach(() => {
  history.replaceState({}, "", path);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
  fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  mount();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

it("navigates nested guides without replacing the sidebar or loading new scripts", async () => {
  const sidebar = document.querySelector(".docs-sidebar");
  fetcher.mockResolvedValue(response("deployment/operator", "Operator"));
  expect(
    click('.docs-sidebar a[href$="deployment/operator/"]').defaultPrevented,
  ).toBe(true);
  await settled();
  expect(location.pathname).toBe(path + "deployment/operator/");
  expect(document.title).toBe("Operator");
  expect(
    document.querySelector('meta[name="description"]')?.getAttribute("content"),
  ).toBe("Operator guide");
  expect(document.querySelector(".docs-sidebar")).toBe(sidebar);
  expect(
    document.querySelector(".docs-sidebar a[aria-current=page]")?.textContent,
  ).toBe("Operator");
  expect(document.activeElement?.id).toBe("main");
  expect(document.querySelector<HTMLAnchorElement>(".brand")?.href).toBe(
    origin + "/nines/",
  );
  expect(document.querySelector<HTMLAnchorElement>("#anchor")?.href).toBe(
    origin + path + "deployment/operator/#example",
  );
  expect(document.querySelector<HTMLAnchorElement>(".skip-link")?.href).toBe(
    origin + path + "deployment/operator/#main",
  );
  expect(document.querySelector("script")).toBeNull();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("deduplicates hover/focus prefetch and click, and closes the compact menu", async () => {
  let resolve!: (response: Response) => void;
  fetcher.mockReturnValue(
    new Promise<Response>((done) => {
      resolve = done;
    }),
  );
  const link = document.querySelector(
    '.docs-mobile-nav a[href$="deployment/operator/"]',
  )!;
  (document.querySelector(".docs-mobile-nav") as HTMLDetailsElement).open =
    true;
  link.dispatchEvent(new Event("pointerover", { bubbles: true }));
  link.dispatchEvent(new FocusEvent("focusin", { bubbles: true }));
  click('.docs-mobile-nav a[href$="deployment/operator/"]');
  expect(fetcher).toHaveBeenCalledTimes(1);
  resolve(response("deployment/operator", "Operator"));
  await settled();
  expect(
    (document.querySelector(".docs-mobile-nav") as HTMLDetailsElement).open,
  ).toBe(false);
  expect(document.querySelector("h1")?.textContent).toBe("Operator");
});

it("restores a cached chapter and saved scroll on browser back without refetching", async () => {
  fetcher.mockResolvedValue(response("deployment/operator", "Operator"));
  click('.docs-sidebar a[href$="deployment/operator/"]');
  await settled();
  history.replaceState({ docsScroll: [0, 240] }, "", path);
  window.dispatchEvent(new PopStateEvent("popstate", { state: history.state }));
  await settled();
  expect(document.title).toBe("Introduction");
  expect(window.scrollTo).toHaveBeenLastCalledWith(0, 240);
  expect(fetcher).toHaveBeenCalledTimes(1);
  click('.docs-sidebar a[href$="deployment/operator/"]');
  await settled();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("keeps anchors in history without fetching the chapter again", () => {
  click("#anchor");
  expect(location.hash).toBe("#example");
  expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalled();
  expect(fetcher).not.toHaveBeenCalled();
});

it("preserves modified clicks, downloads and external links", () => {
  expect(
    click('.docs-sidebar a[href$="deployment/operator/"]', { metaKey: true })
      .defaultPrevented,
  ).toBe(false);
  expect(
    click('.docs-sidebar a[href$="deployment/operator/"]', { button: 1 })
      .defaultPrevented,
  ).toBe(false);
  expect(click("#external").defaultPrevented).toBe(false);
  expect(click("#download").defaultPrevented).toBe(false);
  expect(fetcher).not.toHaveBeenCalled();
});

it("does not let a slower earlier chapter replace the latest click", async () => {
  let resolve!: (response: Response) => void;
  fetcher.mockReturnValueOnce(
    new Promise<Response>((done) => {
      resolve = done;
    }),
  );
  fetcher.mockResolvedValueOnce(response("measurements", "Measurements"));
  click('.docs-sidebar a[href$="deployment/operator/"]');
  click('.docs-sidebar a[href$="measurements/"]');
  await settled();
  resolve(response("deployment/operator", "Operator"));
  await settled();
  expect(document.title).toBe("Measurements");
  expect(location.pathname).toBe(path + "measurements/");
});

it("loads search only on demand, reuses it and renders result text safely", async () => {
  expect(fetcher).not.toHaveBeenCalled();
  fetcher.mockResolvedValue(
    new Response(
      JSON.stringify([
        {
          slug: "deployment/operator",
          title: "Operator <img>",
          text: "Kubernetes operator",
        },
        {
          slug: "../../outside",
          title: "Operator unsafe route",
          text: "operator",
        },
      ]),
    ),
  );
  const input = document.querySelector<HTMLInputElement>("#docs-search")!;
  expect(input.disabled).toBe(false);
  input.value = "operator";
  input.dispatchEvent(new Event("input"));
  await settled();
  const result = document.querySelector("#docs-search-results li a");
  expect(result?.textContent).toBe("Operator <img>");
  expect(document.querySelectorAll("#docs-search-results li")).toHaveLength(1);
  expect(document.querySelector("#docs-search-results img")).toBeNull();
  input.value = "kubernetes";
  input.dispatchEvent(new Event("input"));
  await settled();
  expect(fetcher).toHaveBeenCalledTimes(1);
  input.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
  expect(
    document.querySelector<HTMLElement>("#docs-search-results")!.hidden,
  ).toBe(true);
});

it("retains useful chapter navigation when search fails", async () => {
  fetcher.mockRejectedValue(new Error("offline"));
  const input = document.querySelector<HTMLInputElement>("#docs-search")!;
  input.value = "operator";
  input.dispatchEvent(new Event("input"));
  await settled();
  expect(
    document.querySelector("#docs-search-results p")?.textContent,
  ).toContain("Browse the chapters");
  expect(document.querySelectorAll(".docs-sidebar a")).toHaveLength(3);
});

it("copies code through the same delegated handler after a chapter changes", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  fetcher.mockResolvedValue(response("deployment/operator", "Operator"));
  click('.docs-sidebar a[href$="deployment/operator/"]');
  await settled();
  click("button[data-copy-code]");
  await settled();
  expect(writeText).toHaveBeenCalledWith("nines validate");
  expect(document.querySelector("button[data-copy-code]")?.textContent).toBe(
    "Copied",
  );
});

it("aborts pending requests and removes listeners when disposed", () => {
  fetcher.mockReturnValue(new Promise(() => {}));
  click('.docs-sidebar a[href$="deployment/operator/"]');
  const signal = fetcher.mock.calls[0][1].signal as AbortSignal;
  cleanup();
  cleanup = () => {};
  expect(signal.aborted).toBe(true);
  expect(click('.docs-sidebar a[href$="measurements/"]').defaultPrevented).toBe(
    false,
  );
});

it("closes the compact chapter menu with Escape and returns keyboard focus", () => {
  const menu = document.querySelector<HTMLDetailsElement>(".docs-mobile-nav")!;
  menu.open = true;
  menu
    .querySelector("a")!
    .dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
  expect(menu.open).toBe(false);
  expect(document.activeElement?.tagName).toBe("SUMMARY");
});
