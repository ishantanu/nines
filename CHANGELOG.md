# Changelog

## Unreleased (v0.1.0-alpha.3)

- Prefer shortWindow/longWindow duration strings in burn-policy YAML, with strict parsing, legacy seconds input and matching portal/CLI/operator exports.

- Treat New Relic no-event notices as empty measurements without fabricated coverage; use provider-neutral collection failure text.

- Replace per-chart sample tables and time-point scrubbers with direct pointer/keyboard inspection and compact synchronized readouts.

- Enable VictoriaMetrics connection creation in the portal and managed API, with optional bearer credentials and explicitly authorized cluster account/project routing.

- Remove PostHog operator diagnostics and external recorder configuration; retain consent-based native replay. Simplify overview health counts and show measurement labels as readable fields instead of raw JSON.

- Consolidate dependency PRs #46–#53: update setup-node, Prometheus, x/time, Lucide, PostHog and Vitest; migrate browser telemetry to coordinated OpenTelemetry 3 packages with three-signal export and redaction regression coverage.

- Add opt-in OAuth-protected remote MCP with tenant-fixed, distinct client/user bindings, read-only defaults, request limits and SDK conformance tests; preserve stdio and align Nines credential environment variables.
- Add credential-free provider contract checks and real VictoriaMetrics container acceptance to CI.

- Add on-demand native Datadog/New Relic SLO attainment, history and explicit budget provenance, plus bounded discovery and experimental reviewed YAML import drafts across API, Go client, CLI and MCP.

- Consolidate local playground profiles in one Compose file and add a launcher that remembers backend, Jev, sample workloads and private provider configuration.


- Allow explicit localhost/127.0.0.1 aliases in playground authentication on the configured port, preserving CSRF checks, host-only cookies and strict production origins.


- Show bundled provider SVG symbols and connection identities beside SLOs. Correct workspace provider labels, restrict guided selectors to compatible adapters and document native publication separately from collection and vendor alerting.

- Add reviewed New Relic event-count SLI/SLO publication with account/entity verification, immutable revision ownership, provider-generated metric queries and explicit managed-connection opt-in.

- Add reviewed Datadog native metric SLO publication for 7/30 day windows, explicit connection opt-in and verified provider links. Native burn monitors remain separate.

- Add experimental Google Cloud Monitoring DELTA-count collection, explicit ADC/token authentication, bounded filter validation, reviewed immutable native SLO revisions and separate native health evidence.

- Add experimental dataset-pinned Dash0 collection, reviewed native SLO publication with ownership/version fencing, and offline OpenSLO v1 export with explicit omission warnings.

- Add experimental VictoriaMetrics interval-count collection for single-node and pinned cluster tenants, with official MetricsQL parsing, SLO count-shape validation, explicit partial-response refusal and a disposable real-backend acceptance test.

- Add experimental New Relic NerdGraph event-count collection through the provider extension API, with pinned accounts, bounded NRQL, Nines burn-policy evaluation and setup examples.

- Add a versioned compiled telemetry-provider registry and experimental read-only Datadog count collection, with explicit query languages, tenant credential references and capability discovery.

- Add explicit GitHub review planning and immutable PR-comment publication, guarded by sharing consent, conversation and report digests, current revision and PR head checks. Approval and application remain independent Nines operations.

- Add privacy-preserving diagnostic categories for unexpected synthetic draft evaluation outcomes, without storing model or input text.

- Accept a single complete JSON code fence in portable model output mode while retaining strict decoding and review safeguards.

- Remove a misleading JSON-mode drafting example and add opt-in single-case synthetic live evaluation; deterministic safety checks remain unchanged.

- Publish a versioned OpenAPI 3.1 contract for core SLO and reviewed intent workflows, with route and critical Go-field checks. Other endpoints and full resource schemas remain outside the initial contract.

- Add a practical HTTP API guide, repository instructions for coding agents and per-page Markdown with a copy control for documentation.

- Improve public SLO guidance, documentation canonical URLs and the LLM-readable documentation index; explain Mimir authentication and tenant mapping.
- Expand opt-in Jev acceptance with negative controls for undefined success criteria; model behavior remains subject to acceptance evaluation.

- Add audited review artifacts across HTTP, CLI, Go and read-only MCP, with a Markdown format for CI review.
- Add opt-in, tenant-scoped signed workflow notifications with retryable PostgreSQL delivery and Go receiver verification.

- Add resumable headless intent status across HTTP, CLI, SDK and read-only MCP, with explicit human gates, saved runtime outcomes and an end-to-end CLI acceptance test for independent approval/application and reviewed rollback.

- Add opt-in Jev screenshot assessments linked to exact journey revisions, contract-bound sharing consent, bounded image normalization, image-free durable receipts and separate human review through the portal, CLI and Go client.
- Require separate replay capture and tenant-sharing attestations, retain versioned consent receipts, expose operator privacy notices and capture controls, and add owner-only permanent deletion. Optional PostHog diagnostics require fresh permission and exclude private/evidence panels.

- Add reviewed tenant-owner SLO retirement across HTTP, CLI, Go client and portal, with retained historical reads, concurrent-reference fences, durable audit/tombstones and retryable owned-rule cleanup at the original trusted destination.
- Qualify observed dependency links with actual sample age, series continuity, provenance, declared-link alignment and journey steps. Add an optional instrumented HTTP chain and real exporter-outage acceptance.
- Complete operator acceptance for provider outage/credential rotation and independent severity alert lifecycles on actual Prometheus/Mimir rule output.


- Add a visual service dependency editor with revisioned positions, directed links and SLO references; discover reviewable Kubernetes candidates from local exports or opt-in tenant-scoped cluster inventory with namespace-only RBAC.

- Simplify SLI charts to measured attainment and target, retain incomplete-history qualifications, and add keyboard time inspection, equivalent data tables, readable responsive panels and accessible theme/control contrast checks.

- Package Jev playground configuration behind an explicit optional Compose override for Prometheus and Mimir, preserving key-free default startup and all tenant/rule destinations.
- Reduce documentation scripts to about 9.5 KB with independent static-page enhancement, cached chapter navigation, responsive sticky chapter menus, history restoration and CI bundle budgets.

- Keep guided SLO measurement, settings and YAML edits synchronized, preserve intent authoring context and invalidate stale reviews on tab changes. Prevent implicit replacement of custom measurements and cancel requests when editors close or tenants change.

- Add guided critical user journey definitions with tenant-scoped SLO selection, step dependencies, optional agent contracts and reviewed YAML parity. Standardize spacing, select widths and responsive dialog layout across reliability, telemetry and administration forms.

- Add a separate platform admin console with exact-identity authorization, global account and tenant-membership controls, session revocation, atomic platform audit records and persisted Jev pause/daily-request policies.

- Optional native Jev System One assessments in intent preparation: separate tenant opt-in and consent, bounded category and definition-clarity judgments, audit receipts and deterministic review gates.

- Accept equivalent integer histogram boundary labels such as `2` and `2.0` in generated latency measurements, with strict per-population evidence and duplicate rejection.

- Add an optional Go-based AI workload playground with template-generated experimental SLOs, evaluation completion coverage, linked critical user journeys, named failure scenarios and Prometheus/Mimir degradation/recovery checks.

- Add a provider-neutral template-selection acceptance suite with nine semantic/adversarial scenarios, 45 scripted protocol contracts, separate model/guardrail scoring, pinned corpus/catalog/skill identities and explicit live-run reports.

- Add bounded template discovery and rendering to agentic intent preparation, with catalog fencing, scope/commitment checks, durable template provenance and human confirmation before reviewed changes.
- Add declarative SLO extension bundles with typed inputs, policy/evaluation contracts, conformance tests, immutable lockfiles, pinned local/HTTPS/Git installation, private tenant catalogs and portal/MCP/operator rendering.

- Add a shared categorized catalog of 15 experimental SLO templates, offline CLI generation, tenant API/Go-client reads, read-only MCP rendering and a portal SLI picker. Include agent/LLM/inference guidance, 120 deterministic template/intent checks and six additional model-drafting cases.

- Add `make test-identity` and a dedicated CI job for real OpenLDAP LDAPS/StartTLS, disposable PostgreSQL and native CLI/browser authorization, including durable sessions, role changes, explicit revocation, directory failure and exact consent checks. Document the real-Entra acceptance boundary and effective SSO session lifetime.

- Enforce transaction-bound measurement-quality and error-budget safety checks before reviewed intent changes, with current reviewer authorization and separately approved, expiring corrective-work exceptions.
- Persist post-apply/rollback runtime and SLI observation, require sustained evidence, detect drift or observed regressions, and bind safety/outcome reports into the independent audit journal. Expose outcomes through the portal, CLI, Go client and read-only MCP verification.

- Add revision-bound measurement-quality reports, configurable count/freshness requirements and explicit bounded cohort inspection for Prometheus/Mimir.
- Add SLO-owned error-budget response policies, fail-closed CLI CI checks and frozen budget evidence in agent proposal reviews.
- Document provider extension interfaces and the current lack of native New Relic/Datadog adapters.

- Added optional SLO/intent lifecycle across server, CLI, portal, agent context and Kubernetes CRDs. Noncommitted objectives retain measurements and recording rules while suppressing burn alerts.
- Added per-policy minimum traffic counts for both burn windows and validation rejecting unreachable burn thresholds.
- Lifecycle edits retain measurement evidence, use revision checks and show promotion warnings in reviewed plans.
- Credited both editions of *Site Reliability Engineering* alongside Sloth and Pyrra.

- Rename the portal connection area to Telemetry connections, retaining legacy bookmarks and API/YAML contracts. Add responsive SLO timelines with shared pointer/keyboard inspection, UTC axes, dark-theme colors and an explicit-apply custom time-range picker.

- Preserve approved navigation, controls and known UI states in session replay while masking private values, with a shared server-enforced label catalog and explicit recording policy.

- Harden replay scope transitions, private-region exclusion, stored-payload integrity, structural validation and visible player failures. Prevent stale save responses from changing navigation or discarding a newer capture; document the desktop/offline release acceptance matrix.

- Add optional provider-neutral S3-compatible audit storage configuration and document Ceph RGW as the open-source target. Add disposable Ceph/OpenBao container acceptance for TLS, encryption, retry idempotency and COMPLIANCE retention.

- Add opt-in per-tenant S3 Object Lock archival with durable delivery claims, retries, exact version/content/compliance-retention verification and portal/CLI receipt status.
- Attach portable replay download links before clicking them and validate export/player size.

- Require approval notes and a different applying identity for agent-proposed SLO changes and rollbacks. Audit proposal-preparation failures and routed malformed governance requests.
- Add an independently retained tenant decision journal with filtered portal/API/CLI access, frozen proposal readback, paginated conversation exports and full NDJSON CLI exports.

- Keep replay recording controls visible on SLO detail and missing-resource pages, report recorder startup failures, and preserve CSS layout declarations while removing generated content in browser/server masking. Add a hosted session replay guide and explicit browser/export acceptance boundaries.

- Add direct recent source collection from SLO details and `nines collect --recent`, surface saved collection evidence, and browse bounded metric names/labels in the selected Prometheus or Mimir source. Report empty queries separately from connection errors without rewriting saved measurements.

- Lead SLO views with observed SLI attainment and the SLO target instead of a generic unknown health badge. Show partial-window coverage, no traffic and missing measurements explicitly, with alert state and rule activation kept separate.

- Give the portal canonical `/login` and tenant-scoped page/resource URLs, restore views with browser Back/Forward, prevent late intent-creation responses from overriding navigation, keep deep links reloadable, and reject cross-tenant navigation without replacing it with another tenant's data. Generate UUIDv4 record IDs, keep credentials out of URL/storage return state, exclude credential forms from replay and prevent referrer disclosure.

- Serve product documentation directly from the website with hierarchical chapter navigation, heading links, search, canonical example downloads and static HTML pages for GitHub Pages. Add latency-ratio, critical user journey, intent and MCP guides.
- Pin browser and linked CLI authentication sessions to their provider identity. Removed providers and changed issuer/client or LDAP identity configuration require sign-in again; secret rotation and live tenant role checks remain independent. Pre-pinning sessions require one new sign-in after upgrade.

- Automatically reconcile saved SLO recording and alert rules through owner-enabled Prometheus, Mimir or operator destinations, with persistent retry/pause controls and separate runtime evidence. The playground enables this flow without manual rule apply.

- SLOs now own structured measurements directly. Portal authoring, intent review, offline rule generation, connected telemetry verification and Kubernetes reconciliation work without creating a separate measurement binding. Measurement edits follow the SLO revision and review lifecycle.

- Allow same-name SLO measurement edits with atomic revision archives, fresh active coverage, explicit review warnings and revision-isolated Prometheus/Mimir recording rules.

- Freeze recommendation rationale and evidence into intent change sets, record reasoned approvals/rejections and audited transaction receipts, and add separately reviewed, revision-safe configuration rollback through UI, CLI and the Go client.

- Publish owner-reviewed portal SLOs through the Kubernetes operator, with persistent route pins, distinct portal/Kubernetes revisions, status readback, least-privilege RBAC and guarded withdrawal.

- Add opt-in native Prometheus reload after reviewed rule apply, separate configuration/activation/runtime results, truthful file-written status and a real shared-mount acceptance test. The playground uses this path without a separate watcher.

- First-class alpha Kubernetes operator: release binaries, Kustomize installation, separate credentials, generation-scoped provider evidence, visible blocked cleanup, real API/RBAC/GC/failover acceptance and packaged container smoke checks.

- Rule deployment ownership/write guards, bounded Mimir readback recovery, namespace-aware runtime inspection and actionable UI/CLI diagnostics.
- Revision/digest-scoped recording markers and independent query-dataset output evidence, plus real operator selector and critical/warning alert timing/recovery acceptance fixtures.

- Tag-triggered alpha release drafts for Linux/macOS on amd64/arm64, with archive verification, build metadata and binary smoke checks.
- Practical site examples for the Prometheus playground, CLI, tenant access, SLI queries, Go client and MCP, plus self-contained latency SLO and experimental intent YAML.

- Consistent `admin` / `admin` login for the in-memory demo and loopback playground, with persistent password policy unchanged.

- Prominent web-login credential reference, required environment-backed Compose database passwords and generated CI database credentials.

- Pinned Go/Python lint and formatting gates in Make/CI, checked CLI output/lock-cleanup errors and stricter test fixtures.

- Prometheus-only default playground, with Mimir available through an optional Compose override/profile.

- Consistent default UI builds across local/CI environments and release-aware MCP identification.

- Shared SVG branding across the portal, sign-in screens and GitHub Pages.
- Local-account defaults for persistent Compose and Kubernetes, private credential setup and reviewed deployment guides.

- Named burn-rate policies with independent severity, windows, alert state and history selection.
- Tenant user management, local password sign-in and session revocation on password reset.
- Managed Prometheus/Mimir data source CRUD with encrypted credentials and tenant destination boundaries.
- Playground verification of local sign-in, user/source management and critical/warning alert firing and recovery.

- Separate SLO definition, collection, rule deployment and external evaluation evidence in the UI, API and CLI.
- Self-contained Prometheus/Mimir Compose playground with synthetic traffic and burn-alert/recovery verification.

- Go 1.27.3 across source builds, CI, container builds and documentation.
- Deployment guides and single-instance Linux/Kubernetes reference configuration.
- Community acknowledgements for Sloth and Pyrra.

Initial MVP feature baseline:

- Versioned SLO and critical user journey YAML, explicit links, review and revision checks.
- Prometheus/Mimir measurements, rolling objectives, budgets, charts and burn-rate rule generation.
- Tenant-scoped API, PostgreSQL persistence, configured identity providers and roles.
- Offline/connected Cobra CLI, Go client and stdio MCP adapter.
- Experimental intent/review workflows, session replay and Kubernetes reconciliation.
- Apache-2.0 license, public module path, focused setup and operations documentation.
- Public source packaging, build metadata, dependency notices, CI and candidate artifact preparation.
- SVG branding, a prerendered product site, agent-readable site guidance and a GitHub Pages workflow.
- Patched Go minimum toolchain and networking dependency for current vulnerability findings.

See the release checklist for verification still required before tagging.
