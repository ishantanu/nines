# Product site and logo

The product site is a static React page using the same shadcn/ui button and tabs as the application. The build prerenders HTML for readers and crawlers without JavaScript; hydration enables tabs and copying commands. An `llms.txt` file summarizes resource contracts and agent interfaces. It has no server, sign-in flow, analytics, replay recorder, model integration or API credentials. Product screenshots are represented by a clearly labeled illustration with synthetic values.

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

The intended project is `ishantanu/nines`, with the default Pages address `https://ishantanu.github.io/nines/`. This is a planned address until a successful deployment is recorded; preparing a workflow does not create a public site.

1. Review and publish the curated source export as the repository root. Keep the parent analysis directory, local profiles and historical evidence outside the repository.
2. Set the repository's default branch to `main`.
3. In Settings → Pages, select **GitHub Actions** as the build and deployment source.
4. Review the `github-pages` environment and restrict deployment to `main`.
5. Push a site change or manually run the **Product site** workflow. Its deploy job uses the verified build artifact and displays the actual deployed URL.

Pull requests build and validate the site with read-only repository access. Only `main` deployments receive Pages and OIDC write permissions. The workflow uploads only `dist/site`, never the application build, parent directory, recordings, local configuration or full repository. See [GitHub's workflow documentation](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

## Logo

- [Default wordmark](brand/nines.svg)
- [Wordmark for dark backgrounds](brand/nines-dark.svg)
- [Standalone mark](brand/nines-mark.svg)

The mark is a geometric number nine, referring to reliability expressed in nines. The wordmark uses vector paths, so it requires no installed font. Source SVGs contain no scripts, external assets or embedded bitmap data. The README selects the light or dark wordmark for the reader's theme. The product site and portal share the same source SVGs and build plugin, including the dark-background wordmark.

The logo source is covered by the project license. No trademark registration or exclusivity is claimed. Keep the `viewBox` intact when resizing and provide an accessible text label when using the mark alone.

## Local verification

The production site build and source-export checks pass. Browser verification covered desktop at 1440px and mobile at 390px and 320px, with no page-level horizontal overflow. Both YAML tabs keep the same panel height; arrow-key navigation and copying demo commands work. No browser warnings or hydration errors were reported during that check. The current 58 frontend tests and the rebuilt server demo smoke check also pass. The repository is now accessible. GitHub deployment requires a successful Product site workflow and Pages configuration; the actual run URL is the deployment record.
