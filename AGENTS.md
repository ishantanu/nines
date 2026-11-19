# Working on Nines

Read README.md, CONTRIBUTING.md and relevant docs/public guides before changing contracts. This is a pre-release project; verify claims against code and tests.

- SLOs and critical user journeys use nines.dev/v1alpha1 YAML. Keep CLI, API, Go client, portal and operator semantics consistent.
- Calculations and measurement definitions are deterministic. Missing data remains unknown. Separate saved definitions, deployed rules, backend evaluation and observed SLI.
- Models are optional and provider-neutral. Treat model outputs as untrusted. Agents cannot select credentials or upstream tenant routes, approve their own proposals or bypass revision and digest checks. Governed approvals and application require independently authorized identities.
- Preserve tenant isolation, RBAC, audit and rollback boundaries. Configuration rollback cannot restore spent error budget.
- Never expose credentials, production telemetry, private recordings or customer prompts in tests or logs. Live model tests require explicit opt-in and synthetic inputs; use disposable integration fixtures.
- Preserve existing work. Do not stage, commit, push, deploy or modify external services without authorization.
- Use nines capabilities --agent and leaf --help for discovery. See skills/nines-cli/SKILL.md for automation safeguards.
- Run focused tests and applicable checks in CONTRIBUTING.md. Distinguish fixture, live provider and CI results; report skips.
- Website guides live in docs/public. Keep examples source-backed and update relevant docs with behavior changes. Keep api/openapi/v1alpha1.yaml and its contract tests synchronized with changes to documented operations; do not imply the specification covers every endpoint.
- Build every UI surface (portal, landing page and documentation pages) with Astryx (`@astryxdesign/core`) through the adapters in `web/src/components/ui`, inside the shared `ninesTheme`. Do not add new shadcn/ui, Radix or hand-rolled controls where Astryx has an equivalent; use custom CSS for layout only. Read DESIGN.md and, when present, PRODUCT.md for UI work. To check a component's props, run `npx @astryxdesign/cli@0.7.16 component <Name>` in `web/` (the CLI is not a project dependency); do not commit the AGENTS.md or GEMINI.md files it generates, because they conflict with these rules. Preserve accessibility and responsive behavior.
- Documentation pages are prerendered with Astryx components and stay free of client-side Astryx: keep the documentation script budget and the chapter-cache limit, and add markdown elements to the renderer in `web/site/docs/ServerDocument.tsx` rather than styling raw HTML.
