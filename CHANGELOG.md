# Changelog

## Unreleased

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
