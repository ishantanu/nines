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


## Guided measurement setup, 2026-10-23

The portal now offers structured availability/latency measurement forms and a measurement-based SLO creation path. The API resource kind remains `MeasurementBinding`; CLI and operator contracts are unchanged. Separately saved measurements are reusable, and preview does not save an SLO or deploy rules.

Local frontend checks passed 84 tests in 22 files. Targeted Go race tests passed for service, HTTP API, model, CLI and operator packages. The browser flow used a separate loopback, in-memory server with explicitly synthetic telemetry: structured measurement save, automatic selection in the SLO dialog, read-only compilation and planning, stale SLO revision rejection after a concurrent client write, fresh review and successful apply. API readback verified that preview had not created an SLO and that the applied definition retained its measurement reference. These browser checks do not establish real provider compatibility or production readiness.


## Intent recommendation governance, 2026-10-23

Intent change sets now freeze the visible recommendation, model/skill/compiler provenance, measurement/source evidence and before-and-after settings into the reviewed digest. The portal requires a reviewer note and acknowledgement; rejected or withdrawn approvals cannot apply, and revised goals/selections invalidate pending approvals. The definition form now hands off to the same conversation approval flow instead of independently applying the SLO and journey. Existing owner/editor manual resource APIs remain available; automated assistants must receive the server's agent role, since CLI `--agent` only changes output formatting.

A separate loopback, in-memory fixture with explicitly synthetic telemetry exercised the real browser flow: review, approve, withdraw, prepare a fresh proposal, approve/apply, prepare a rollback, separately approve/apply the restore, and verify SLO/CUJ revision 3. It also checked that rationale displayed the target change from 99% to 99.9% and its allowed bad-event fraction from 1% to 0.1%. A real CLI audit export was independently checked against each SHA-256 chain link. CLI operator export produced the exact scoped restore YAML without contacting Kubernetes. No live model/provider or production destination was used. The fixture server, temporary source and task-owned browser tab were removed.

The final PostgreSQL-backed race suite passed 631 tests/subtests with no failures and eight opt-in fixture skips; the required database gate passed with no required skips. Regression coverage includes approval withdrawal, supersession, agent/tenant boundaries, credential redaction, event-chain and historical recommendation corruption, atomic rollback, concurrent edits, creation blockers, ambiguous commit recovery and original actor attribution. PostgreSQL repository reopening preserved the audit. Frontend checks passed 90 tests in 23 files. Go/Python lint, formatting, vet, 14 release-script checks, public-source preflight, all four native builds, portal/site production builds and site static checks passed.

Rollback restores existing definitions as new revisions. Newly created resources block the entire rollback; external recording/alert rules require a separate reviewed deployment, fresh measurement coverage is required, and consumed error budget is not recoverable. The audit is append-only through the application with digest checks and export; it is not externally signed immutable storage. This alpha permits the same authorized reviewer to approve and apply. No commit, push or public deployment was performed.


## Same-name SLO measurement edits, 2026-10-23

Server/API/CLI edits now permit service, source, binding revision, queries, scrape cadence and sampling interval changes under the same SLO identity. Plans flag measurement changes and explain fresh-coverage and separate-deployment effects. The portal unlocks custom measurement fields, requires acknowledgement of the reset, exposes previous revisions, and explicitly detaches saved binding/intent provenance when switching to custom queries. Verified binding contracts remain enforced. Agent recommendation diffs include measurement fields and carry the same warnings; exact-digest approvals and separately reviewed rollback remain required.

Each replaced definition and its then-retained observations are archived in the same memory snapshot or PostgreSQL transaction as the edit. New measurement identities clear active samples, windows and health. Tests cover interval/provider-mode transitions, saved binding revisions, stale writes, late collector success/failure, tenant-scoped archive reads, pagination, non-aliasing and aborted/committed atomic definition batches. PostgreSQL tests read the new archive through an independently opened repository. The history CLI accepts an archived revision and uses the original definition/evidence.

The real browser flow used a separate loopback, in-memory server with its own acceptance SLO and Mimir ruler namespace. It changed the existing SLO's queries from explicit synthetic vectors to playground checkout counters, reviewed the reset, and applied revision 2 under the same name. The immediate view showed unknown attainment, empty coverage and out-of-date external rules. Revision 1 remained readable with its original YAML and eight interval samples. CLI readback returned revision-1 history. After a separate rule review/apply, the actual local Mimir runtime loaded all 17 rules with only revision-2 labels and no evaluation errors; the new good-event and 60-second SLI recordings returned data. No existing playground SLO or rule group was changed. The temporary loopback server is no longer running and the browser tab was not marked to persist. The local Mimir endpoint was unavailable during the final cleanup attempt, so removal of the task-owned group `nines-editable-acceptance-17f733215b` from namespace `nines-edit-acceptance-20261023` could not be confirmed. Existing playground groups were not targeted. Historical test recordings remain under backend retention.

All interval recording labels and their aggregate/alert selectors now pin the configuration revision, matching provider-window isolation. Operator tests cover same-name edits, unchanged resource UID, generation/portal-revision advancement, ownership guards and lost status. This pass did not rerun a live Kubernetes cluster. Source/destination route transfers still require original-route cleanup.

The full PostgreSQL-backed race suite passed 642 tests/subtests with zero failures and eight opt-in fixture skips; the required database gate passed without required skips. Frontend checks passed 92 tests in 24 files. Go/Python lint, formatting, vet, 14 release-script checks, public-source preflight, all four native builds and the portal production build passed. Archive snapshots have no automatic expiry in this alpha and cannot recover evidence that expired before the edit. No commit, push or public deployment was performed.


## 2026-10-23: SLO-owned measurement contracts

The default authoring flow creates one `ServiceLevelObjective` with `spec.measurement`, target, rolling window and policies. The compiler verifies recent telemetry without creating an SLO or a hidden binding. Native definitions omit derived provider selectors. Existing direct queries and explicit legacy binding references remain accepted as advanced paths.

Automated acceptance covers inline counter and classic-histogram compilation, contradictory-input rejection, missing/truncated provider evidence, no-catalog compilation, contract drift blocking/recovery, same-name edits and archived contracts. Intents carry the same measurement: owner/editor confirmation pins the exact contract/goal/version, agents cannot confirm, stale confirmations fail, edits clear evidence, and approved apply and separately approved rollback work in memory and PostgreSQL without a binding catalog entry. API, Go client, CLI and MCP tests use the real HTTP contracts with synthetic telemetry. The operator fixture compiles an inline intent to an owned SLO and recording rules without any binding CR, and checked-in CRDs match the shared schema.

The full race-enabled Go suite against a unique disposable PostgreSQL 17 container passed **652 tests/subtests**, with no required database skips. Eight opt-in real Mimir, Prometheus and Kubernetes tests were not enabled in this pass. The temporary database/container and private credential file were removed. **97 UI tests** passed across 25 files; TypeScript, portal build and site build passed. Go/Python lint, vet, 14 release-script tests, source-export preflight, static-site checks and all four native binary builds passed. Subsequent wording/request-media changes were checked with the affected client, CLI, API, intent and MCP suites.

This pass does not claim a new live-browser walkthrough, live model acceptance or real-cluster admission of the added inline schema. No public site deployment, commit or push was performed. See [measurements](measurements.md) for the one-resource flow and current constraints.


## Automatic rule deployment, 2026-10-23

Owners can enable `rules.autoDeploy` per source. Saved SLO revisions, including approved intent apply and rollback, then reconcile recording and burn-rate alert rules in the background. Prometheus uses its configured shared directory and native reload; Mimir uses the pinned ruler route; portal operator publishing uses the existing owned-resource bridge. Operational status, retries and owner pause/resume persist separately from the SLO definition. Pauses survive edits and PostgreSQL-backed server restarts. Existing manual sources remain manual.

A unique disposable dual-backend Compose stack exercised the same resource plan/apply HTTP endpoints used by the portal. Creating an inline-measurement SLO reached matching stored rules, current runtime evaluation and matching recorded output in both Prometheus and Mimir. Same-name edits automatically reached revision 2. Pausing, saving revision 3 and restarting Nines retained the pause and reported out-of-date provider rules. Owner resume deployed revision 3 on both backends. No manual rule-apply endpoint was called.

The complete playground verifier then passed local sign-in, tenant user CRUD/RBAC/session revocation, encrypted data-source CRUD and host bounds, measurement collection, automatic rule delivery, external evaluation and recorded SLI. Independent critical and warning burn alerts fired in Nines, Prometheus and Mimir, then recovered after synthetic failures returned to zero. The test retained Mimir's default one-minute ruler delay. Output verification now accounts for that default offset using the stored sample timestamp; a longer custom delay can still produce stale output. Only the test-owned containers, network and volumes were removed.

The full PostgreSQL-backed Go race suite passed **664 tests/subtests**, with zero failures and no required database skips. Eight opt-in upstream/cluster fixture anchors were not enabled in that suite; real Prometheus and Mimir behavior was exercised separately through the Compose flow above. Final affected service, intent, tenancy and query-output race regressions also passed. Tests cover opt-in defaults, foreign-rule protection, reload failure/backoff/recovery, persistent pause state, exact-revision and tenant/role controls, managed-source policy changes and preservation of pinned operator routes. **98 UI tests** passed across 25 files. TypeScript, portal/site builds, source formatting, Go/Python lint, vet, 14 release-script checks, public-source preflight, static-site checks, all four native builds and the embedded-demo HTTP smoke passed.

This pass did not run a live browser walkthrough or a Kubernetes cluster. Automatic delivery reuses the operator publication path; it does not change controller behavior or certify cluster scale. The MVP still requires one collecting server process and one owner per provider group. No commit, push or public site deployment was performed.


## Identity session revalidation, 2026-10-23

Browser authentication now records a digest of the authenticating provider identity. Every browser request and linked CLI credential checks that binding against current server configuration. Removed providers, changed OIDC issuer/client identity and changed LDAP directory/search/subject configuration reject old authentication, including direct tenant owners. Display-name and client-secret rotation preserve otherwise valid sessions. Current memberships and mappings remain independently evaluated. Pre-pinning sessions require one new sign-in; no identity-data migration is performed.

The regression failed without the binding check and passed with it. Controlled signed OIDC acceptance follows browser login through CLI authorization, approval, PKCE exchange and tenant HTTP endpoints. It verifies editor restrictions, single-tenant CLI access, live role changes and denial after new claims remove access. Cancellation consumes its request without issuing a grant. PostgreSQL reopen preserves the binding and rejects a persisted direct-owner session after an OIDC client change.

The full race suite against a unique disposable PostgreSQL 17 container passed **676 tests/subtests**, with zero failures and no required database skips. Eight opt-in upstream/cluster fixture anchors were not enabled. The container was removed. Existing encrypted LDAP and identity fixtures ran locally; this pass did not contact Microsoft or a corporate directory. Issue #22 remains open for its remaining acceptance evidence. See [identity acceptance](deployment/identity-acceptance.md).

## Website-hosted documentation, 2026-10-23

The product website publishes **31 directly addressable documentation pages** from curated Markdown, with grouped chapter navigation, on-page outlines, heading anchors, previous/next links, optional search and code copying. Small screens use a native disclosure menu. Prerendered guides and navigation remain readable without JavaScript. Markdown parsing stays on the build/dev-server side, raw HTML is disabled, and public example includes are confined to the example tree. Downloads match canonical YAML sources. Product guides stay on the website; explicitly labeled source-edit links open GitHub.

New guides cover latency bucket selection and ratio/burn arithmetic, critical user journeys, intents and MCP/skills. Published Markdown changes trigger the Pages build. LLM discovery includes hosted guide links and a full-text corpus.

**102 frontend tests** passed across 26 files, covering route/subpath resolution, link rewriting, safe Markdown, code controls and include boundaries. Build/static checks passed for all guides, local assets and heading fragments. Real HTTP checks verified 31 direct guides and 2,547 local links/assets at each of `/` and `/nines/`. The loopback development preview also served nested guides, search, artwork and canonical downloads. No live-browser visual or interaction walkthrough was performed.

Go/Python lint and formatting, vet, 14 release-script checks, public-source preflight, all four native builds, embedded-demo HTTP smoke and dependency-notice collection passed. These are local changes; no commit, push, issue closure or public website deployment was performed for these chunks.

## Portal navigation and identifiers, 2026-10-23

The portal uses canonical login and tenant page/resource paths with native browser history. Rendered DOM tests cover Back/Forward within a page and between pages/tenants, cancellation of the old tenant scope, denied tenant links, same-tab sign-in returns, expired sessions, legacy hash normalization and demo login under React Strict Mode. URL tests reject external return destinations, traversal, malformed identifiers, query-string secrets and unsupported resource routes. Server tests check GET/HEAD deep links, no-store shell responses and 404 behavior for unknown APIs/assets. No physical macOS swipe/browser walkthrough was performed; the tests exercise the history events those gestures use.

New server-generated alert/change-set/rollback IDs and portal-created opaque records use UUIDv4. Tests check the UUID version and variant. Authentication credentials retain their separate 256-bit random generator and hashed storage. Credential forms are excluded from replay, portal API keys stay in memory, login return state contains only a validated non-secret resource path, and responses prevent referrer disclosure. This is scoped verification, not a complete security audit.

The complete Go race suite passed 712 tests/subtests with a disposable PostgreSQL database and no required database skips. The temporary container was removed. All 115 frontend tests passed; production UI and the 31-page static documentation builds passed. Go lint/vet, Python lint/format, release-script tests, source-export checks and generated documentation link/fragment checks passed. All four native binaries compiled; the final read-only demo smoke verified 3 SLOs, 2 tenants, 13 embedded assets, 4 portal paths and 3 missing-route 404s. Dependency notice collection passed for 432 dependencies. No public deployment was performed.

## Observed SLI presentation, 2026-10-23

SLO rows, headers, home summaries and linked objectives now show measured SLI attainment and the target instead of a generic unknown health badge. Partial coverage, zero traffic and missing measurements have distinct labels. Existing firing/pending alerts remain visible, and unavailable budgets no longer show an exhausted progress bar. The SLI chart includes a dashed partial-window rolling series; backend compliance/alert coverage requirements are unchanged.

All 123 frontend tests passed, including regression cases for partial and tiny coverage, missing telemetry versus zero traffic, continued firing alerts, compact journey labels, missing budgets and partial/full chart-series separation. Portal and hosted documentation production builds, formatting and generated links passed. This is a presentation change; the prior database race results remain applicable. No physical browser walkthrough or public deployment was performed.

## Source reads and missing-metric diagnostics, 2026-10-23

Read-only inspection of the running playground found that its checkout SLO already had collected measurements, while API-availability SLOs referenced metrics absent from that dataset. The fixture exports `playground_requests_total`; connection success does not establish that an arbitrary metric exists.

SLO details now expose saved collection evidence, recent collection and bounded metric-name/label discovery through the selected tenant source. The CLI accepts `collect --recent`; explicit backfill ranges remain supported. Collection commits measurements but does not rewrite definitions or deploy rules. Viewer browsing, denied viewer/agent collection, cross-tenant denial, pinned Mimir organization/credentials, bounded server-clock ranges, empty results and sanitized provider failures have regression coverage.

A disposable memory-only Nines process successfully read the existing local Prometheus and Mimir backends. Both returned 120 provider-window observations and 100% SLI/coverage for a ten-minute fixture SLO. An absent metric returned zero observations and a null SLI in both backends. Metric-name discovery, label inspection and the built CLI recent-read command passed for both sources. The process was stopped and no existing SLO definitions, source configuration or external rule deployments were changed.

All 127 frontend tests and 719 Go race tests/subtests passed, including a disposable PostgreSQL run with no required database skips. Go/Python lint and formatting, vet, release-source checks, production UI/site builds and hosted documentation link checks passed. The temporary database was removed. No public deployment or physical browser walkthrough was performed.


## Commit review and browser acceptance, 2026-10-23

Fixed a late-response navigation bug in intent conversation creation: completing a request after leaving the workspace or selecting another conversation no longer opens the created conversation over the user's current view. Three StrictMode DOM regressions cover normal creation, unmount during creation and selection changes while the request is pending. All 130 frontend tests, TypeScript and the embedded portal build passed. Go and Python lint/format passed without findings; the earlier 719-case PostgreSQL-backed Go race result remains applicable because this final fix changes only the portal.

A rebuilt native server with temporary memory-only state and the existing local Prometheus passed browser sign-in and return to a tenant-scoped SLO deep link, displayed measured SLI/partial coverage, metric-name and observed-label discovery, and recent collection. Browser Back/Forward restored the expected SLO/home views and reloading an SLO deep link retained sign-in. An absent metric displayed no SLI data and an explicit empty-source-read explanation. Portal browser warning/error logs were empty. The test used no external rule destination and did not modify the existing playground's definitions.

The hosted documentation preview passed a real browser walkthrough of latency guidance, hierarchical navigation and search into the hosted approval/rollback page. Its browser warning/error logs were empty. This acceptance does not certify a physical macOS swipe gesture or a real Entra/LDAP deployment; the identity acceptance checklist still applies. No public deployment was performed.

## Replay controls and masking acceptance, 2026-10-24

This local working tree is based on merged main `a48f5d1`; these replay changes are not yet a published release. A recorder that cannot initialize now reports failure and clears its timer. Recording/stop controls remain available on SLO detail and missing-resource pages. Browser and server CSS masking now remove complete generated-content declarations, including quoted semicolons, without truncating `justify-content`, `align-content` or Tailwind property names. Existing recordings cannot recover styles removed before this correction.

A disposable memory-only demo passed real browser consent, current-app capture, SPA navigation and scrolling, pausing on an API/client screen and credential dialog, resuming on return, stopping from an SLO detail page, saving and play/pause/seek. The recorded SLO screen appeared rather than the synthetic checkout sample. A synthetic credential sentinel, the source URL and checked unmasked UI strings were absent from the 45-event stored payload. After correcting CSS masking, a fresh 14-event recording passed playback/seek with no browser warning/error logs; the earlier CSS parser warnings were no longer present. Both test servers were isolated from the existing playground.

The download action was exercised, but this in-app-browser environment did not expose a saved download artifact, so browser file delivery is not certified by this run. The shipped `portableReplayHTML` function independently generated a 341,842-byte self-contained test artifact from the corrected recording and bundled player, with the restrictive CSP and no development player URL. Opening a local file was blocked by the browser URL policy; offline-file playback was not attempted through a workaround and remains unverified.

| Acceptance surface | Evidence in this pass | Outstanding |
| --- | --- | --- |
| macOS in-app browser | Consent, navigation, scrolling, private-screen pause/resume, persistent SLO stop controls, save, playback and seek | Theme/viewport variations, browser-managed download delivery, full session/role transition matrix |
| Chrome, Firefox and Safari | Not run | Separate supported-browser/version acceptance |
| Portable HTML | Shipped exporter, embedded events/player, CSP and content checks | Reopen with Nines stopped and network unavailable; playback/seek and network inspection |

All **135 frontend tests** passed. The full Go race suite passed **720 tests/subtests** against a disposable PostgreSQL fixture, with no required database skips; eight opt-in upstream/cluster cases were not enabled. The temporary database was removed. Go/Python lint and formatting, the embedded portal build, the **32-page** site build, hosted documentation links, 14 release-script checks and public-source preflight passed. The session replay guide includes reproducible user steps and an explicit acceptance checklist. Issue #21 remains open for the unverified matrix above. No feature commit or public deployment was performed in this pass.


## Intent approval separation and tenant audit journal, 2026-10-24

This working tree requires nonempty approval notes across portal/API/CLI and different authenticated owner/editor subjects for approval and application, including rollback. Same-subject apply attempts are denied and audited. Separate credentials do not prove separate people; individual SSO identities and credential issuance controls remain necessary. Admitted proposal-preparation failures and routed malformed governance requests leave sanitized decision records without request bodies or raw provider errors.

A separate journal commits with conversation updates and retains events and frozen proposal snapshots beyond the conversation's 256-event recent view. PostgreSQL keeps compact search projections so list requests do not hydrate every large proposal. The portal's **Audit log** page, owner/editor tenant filters, full record readback, Go client and `nines audit list/show/export` are implemented. Conversation exports follow a stable sequence range; UI regressions verify that later pages are included and cross-tenant pages cannot produce a partial download. The MCP conversation audit reader accepts `after`/`through` and documents incomplete first pages. No new approval or application MCP tools are exposed.

Initial journal creation recovers complete valid chains retained in existing conversations, marking recovered snapshots explicitly. Subsequent startup does not silently recreate deleted rows. Regression checks cover recovery, PostgreSQL reopen, frozen proposal digests, retained history beyond 256 events, filtered pagination, role/tenant isolation, mutation rejection after journal-head deletion and independent rollback application. A resource commit followed by conversation persistence failure is ambiguous: it is recorded as `uncertain` when possible. Exact retry receipt recovery preserves the original actor without applying the definitions twice. Configuration rollback cannot restore consumed error budget.

The full PostgreSQL-backed Go race run passed **735 tests/subtests**, with no required database skips and eight opt-in upstream/cluster cases not enabled. The later MCP pagination regression passed its package's race suite separately. The disposable database was removed. All **142 frontend tests**, TypeScript, portal formatting and embedded portal build passed. Go/Python lint and formatting, the **32-page** hosted documentation build/link checks, 14 release-script checks and public-source preflight passed.

A loopback-only, memory-only synthetic telemetry fixture passed real browser token sign-in/return to the audit deep link, decision listing (proposal, approval, denied self-apply, independent apply and blocked creation rollback), filtering and frozen recommendation/definition readback. Browser warning/error logs were empty. A fresh native CLI exported all five fixture records across three pages, including two frozen proposal snapshots and no duplicate IDs; all records retained the selected tenant. The normal playground was not modified.

This verifies application audit behavior, not externally immutable storage. An external archive provider/retention policy is not configured, and database administrators remain outside the application's integrity boundary. The earlier replay cross-browser/download/offline-file acceptance limits remain open. No feature commit, push or public deployment was performed.


## S3 archive delivery and portable replay download, 2026-10-24

The optional tenant-owned S3 Object Lock writer now uses conditional object creation, checksum-bound uploads, expected bucket ownership, exact version/content readback, encryption checks and COMPLIANCE retention verification. PostgreSQL retains delivery claims, retry schedules and verified receipts; the journal is the durable outbox. Retention-generation recovery permits pending work to resume after a prolonged outage without overwriting earlier objects. Configuration changes produce independent destination-policy identities. No bucket policy, deletion, retention bypass or cloud credentials are exposed to agent tools.

Archive delivery and per-record receipts are available through the portal, `nines audit status`, the Go client and a tenant-scoped read API. The portal distinguishes disabled, pending/retrying, verified readback and elapsed retention. Production in-memory archival is rejected. An isolated memory-only demo visibly reported **Not configured**, rather than implying externally protected storage; browser warning/error logs were empty.

The full PostgreSQL-backed race run passed **750 tests/subtests**, with no required database skips. Nine opt-in checks were not enabled, including the new real-S3 acceptance test. A later affected-package race run passed after the lint-only switch/condition corrections. All **147 frontend tests**, TypeScript, portal build, Go/Python lint/formatting, the **33-page** hosted documentation/link checks, 14 release-script checks and public-source preflight passed. Native server, CLI, MCP and operator binaries were rebuilt. Temporary PostgreSQL state was removed.

Signed SDK/HTTP contract tests cover locked and unlocked buckets, exact conditional-retry recovery, content/version/retention mismatch and redirect rejection. Database checks cover leases, backoff, restart/readback, changed destinations, expired pending generations and cross-tenant access. No real AWS bucket was contacted. The official local MinIO image was unavailable and its official binary endpoint returned HTTP 410; no untrusted replacement was installed. A real-provider acceptance command is documented and requires explicit private bucket configuration plus confirmation; it writes one synthetic object retained for one day.

Portable replay downloads now attach their anchor to the document before clicking and remove it afterward. Unit checks cover connection at click, cancellation, embedded size limits and missing player code. A real in-app-browser download completed and returned an actual file under Downloads. Its 201,978-byte HTML contains five events including a full snapshot, an embedded player and restrictive no-network CSP. SHA-256: `5134973fd006bf25aee4b97548e05ba63efac23794fac1bdcd32fe0e43747c8f`. This verifies file delivery; it does not verify offline playback. The supported browser inventory still exposes only the in-app browser. Local-file navigation remains blocked by its policy, and no workaround was attempted. Chrome/Firefox/Safari and offline-file acceptance remain pending. No feature commit, push or public deployment was performed.


## Provider-neutral audit storage, 2026-10-24

Replaced the provisional MinIO-specific provider with `s3-compatible`. An explicit endpoint is required; AWS account-owner and KMS-ARN fields are rejected in this mode. AWS `s3` mode retains its expected-owner requirement. HTTPS, path-style bucket addressing, COMPLIANCE retention, checksum/content/version verification and encryption readback apply to the compatible path. A tenant without `auditArchive` still has no external storage requirement.

Signed SDK/HTTP contract tests cover both modes, including conditional retry recovery, unlocked buckets, missing versions, wrong content/encryption, shortened or governance retention and redirects. They verify omission of the AWS ownership header for compatible gateways and confirm that an explicit destination overrides an SDK endpoint environment variable. Tenant configuration tests cover the optional default, gateway parsing and rejected provider/endpoint combinations. The six affected Go packages passed `go test -race -count=1`; Go/Python lint and formatting passed. The hosted documentation build and all 33-page link checks passed.

The hosted guide documents Ceph RGW setup, server-side encryption, credentials and the opt-in real-provider check. No real Ceph or AWS destination was contacted, no bucket was provisioned, and no externally retained object was written. Actual Ceph retention, conditional-write and encryption acceptance remains pending. No changes were committed, pushed or publicly deployed; existing staged replay changes were preserved.


## Real Ceph container acceptance, 2026-10-24

The Nines S3-compatible writer passed against the actual ARM64 Ceph 20.2.4 RGW, using trusted loopback TLS and OpenBao 2.5.0 transit keys for SSE-S3. The official images are pinned by digest in `integration/ceph/run.sh`; `make test-ceph` bootstraps a disposable single-node monitor/manager/BlueStore OSD/RGW fixture without privileged mode or host devices. Its 2 GiB sparse block file lives only in the container filesystem.

Both an exploratory run and the complete self-contained runner passed. Nines verified the synthetic audit bytes, SHA-256, object version, AES256 encryption and one-day COMPLIANCE retention. An exact retry reused the same key/checksum/version and independent version listing found exactly one version. Deleting an unprotected encrypted control object succeeded; deleting the protected version and shortening its retention both returned AccessDenied. Retention readback was unchanged after those attempts. An unlocked bucket was rejected. The real response prompted an error-classification correction: ObjectLockConfigurationNotFoundError now reports bucket_not_locked instead of a generic destination outage, with matching signed-HTTP regressions in both provider modes.

The self-contained runner removed both of its containers and temporary credentials/certificate directory. The exploratory containers were removed separately. The existing playground was not changed, and no hosted account or real AWS bucket was contacted. Production IAM/KMS, storage administrator boundaries, key rotation, node failures, HA and disaster recovery remain separate acceptance requirements. Destroying the disposable container filesystem is outside API Object Lock enforcement. No changes were committed, pushed or publicly deployed.

The affected archive, intent-flow, tenant and server Go race suites passed, and the new acceptance command compiled. Go/Python lint and formatting, native server rebuild, all 14 release-script tests and the 708-file public-source preflight passed.


## Replay issue 21 closeout preparation, 2026-10-24

Replay hardening now validates timeline structure, timestamps, node bounds and a complete page snapshot before playback/export. Corrupt or absent streams fail visibly, the parent waits for the actual player handshake, and an initialization timeout or player error replaces unexplained blank output. Failed loads clear the old player so its timer cannot overwrite the error. Both sanitizers exclude explicitly private subtrees. Stored event bytes are checked against the retained digest before readback.

StrictMode regressions cover private-page pause/resume, tenant switching with consent reset, viewer restrictions, API-rejected session expiry, owner-to-viewer transitions and immediate discard while logout is pending. Save regressions prove that late completion cannot change the user's newer route, discard a newer capture or accept foreign-tenant metadata. Memory/PostgreSQL checks reject unauthorized read contexts and corrupted event bytes, and inspect physical expired-record deletion rather than relying only on filtered reads.

All **172 frontend tests in 39 files**, TypeScript and the embedded portal/player build passed. The complete Go race suite passed **772 tests/subtests** with disposable PostgreSQL and no required database skips; nine separately enabled upstream/cluster/provider checks were not run in that suite. The temporary database was removed. The earlier real Ceph container run remains separately recorded above.

A fresh isolated in-app demo captured the actual SLO list/detail navigation, scrolling, light/dark change and a private credential dialog pause/resume. Stopping on the detail page, saving, play/pause and seeking to the recorded SLO screen passed with no browser warning/error logs. The retained stream contains **44 events**; its synthetic credential sentinel, source origin and checked original title are absent from the stored event payload. A download was requested, but this latest run did not expose a delivered browser download file. The shipped exporter separately produced a **631,985-byte** self-contained HTML from that same stream and current bundled player, SHA-256 `f5a5a3b2f14dd6c10ff8db94b05e1020845f0f5475c0972497c786d286513bbe`. Artifact generation does not establish offline browser playback. The capture/player browser flow preceded the final sign-out and response-scope guard additions, which passed their regressions and a fresh embedded build.

The browser surface exposes only the in-app browser and blocks local-file navigation. No alternate browser surface or local-file workaround was used. The maintainer agreed to run the Chrome/Firefox/Safari and direct offline HTML matrix in the [session replay guide](session-replay.md#issue-21-alpha-acceptance). Those manual rows, including actual download delivery in each browser, remain pending. Issue #21 is not yet closed. The browser tab and isolated demo server were stopped; the normal playground was not modified. No commit, push or public deployment was performed.

Final Go/Python lint and formatting, all four native builds, the 14 release-script tests and the 714-file public-source preflight passed. The existing 35-file staged replay snapshot was preserved; the later hardening and regenerated assets remain working-tree changes awaiting deliberate staging.

### Replay selective masking, 2026-10-24

New recordings use `selective-v2`. Product-owned navigation, SLO table headings, detail tabs, chart headings, integration stages and known alert/data states can remain readable through explicitly annotated labels from one finite browser/Go catalog. Arbitrary annotations, nested unmarked text, resource names, provider values and input events remain masked. Existing private-screen pauses and subtree exclusions still apply. Legacy `masked-v1` ingestion/playback retains blanket masking and its retry identity; changing the consent policy on an ingestion retry is rejected. Existing masked text cannot be recovered by export.

- Portal: 175 tests across 41 files passed, including a resource named `Home` remaining private while the product's `Home` navigation is readable. Shared adversarial streams cover snapshots, text mutations, added/reused nodes, private subtrees, unknown annotations, input events, scope resets and sanitizer idempotence.
- Go: 776 tests/subtests passed under the race detector with disposable PostgreSQL; nine opt-in tests skipped, with no required database skips. The first parallel attempt exhausted temporary linker disk space; the successful rerun serialized compilation. The temporary database was removed.
- In-app browser: a fresh current-tab capture traversed the SLO list, synthetic search, detail/configuration/performance tabs and a private credential dialog. A saved 66-event, 13.1-second replay displayed readable navigation, detail tabs, chart headings and integration states. SLO names, queries and metric numbers stayed masked. Stored payload checks confirmed both synthetic search/credential sentinels and the original URL were absent; browser warning/error logs were empty. Final annotations and the dynamic-heading boundary also passed the automated suite.
- The shipped exporter generated a 66-event portable HTML fixture from that capture, with private sentinel/URL checks passing. This is exporter evidence, not confirmation of browser-managed file delivery or an additional offline browser test. The maintainer reported that the previous offline fixture worked, without supplying a browser/version matrix. The desktop-browser/offline matrix in the session replay guide remains the release acceptance boundary for issue 21.

Consent copy, the hosted session replay guide and the site's LLM-readable feature notes describe selective masking. Go/Python lint and formatting, all four native builds, the 33-page static documentation build/link checks and the 722-file public-source preflight passed. The existing 35-file staging snapshot is unchanged. This verification does not publish the site, commit changes or close the issue.


### Replay alpha acceptance decision, 2026-10-24

The maintainer chose to stop additional desktop-browser testing and accept the completed replay work for the experimental alpha. This supersedes the earlier manual-matrix closure gate above. Existing automated results, real in-app-browser captures and the maintainer-reported successful offline fixture remain the evidence; Chrome/Firefox/Safari versions and per-browser download/offline behavior remain explicitly unverified. No additional compatibility passes are claimed. Issue #21 may close with this caveat when the implementation merges. Browser-specific failures should be filed as defects with reproduction details. Replay retains its experimental status.
