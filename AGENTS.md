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
- Use existing shadcn/ui components and, when present, DESIGN.md and PRODUCT.md for UI work. Preserve accessibility and responsive behavior.
