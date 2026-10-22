# Local MVP verification

Verified on 2026-10-21 against a local candidate using the `github.com/ishantanu/nines` module path. This source tree has no release commit or tag. GitHub CI execution and release approval remain outstanding.

| Check | Local result |
| --- | --- |
| Go toolchain and module verification | Go 1.27.3; module integrity checked |
| Go race suite without external fixtures | 445 cases passed, 35 fixture-dependent cases skipped |
| Go race suite with disposable PostgreSQL | 481 cases passed; only the two live Mimir cases skipped |
| Live disposable Mimir | All 11 cases passed, including tenant collection and ruler destinations |
| Go vet | Passed |
| Frontend | 55 tests in 16 files passed; production build passed |
| Release script checks | Five checks passed; publication source preflight passed |
| Server container | Image built; authenticated synthetic demo and packaged dependency notices passed |
| Locked npm dependency audit | No reported vulnerabilities |
| Go vulnerability scan | No reachable or imported-package vulnerabilities reported |

The Go scan still reports module-only advisory GO-2026-5932 for the unmaintained OpenPGP package in `golang.org/x/crypto`; this application does not import that package. Repeat scans on the actual release commit rather than treating this report as permanent assurance.

The frontend build reports a large minified chunk warning. It passes compilation; bundle optimization remains follow-up work.

Dependency notices are collected with a machine-readable inventory. The PostHog browser-common and react-remove-scroll-bar fallback notice provenance requires maintainer review, as recorded in `third_party/license-sources.json`. This is not a completed legal review or comprehensive source-provenance audit.

Database and Mimir fixtures used dedicated loopback containers that were removed after the checks. Existing local services were left running. Synthetic identity tests do not establish a real Microsoft Entra or LDAP deployment. A disposable Compose deployment passed authenticated CLI plan/apply, restart persistence and a logical database backup/restore round trip. Public ingress, real deployment recovery, production scale and operator-cluster acceptance remain unverified.

## Integration playground verification, 2026-10-22

The isolated Compose playground passed source collection, reviewed rule deployment, recent external evaluator health, recorded SLI queries, burn-alert firing and recovery in both Prometheus and Mimir. The verifier first established healthy alert baselines, then injected 10% synthetic failures and restored successful traffic. Both Nines' own evaluator and each external backend fired and recovered. The stack uses its own loopback ports and volumes; the existing Prometheus on port 9090 was not reconfigured.

The complete Go suite passed without externally configured test fixtures; affected rules/API/CLI/client packages also passed race checks. Frontend tests, the embedded production UI build, Go vet, publication preflight and all five release-script checks passed. A fresh vulnerability scan reported zero reachable or imported-package vulnerabilities. Dependency notice collection passed for 288 dependencies; legal/provenance review remains a release requirement. The CI playground job is configured but has not been executed on GitHub for these changes.

This verifies synthetic alert state, not Alertmanager notification delivery, production Mimir storage, high availability or performance under load. The playground intentionally uses public development credentials and short windows; it is separate from production deployment guidance.

## Management and severity policies, 2026-10-22

The complete Go suite and affected-package race checks passed. Frontend validation passed 58 tests across 17 files and the embedded production build. Publication source preflight and Go vet passed.

Disposable Prometheus and Mimir verified independent critical and warning policy alerts through firing and recovery, with matching Nines evaluator state. Local password sign-in, user create/read/update/access removal, viewer RBAC rejection, password-session revocation, managed source create/test/read/update/delete, credential redaction and upstream host/organization boundaries passed against the real PostgreSQL-backed server. These are synthetic local checks, not production identity-provider or notification-delivery certification.

## MVP branding and deployment check, 2026-10-22

The portal and Pages build serve the same canonical SVG wordmarks and mark. The frontend passed all 58 tests, TypeScript compilation and the production build; Linux container and local build produced the same main asset hash. The prerendered Pages build, static asset/SVG checks, publication preflight and five release-script checks passed. The complete Go suite and Go vet passed; the native demo smoke check passed with the updated severity-policy fixtures. CLI/MCP binaries built, offline validation and severity-rule generation passed, npm reported zero vulnerabilities, and the Go scan reported no reachable or imported-package vulnerabilities. The previously recorded module-only advisory remains outside imported code.

A fresh isolated persistent Compose deployment passed local account sign-in, user creation, viewer mutation denial, SLO creation and account/role/SLO retention after server restart. Private configuration generation passed its permission and overwrite-refusal checks. A disposable kind v0.33.0 / Kubernetes v1.37.0 cluster accepted the supplied Deployment, Service and authentication ConfigMap. It passed the same login/RBAC/SLO checks and retained accounts and SLOs across replacement of the server Pod. The cluster used a separate PostgreSQL test fixture, not production storage, ingress or PostgreSQL TLS.

The isolated Prometheus/Mimir management and severity verifier passed both critical and warning alert firing and recovery, with matching Nines states. Its synthetic windows account for upstream delay; checks match current revision labels. The deployment guides cover persistent Compose, Kubernetes, native Linux, authentication, metrics/rule destinations and backup/restore. GitHub workflows, publication and a version tag remain maintainer actions on the final committed candidate.

## Final source and archive pass, 2026-10-22

A fresh public-source export passed 489 Go race test cases against a dedicated PostgreSQL fixture, with only the two live Mimir cases skipped. The required database gate passed with no database-dependent skips. The demo window-edit test was corrected to keep both severity policies within the shortened SLO window. The affected MCP protocol race tests and Go vet passed after replacing its hardcoded version with release build metadata.

All 58 frontend tests passed. A clean dependency install and production build exposed a difference between unset and explicitly empty opt-in telemetry settings; the build now normalizes both cases and matches the committed embedded assets byte for byte. The Pages build/static checks, all three workflow definitions under actionlint, module integrity, five release-script checks and publication source preflight passed. Fresh npm audit reported zero vulnerabilities; govulncheck reported no reachable or imported-package vulnerabilities, with the previously recorded module-only advisory still outside imported code.

An unpublished `darwin/arm64` candidate archive passed checksum and dependency-inventory verification, offline CLI initialization/validation, critical/warning rule generation, GitOps rendering, embedded server demo startup and a real stdio MCP initialization/tool-discovery exchange. MCP reported the candidate version and exposed 43 read-only tools by default. The archive README and native-service guide now distinguish binary installation from commands that require a source checkout. This was a local preview with unknown commit metadata, not a signed or published release; other native targets remain subject to the committed candidate workflow.

The running playground still reported saved, collected, deployed and evaluating states for both Prometheus and Mimir. This final pass did not repeat fault injection; the earlier full severity firing/recovery evidence above remains the integration record. The dedicated launch database fixture was removed, and existing services were preserved. Dependency-notice provenance review, GitHub execution on the final commit, repository security settings and explicit release approval remain maintainer gates. The experimental operator's live-cluster acceptance remains outstanding.


## Prometheus-first playground, 2026-10-22

The base Compose configuration now contains only the Prometheus workspace/backend, without a Mimir service, dependency, volume or remote-write destination. The optional `compose.playground.mimir.yaml` override with `--profile mimir` adds the second backend and its remote-write, tenant/auth and verifier settings. Release-source export includes the override and all three optional configuration fixtures. CI now has separate Prometheus-only and dual-backend matrix runs; the definitions passed actionlint, but those matrix jobs still need execution on GitHub.

Both isolated local configurations passed local login, the exact expected workspace list, user CRUD/RBAC/session revocation, managed-source CRUD/redaction/host bounds, measurement collection, reviewed rule deployment, external evaluation, recorded SLI, critical/warning alert firing and recovery. The optional run additionally passed Mimir organization bounds and both evaluators' independent alert lifecycles. Live Prometheus configuration confirmed that remote write is absent by default and enabled only in the optional mode. Five release-script checks, publication preflight and Python compilation passed.

The lifecycle fixtures reused the previously verified server image. A fresh image built from the updated public-source export produced the same embedded UI asset hash and was installed into the existing playground. Existing local-account login and the saved SLO survived the switch from two workspaces to one; collection, deployment and external evaluation remained healthy. The running playground no longer has a Mimir container, and its Mimir data volume was preserved. Both temporary verification projects and their synthetic volumes were removed. No Git changes were staged or committed.

## Go and Python lint gate, 2026-10-22

The initial uncapped golangci-lint v2.15.0 run reported 336 findings: 285 unchecked errors, 47 static checks and four unused command wrappers. The final `make lint` passed with zero Go issues, no Go/import formatting drift, and clean Ruff v0.9.6 correctness/import/bugbear/upgrade and formatting checks across all 13 Python files. The standard Go linters remain enabled without blanket issue exclusions. CLI table/prompt write failures and configuration-lock cleanup failures now reach callers; test setup/database fixture failures are checked. Cleanup errors with no recovery path are explicitly discarded. Regression tests cover early output failure and preservation of both action and lock-cleanup errors.

A fresh full Go race run against dedicated PostgreSQL passed 491 cases, with only the two live Mimir cases skipped. The required-database gate passed with no database skips. Five release-script unit checks, Python compilation, publication source preflight, workflow lint and a freshly built native server demo smoke check passed. Python AST review showed only formatting/import changes outside the explicit playground callback bindings and source-export configuration additions. The temporary lint database was removed; the running playground was left intact.

The pinned lint tools and configuration are included in public-source export. `make lint`, `make lint-go` and `make lint-python` check source without editing it; CI installs the pinned versions in its own environment. Python remains limited to development/release verification, local setup and synthetic playground helpers. The native Go binaries and final server container have no Python runtime dependency. The new CI job is configured and locally linted, not yet executed on GitHub for the final commit.


## Credential clarity and environment-backed PostgreSQL, 2026-10-22

The README now opens with a login table separating in-memory demo, playground and persistent Compose credentials. Deployment guides and `.env.example` distinguish web passwords, API tokens and database passwords. `scripts/setup-local.py --playground` prints the playground's actual default login and startup command; the default invocation prints persistent-mode instructions. Generation preserves private permissions, never prints generated private secrets and refuses to overwrite an existing file. Its new regression check passed for both modes, bringing release-script checks to six.

Persistent Compose, the Prometheus playground and the optional Mimir configuration all reject a missing `POSTGRES_PASSWORD`. Config checks verified that the supplied environment value reaches both PostgreSQL and the Nines database URL without a fixed fallback. CI's database and playground paths now generate masked per-run passwords. The exact CI database bootstrap script was executed locally with a private temporary environment file; both PostgreSQL restart/persistence anchors passed with generated credentials. TCP readiness avoids accepting the image's initialization-only Unix socket. Its temporary container, volume and environment file were removed.

The existing playground received a private, ignored mode-0600 `.env`. Its database role was synchronized with the generated environment password before database/server recreation; no volume was deleted and the web password was not changed. Existing `admin` / `nines-playground-password` login, the Prometheus workspace, saved SLO, collection, deployed rules and external evaluation all passed afterward. No private credential values were printed. Go/Python lint, formatting, all six release-script checks, workflow lint and public-source preflight passed. CI changes still require execution on GitHub for the committed candidate.


## Consistent demo login, 2026-10-22

The loopback playground now defaults to `admin` / `admin`, matching the in-memory demo. The explicit `allowDemoCredentials` setting accepts this short password only for the configured local bootstrap administrator and requires a loopback public URL. Ordinary users and persistent deployments retain the 12–72-byte password policy; PostgreSQL still requires its separate environment-backed password. Bootstrap does not overwrite existing account passwords on restart.

Affected identity, HTTP API and server race tests passed, including configuration boundaries, real browser-session login, ordinary-user password rejection, password changes, session revocation and restart retention. Go/Python lint, all six release-script checks and public-source preflight passed. The local playground image reuses its existing runtime layer with a newly cross-built Linux/arm64 server binary. The running administrator was updated through the version-checked user API without resetting storage. Browser-session login with `admin` / `admin`, the single Prometheus workspace and retained saved/collected/deployed/evaluating SLO states passed. No full burn-alert cycle was repeated for this credential change.


## Alpha binaries and practical site examples, 2026-10-22

Main commit `04c6ccae74019d678c0d92e9d896734788e9d53f` passed repository CI (run `39777521006`). The existing candidate workflow built `v0.1.0-alpha.1` for Linux/macOS on amd64/arm64 in run `39779477657`. All four downloaded archives passed exact-version/commit, clean-checkout, inventory, size and SHA-256 checks. The actual darwin/arm64 CLI reported the expected metadata and validated the bundled SLO example offline; the packaged server passed the synthetic demo/embedded-assets smoke check. Other architectures were cross-built by this run and were not executed locally.

The new tag workflow requires a numbered alpha tag on a main-reachable commit with successful main-branch CI. It reuses the candidate builder, checks all archive metadata/checksums, runs Linux amd64 CLI/server smoke checks and prepares a draft prerelease. Published assets are never overwritten. This workflow has passed actionlint locally; its tag-triggered execution remains to be verified after merge and tag creation.

The site now includes six copyable practical workflows and four YAML resource examples, with the Go client source imported from the canonical example. Static prerendering supports Go as well as YAML sources. The production build, static asset/prerender checks, publication preflight, all twelve release-script checks and Python lint/formatting passed. Browser checks covered all six workflow tabs at a 559-pixel viewport: panel dimensions remained equal, page width matched the viewport and no browser errors or warnings were captured. This pass does not claim a full device matrix. Pages now watches the shared root TypeScript configuration so compiler fixes also trigger a product-site rebuild.
