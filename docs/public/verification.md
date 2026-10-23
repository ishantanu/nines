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

## Operator support verification, 2026-10-22

The operator is now part of the supported alpha deployment path and release inventory. Local acceptance used a disposable agentless K3s API with verified TLS, the exact shipped ServiceAccount/Role, real Prometheus Operator and real Mimir. It passed CRD admission, namespace/project isolation, all four kinds, explicit binding generation selection, stale-parent write fencing, owned drift repair, foreign ownership refusal, cascading garbage collection, lease revocation/standby takeover, current-generation Mimir evaluation/output and guarded finalizer cleanup. Binding telemetry metadata was synthetic; this does not establish model quality.

The packaged nonroot read-only Linux container passed service-account authentication, `/healthz`, `/readyz`, reconciliation of the shipped SLO/CUJ sample and graceful termination. This uncovered and fixed Kubernetes MicroTime lease serialization on Linux; nanosecond, trailing-zero and integral-second cases now have regression coverage. Native archive preparation included a versioned executable operator, dependency notices and the installation files. This was an unpublished dirty-tree preview, not a signed or clean release candidate.

The full PostgreSQL-backed race suite, required operator cases, Go/Python lint, vet, 14 release checks, source export, 61 frontend checks and site build passed locally. The fixture does not schedule workload pods or certify production scale, every Kubernetes/provider version, ingress or a real identity provider. Existing services were preserved; disposable containers, image, network and credentials were removed. The committed candidate still needs successful GitHub CI and maintainer release review.

## Native rule activation and honest status, 2026-10-23

A disposable pinned Prometheus 3.5.1 fixture verified the shared rule directory, file-only apply remaining unloaded, review invalidation when reload is enabled, native reload acknowledgement, exact current runtime evaluation, current query-output identity and unchanged-apply activation suppression. Unit tests cover disabled lifecycle, denied credentials, failed/ambiguous acknowledgements, runtime readback, redirect refusal, separate authentication and the final authority/destination guard. Files remain written after an activation failure; HTTP success for persistence is explicitly separate from activation/evaluation success.

The real Mimir fixture passed its required collection, ruler destination/output and independent critical/warning pending-duration/recovery cases using the new apply orchestration. The final real Kubernetes/operator fixture also passed CRD admission, namespace RBAC, reconciliation, ownership, garbage collection, lease failover, provider evidence and the packaged nonroot service-account container checks. These fixtures use only their own temporary resources; they do not certify production scale or all provider versions.

The full PostgreSQL-backed Go race run recorded 595 test/subtest passes, no failures and seven opt-in fixture skips. All seven fixture anchors passed separately through their required Prometheus/Mimir/operator runners. The final affected Go packages passed again after persistence-message changes. Frontend checks passed 67 tests in 20 files, including failed activation and insufficient-window/zero-traffic explanations. The embedded portal and product site built successfully. All four native binaries built; the synthetic server smoke verified three demo objectives, two tenants and 13 embedded assets. Go/Python lint, formatting, vet, 14 release-script checks and public source preflight passed.

Browser verification used a separate Compose playground: local sign-in, an SLO rule plan, reviewed unchanged apply, immediate integration refresh and the separate configuration/activation/evaluation result were observed. The running group and recorded marker were current while the new ten-minute SLO still had partial measurement coverage. Unknown health remains a measurement result; rule activation never manufactures healthy attainment. The final text additionally names missing coverage or zero traffic beside the SLO summary.

The full Compose playground lifecycle passed with the native reload path and no watcher: local sign-in, tenant user CRUD/RBAC/session revocation, encrypted data-source CRUD/bounds, source collection, reviewed rule apply, external evaluation, recorded SLI, critical and warning firing in both evaluators, and recovery after synthetic failures returned to zero. Its generated database credentials, project volumes, network and containers were removed; the unrelated existing database remained running.

This is local dirty-tree verification. These changes have not been committed, pushed, published or verified by GitHub CI. Release signing, a clean candidate and maintainer approval remain separate steps.

## Portal-to-operator publication, 2026-10-23

Portal rule plans now publish reviewed `ServiceLevelObjective` CRs through an administrator-configured, same-cluster namespace and stable publisher identity. The server service account is restricted to get/create/update/delete on SLO CRs; the operator retains provider-rule writes, status, finalizers and its lease. The dedicated real-cluster case used the shipped publisher Role and verified that this account could not read PrometheusRule resources.

The real API flow created and edited a portal SLO before its first publication, proving portal revision 3 remained distinct from Kubernetes generation 1. Saving and planning caused no publication. Agent/viewer deployment and agent withdrawal were rejected; a stale digest was rejected. Owner apply published the CR, the running lease-elected operator reconciled it into Mimir, and the server verified current runtime/output using the actual Kubernetes UID. An unreviewed edit left the old publication unchanged and showed out-of-date evidence; a subsequent reviewed apply reached revision 4. Withdrawal waited for Mimir finalizer/foreground cleanup, retained the portal definition and history, and released the persistent route pin only after the parent was absent.

Unit tests cover foreign GitOps ownership, durable route guards across publisher restart, metadata-only revision changes, stale operator evidence, ambiguous write readback without repeated mutations and pending withdrawal. PostgreSQL restart preserved the route guard through definition updates. Named operator connections work for SSO-only tenants without adding API-key grants. MCP withdrawal is a separate opt-in in addition to write/deployment capabilities and still requires the exact reviewed digest and API owner role.

The final PostgreSQL-backed race suite passed 607 tests/subtests with no failures and eight opt-in fixture skips. All four required real operator cases passed separately, including the new portal flow, plus the packaged nonroot Linux runtime checks. The Prometheus path has compiler, ownership and real upstream selector/ConfigMap coverage; the new portal end-to-end provider case uses Mimir. The agentless Kubernetes fixture does not schedule Prometheus workload pods. Frontend checks passed 68 tests in 20 files; Go/Python lint and formatting, vet, 14 release-script checks, public source preflight, all four native builds and the embedded demo smoke passed. Site build and static deployment checks passed.

The server publishes only portal-owned SLOs. It does not import existing GitOps resources or publish portal CUJ/binding/intent CRs. Provider activation, output visibility, measurement health and runtime removal after withdrawal remain separate evidence. This is local working-tree verification; no commit, push, production-cluster change or public deployment was performed.
