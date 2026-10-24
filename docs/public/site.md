# Product site and logo

The product site and documentation are static React pages using the same shadcn/ui components as the application. The build prerenders HTML for readers and crawlers without JavaScript; hydration enables tabs and copying commands. An `llms.txt` file summarizes resource contracts and agent interfaces; `llms-full.txt` contains the published guides. It has no server, sign-in flow, analytics, replay recorder, model integration or API credentials. Product screenshots are represented by a clearly labeled illustration with synthetic values.

## Website-hosted documentation

The **Docs** navigation opens `/docs/` on the product website. Guides are organized into Getting started, Define reliability, Work with agents, Deploy and connect, and Operate and contribute. Each page has a grouped sidebar, stable heading anchors, an on-page outline and previous/next links. Small screens use a native expandable chapter menu. Search loads its public index only when used; code blocks support copying after hydration.

The Markdown in `docs/public` and `docs/cli.md` remains the source. The explicit page registry in `web/site/docs/navigation.ts` determines the published guides; it does not scan private documents or the rest of the repository. The build renders Markdown with raw HTML disabled. Links to other published guides stay on the website. Links explicitly labeled **Edit this page on GitHub** open the source for contributors.

An `<!-- example: examples/request-latency.slo.yaml -->` directive includes a canonical public example during rendering. Includes are restricted to YAML/Go inside the public example tree. The same files are available as downloads. This avoids maintaining a second YAML definition in the website source. Add new public guides to the registry and the Markdown directory in the same change.

Every guide is emitted as its own `index.html`, so direct visits and reloads work on GitHub Pages without a server rewrite or JavaScript router. Initial content, navigation, tables, examples and anchors work without JavaScript. Markdown parsing and the guide corpus are not shipped in the interactive browser bundle. The production checker verifies every generated HTML page, internal link, fragment and local asset.

## Examples

The site shows complete, source-backed YAML for an SLO, a critical user journey, a self-contained latency SLO and an experimental SLO intent. The practical examples cover the Prometheus playground, offline rule generation, connected tenant access, recorded SLI and burn metrics, the typed Go client, a read-only MCP client and synthetic intent workflow evaluations. Code can be copied from each example; links lead to the full guides. Workflow panels retain their dimensions when switching tabs, and inactive examples remain in prerendered HTML for readers and crawlers.

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

The build goes to `dist/site`, separately from the embedded application UI. All assets use relative URLs so the same build works at a GitHub project subpath. Dependencies reuse the application's committed lockfile. Bundled runtime notices and the incorporated shadcn/ui notice are included in `third-party-notices.txt`.

## GitHub Pages

The product site is served at [ishantanu.github.io/nines](https://ishantanu.github.io/nines/) through GitHub Actions. New content reaches the live site after merge to `main` and a successful Product site deployment.

1. Review and publish the curated source export as the repository root. Keep the parent analysis directory, local profiles and historical evidence outside the repository.
2. Set the repository's default branch to `main`.
3. In Settings → Pages, select **GitHub Actions** as the build and deployment source.
4. Review the `github-pages` environment and restrict deployment to `main`.
5. Push a site or published documentation change, or manually run the **Product site** workflow. Its deploy job uses the verified build artifact and displays the actual deployed URL.

Pull requests build and validate the site with read-only repository access. Only `main` deployments receive Pages and OIDC write permissions. The workflow uploads only `dist/site`, never the application build, parent directory, recordings, local configuration or full repository. See [GitHub's workflow documentation](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

## Logo

- [Default wordmark](brand/nines.svg)
- [Wordmark for dark backgrounds](brand/nines-dark.svg)
- [Standalone mark](brand/nines-mark.svg)

The mark is a geometric number nine, referring to reliability expressed in nines. The wordmark uses vector paths, so it requires no installed font. Source SVGs contain no scripts, external assets or embedded bitmap data. The README selects the light or dark wordmark for the reader's theme. The product site and portal share the same source SVGs and build plugin, including the dark-background wordmark.

The logo source is covered by the project license. No trademark registration or exclusivity is claimed. Keep the `viewBox` intact when resizing and provide an accessible text label when using the mark alone.

## Local verification

The production site build and source-export checks pass. Browser verification covered desktop at 1440px and mobile at 390px and 320px, with no page-level horizontal overflow. Both YAML tabs keep the same panel height; arrow-key navigation and copying demo commands work. No browser warnings or hydration errors were reported during that check. The application frontend tests and rebuilt server demo smoke check passed during that verification; repeat the release checks for the candidate being published. The repository is now accessible. GitHub deployment requires a successful Product site workflow and Pages configuration; the actual run URL is the deployment record.

## Intent workflow documentation

The **For agents** section explains exact-digest approval and the post-apply rollback boundary. **Examples → Intent checks** provides credential-free drafting and workflow commands. Both link to the [intent evaluation guide](intent-evaluations.md), which records provider setup, source and backtest provenance, persistence tests and measured local-model limitations. `llms.txt` carries the same scope and limits for agent readers.

The site continues to mark model-assisted workflows as experimental. Passing scripted adapter fixtures does not establish real model quality. The new guide and website content are published only after this change is merged to `main` and the Product site workflow completes; a feature-branch push does not update the live site.
