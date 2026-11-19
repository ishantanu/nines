# Product site and logo

The product site and documentation are static pages generated from React components using the same Astryx components and `nines` theme as the portal. Documentation markdown is rendered to static HTML with Astryx headings, links, code, quotes, tables and buttons, so it needs no client-side Astryx.

- The build prerenders HTML for readers and crawlers without JavaScript.
- The landing page hydrates its interactive examples; documentation uses a separate small script for navigation, search and copying commands, without loading React in the browser.
- An `llms.txt` file summarizes resource contracts and agent interfaces; `llms-full.txt` contains the published guides.
- It has no server, sign-in flow, analytics, replay recorder, model integration or API credentials.
- Product screenshots are represented by a clearly labeled illustration with synthetic values.

## Website-hosted documentation

- The **Docs** navigation opens `/docs/` on the product website.
  - Guides are organized into Getting started, Define reliability, Work with agents, Deploy and connect, and Operate and contribute.
  - Each page has a grouped sidebar, stable heading anchors, an on-page outline and previous/next links.
  - Small screens and narrow browser windows, including Chrome with docked DevTools, use a sticky native expandable chapter menu with its own scroll area.
  - Header height is measured when the layout changes, so the menu stays below the header even when search wraps.
  - Search loads its public index only when used; code blocks support copying after the documentation script loads.
- The Markdown in `docs/public` and `docs/cli.md` remains the source.
  - The explicit page registry in `web/site/docs/navigation.ts` determines the published guides; it does not scan private documents or the rest of the repository.
  - The build renders Markdown with raw HTML disabled.
  - Links to other published guides stay on the website.
  - Links explicitly labeled **Edit this page on GitHub** open the source for contributors.
- An `<!-- example: examples/request-latency.slo.yaml -->` directive includes a canonical public example during rendering.
  - Includes are restricted to YAML/Go inside the public example tree.
  - The same files are available as downloads.
  - This avoids maintaining a second YAML definition in the website source.
  - Add new public guides to the registry and the Markdown directory in the same change.
- Every guide is emitted as its own `index.html`, so direct visits and reloads work on GitHub Pages without a server rewrite or JavaScript router.
  - Initial content, navigation, tables, examples and anchors work without JavaScript.
  - Markdown parsing happens at build time. Chapter fragments are a separate static data cache, not part of the interactive browser bundle.
  - The production checker verifies every generated HTML page, internal link, fragment and local asset.
- With JavaScript enabled, chapter links replace only the article and outline. After first paint, a low-priority request warms a content-addressed cache of prerendered chapter fragments. Once warm, direct chapter clicks need no new HTML request.
  - The sidebar, styles and script remain loaded.
  - Hovering or focusing a chapter link prefetches that guide, with at most two concurrent speculative requests.
  - The fragment cache is capped at 2 MB of uncompressed JSON. It is separate from the small navigation script and never executes embedded scripts. Data-saver and slow-2G/2G connections skip background warming.
  - Until warming completes, a clicked chapter fetches its HTML at high priority without waiting for the background request. A failed cache request leaves ordinary chapter navigation available.
  - An in-memory cache holds up to eight parsed pages; browser Back/Forward restores the route and saved reading position.
  - Modified clicks, downloads and external links keep their native behavior.
  - Failed navigation falls back to a normal full-page visit.
  - This is progressive enhancement: no server API, SPA rewrite or client-side Markdown renderer is required.
- The documentation script has a regression-tested 12 KB minified budget and no third-party runtime dependencies.
  - The production build also limits the bootstrap plus static documentation dependencies to 16 KB, and the chapter cache to 3 MB.
  - The landing-page React bundle is requested only on the landing page.
  - This prevents interactive examples from delaying documentation interactivity; it is not a promise of a particular LCP under every network or device.
  - Use Chrome's Performance panel with cache disabled and a consistent throttling profile to compare first visits and navigation after deployment.

## Portal screenshots

- The homepage gallery, README and SLO guides include reviewed captures of the current portal: SLO trends, per-service burn, coverage counts and timestamped chart inspection.
- Captures use only the synthetic demo workspace (`go run ./cmd/server -demo`), which contains no production or provider data. They are optimized JPEGs, lazy-loaded with reserved dimensions; screenshot bytes are pinned in the public-source allowlist.

## Examples

- The site shows complete, source-backed YAML for an SLO, a critical user journey, a self-contained latency SLO and an experimental SLO intent.
  - The practical examples cover the Prometheus playground, offline rule generation, connected tenant access, recorded SLI and burn metrics, the typed Go client, a read-only MCP client and synthetic intent workflow evaluations.
  - Code can be copied from each example; links lead to the full guides.
  - Workflow panels retain their dimensions when switching tabs, and inactive examples remain in prerendered HTML for readers and crawlers.

## Local preview

```sh
npm ci --prefix web
npm run dev:site --prefix web
```

For a production build:

```sh
make site
python3 -m http.server 4173 --bind 127.0.0.1 --directory dist/site
```

- The build goes to `dist/site`, separately from the embedded application UI.
  - All assets use relative URLs so the same build works at a GitHub project subpath. Dependencies reuse the application's committed lockfile.
  - Bundled runtime notices and the incorporated shadcn/ui notice are included in `third-party-notices.txt`.

## GitHub Pages

- The product site is served at [ishantanu.github.io/nines](https://ishantanu.github.io/nines/) through GitHub Actions. New content reaches the live site after merge to `main` and a successful Product site deployment.

1. Review and publish the curated source export as the repository root. Keep the parent analysis directory, local profiles and historical evidence outside the repository.
2. Set the repository's default branch to `main`.
3. In Settings → Pages, select **GitHub Actions** as the build and deployment source.
4. Review the `github-pages` environment and restrict deployment to `main`.
5. Push a site or published documentation change, or manually run the **Product site** workflow. Its deploy job uses the verified build artifact and displays the actual deployed URL.

- Pull requests build and validate the site with read-only repository access.
  - Only `main` deployments receive Pages and OIDC write permissions.
  - The workflow uploads only `dist/site`, never the application build, parent directory, recordings, local configuration or full repository. See [GitHub's workflow documentation](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

## Logo

- [Default wordmark](brand/nines.svg)
- [Wordmark for dark backgrounds](brand/nines-dark.svg)
- [Standalone mark](brand/nines-mark.svg)
- The mark is a geometric number nine, referring to reliability expressed in nines.
  - The wordmark uses vector paths, so it requires no installed font.
  - Source SVGs contain no scripts, external assets or embedded bitmap data.
  - The README selects the light or dark wordmark for the reader's theme.
  - The product site and portal share the same source SVGs and build plugin, including the dark-background wordmark.
- The logo source is covered by the project license.
  - No trademark registration or exclusivity is claimed.
  - Keep the `viewBox` intact when resizing and provide an accessible text label when using the mark alone.

## Local verification

- The production site build and source-export checks pass.
  - Browser verification covered desktop at 1440px and mobile at 390px and 320px, with no page-level horizontal overflow.
  - Both YAML tabs keep the same panel height; arrow-key navigation and copying demo commands work.
  - No browser warnings or hydration errors were reported during that check.
  - The application frontend tests and rebuilt server demo smoke check passed during that verification; repeat the release checks for the candidate being published. The repository is now accessible.
  - GitHub deployment requires a successful Product site workflow and Pages configuration; the actual run URL is the deployment record.

## Intent workflow documentation

- The **For agents** section explains exact-digest approval and the post-apply rollback boundary. **Examples → Intent checks** provides credential-free drafting and workflow commands. Both link to the [intent evaluation guide](intent-evaluations.md), which records provider setup, source and backtest provenance, persistence tests and measured local-model limitations. `llms.txt` carries the same scope and limits for agent readers.
- The site continues to mark model-assisted workflows as experimental.
  - Passing scripted adapter fixtures does not establish real model quality.
  - The new guide and website content are published only after this change is merged to `main` and the Product site workflow completes; a feature-branch push does not update the live site.

### Portal sign-in component

The portal uses Beautiful UI's free Notchset Sign In registry block, with the retained MIT source at `web/src/components/ui/notchset/blocks/sign-in.tsx`. A Nines account-form slot connects the existing local/LDAP forms and configured OIDC links. The adapter hides email-code, passkey and signup controls because those backends are not configured. Demo mode uses the same block and keeps `admin / admin` explicitly limited to the sample workspace. The upstream OTP/segmented-control modules are lazy-loaded and are not requested by the account adapter. Authentication, CSRF and tenant/RBAC decisions remain on the server.

### Homepage feature illustrations

The public homepage uses the free MIT Beautiful UI Feature Bento registry block after the detailed critical user journey example, before the portal screenshots. The Features navigation link opens this section directly. Its tiles illustrate change review, documentation discovery, synthetic budget burn and independent approval/application. The block is labelled as illustrative; it does not execute CLI commands, call telemetry providers or change SLOs. The guide palette keeps its three destinations visible until a reader searches, and opens local documentation and retains ordinary links in the prerendered HTML. Readers can pause the illustrations; reduced-motion, hidden-tab and offscreen states stop the shared animation clock. Component attribution is retained in the website's third-party notices. Documentation enhancement JavaScript remains independent of the landing-page component.
