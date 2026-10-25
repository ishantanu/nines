# Changelog

## Unreleased

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
