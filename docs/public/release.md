# Public MVP release

The release goal is one reproducible path from a clean clone to a working local SLO/CUJ workspace, plus deterministic offline CLI use. Freeze feature expansion while correcting release failures. Keep experimental capabilities documented without promoting them to supported production behavior.

## Release gates

- [x] Project license chosen: Apache-2.0.
- [x] Product README and focused public setup/operation documentation.
- [x] Source export excludes the parent directory, local profiles, binaries, recordings and historical lab evidence by construction.
- [x] Candidate workflow definitions for frontend, race/database checks, live disposable Mimir, container build and cross-platform artifact preparation.
- [x] Select `ishantanu/nines` and use `github.com/ishantanu/nines` consistently for the Go module and imports.
- [ ] Review the exported source and dependency notices; run current vulnerability scans and resolve blocking findings.
- [ ] Run the committed workflows successfully in the actual repository; record run links and the exact candidate commit.
- [ ] Verify clean install/demo, persistent container startup, auth configuration, source queries, reviewed rule deployment and restore on the candidate.
- [ ] Enable branch protections, secret scanning and private vulnerability reporting; confirm maintainer contact.
- [ ] Review candidate assets, checksums, dependency inventory and release notes, then explicitly publish the prerelease.

The [local verification record](verification.md) records completed checks and limitations. These checkboxes track implemented release preparation, not GitHub execution results. The public source repository is `ishantanu/nines`. The manual candidate workflow never creates tags or releases. The alpha tag workflow prepares a draft prerelease for maintainer review; publication remains explicit.

## Source export

```sh
make public-source
# Review dist/public-source/ and dist/public-source-manifest.json.
```

Only the platform's allowed source/configuration trees, focused public docs and release files are copied. `.git`, `.env`, node_modules, binaries, editor profiles, parent analysis files and historical evidence are excluded. The export refuses symlinks, private absolute paths, common credential markers and broken local documentation links. This is a bounded preflight, not a complete secret or provenance audit; manual review remains mandatory.

The existing Nines checkout can be committed directly after reviewing its staged and unstaged changes. The export is an optional clean-source review artifact; do not initialize another Git repository inside this checkout. Preserve third-party notices. The selected repository is `ishantanu/nines`; the Go module and SDK examples use `github.com/ishantanu/nines`.

## Candidate assets

`make release VERSION=v0.1.0-alpha.1` prepares archives for the selected build targets; it does not publish anything. Override `RELEASE_TARGETS` for a smaller local proof. The builder requires a version, collects dependency notices, includes CLI/server/MCP binaries and emits SHA-256 checksums and build metadata. The Kubernetes reconciler is excluded from first-release binary assets. The server container includes application and dependency notices under `/licenses/`.

The manual GitHub candidate workflow builds and uploads reviewable artifacts with read-only repository permissions. It neither creates a tag nor publishes a GitHub release. A maintainer reviews the exact commit, all release gates and assets before taking the final publication step. Public SDK installation requires the selected repository and version to be published.

## Tag-based alpha binaries

After merging the release workflow into `main`, wait for a successful **CI** run on the exact commit you intend to release. Tags such as `v0.1.0-alpha.1` trigger **Alpha release**. The workflow accepts only numbered alpha tags on commits reachable from `main` with successful main-branch CI.

```sh
git fetch origin --tags
git tag -s v0.1.0-alpha.1 <tested-main-commit> -m "Nines v0.1.0-alpha.1"
git push origin v0.1.0-alpha.1
```

Choose the next unused version; never move a published version tag. Tag signing uses your local Git signing configuration. If CI is still running when the tag is pushed, validation fails with an explanation; rerun the workflow after CI succeeds.

The workflow reuses the candidate builder to prepare Linux and macOS archives for amd64 and arm64. Each archive contains the CLI, server and MCP adapter, examples, docs and dependency licenses. `SHA256SUMS` covers all archives and `build.json`; the manifest identifies the exact version, commit and targets. The workflow verifies metadata, archive inventory and checksums and smoke-tests the Linux amd64 CLI and server before uploading anything to a release. Other architectures are cross-built, not run by that smoke test.

Only the final draft job receives release-write permission. It creates a **draft prerelease**, not a public announcement. Review the assets, release notes and checksums in GitHub Releases, then publish the draft when satisfied. Reruns may refresh a draft for the same commit; they refuse to overwrite published releases or drafts for another commit. Binaries are not code-signed or notarized. Checksums provide integrity checking, not independent publisher authentication.

For an unpublished preview before tagging, manually run **Prepare release candidate** with the desired version. Its artifacts remain attached to the workflow run; it does not create a Git tag or release.

## Before the first push

From a clean checkout with Go 1.27.3, Node 24, Python 3 and Docker Compose:

```sh
npm ci --prefix web --no-audit --no-fund
npm test --prefix web
npm run build --prefix web
go test ./...
make check build release-check
npm run build:site --prefix web
python3 scripts/site/check.py
python3 scripts/release/source.py --publication
```

Run the [integration playground verifier](deployment/playground.md) for collection, managed users/sources, both severity policies and recovery. Review `git status`, the final diff and dependency notices; do not stage `.env`, generated binaries or local evidence. Commit with your configured signing key, push to `ishantanu/nines`, and inspect the CI and Pages runs before tagging a release. See the [local verification record](verification.md) for the checks already performed and their limits.
