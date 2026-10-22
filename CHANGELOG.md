# Changelog

## Unreleased

- Tag-triggered alpha release drafts for Linux/macOS on amd64/arm64, with archive verification, build metadata and binary smoke checks.
- Practical site examples for the Prometheus playground, CLI, tenant access, SLI queries, Go client and MCP, plus measurement-binding and experimental intent YAML.

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

First public MVP candidate. No public version or release tag has been published.

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
