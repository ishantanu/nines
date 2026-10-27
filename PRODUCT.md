# Nines

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Nines primarily serves SRE and platform engineering teams responsible for service reliability across multiple tenants and teams. Engineers need to define objectives, verify measurements, investigate budget consumption and review proposed changes. Tenant owners manage access and trusted telemetry connections. Application teams consume the SLOs and critical user journeys relevant to their services.

Engineers also use the CLI, Go client, Kubernetes operator and MCP adapter. These are first-class product interfaces and must share the portal's resource semantics and authorization boundaries.

## Product Purpose

Manage service level objectives and critical user journeys as code. Connect measurable reliability goals to the user outcomes they protect, and make the evidence behind reliability decisions understandable.

A successful workflow lets an engineer define the eligible population and good events, verify telemetry, choose a target and window, configure burn-rate policies, inspect a change and apply the exact reviewed revision. Engineers must be able to distinguish saved configuration, provider activation, runtime evaluation and actual measured attainment.

## Positioning

Nines combines versioned SLO definitions, explicit critical user journey and journey-step links, deterministic engineering tools and agent-assisted preparation. Agents help find measurements, draft recommendations and assemble evidence. Engineers approve changes before deterministic application. Models do not approve their own proposals, determine reliability arithmetic or select credentials and tenant routes.

Provider-neutral model configuration and local models are product requirements. No OpenAI account, SDK or hosted model service is mandatory. The ordinary product and playground must remain useful without any model account.

## Operating Context

- Self-hosted evaluation and deployments, including a local Compose playground, native binaries and Kubernetes.
- An offline/connected Cobra CLI: validate YAML and generate rules without a server; authenticate to a selected server and tenant for connected operations.
- Version control and reviewed diffs are normal parts of defining reliability. The Kubernetes operator is a first-class GitOps interface.
- Portal navigation, browser history, deep links, authentication and tenant selection must behave predictably.
- Prometheus and Mimir are the current native telemetry providers. Recording and burn-rate alert rules must reach the correct trusted destination and dataset. Other providers require explicit adapter implementation and acceptance, not a marketing claim.
- Local accounts, API credentials, configured OIDC/SSO and LDAP serve different deployment needs. Platform administration is distinct from tenant permissions.

## Capabilities and Constraints

- Use the term **critical user journeys** in full. Explicit links between SLOs and journey steps are central product concepts.
- Author SLOs and critical user journeys in YAML with consistent `nines.dev/v1alpha1` versioning. JSON remains an HTTP transport format, not a competing default authoring model.
- Prefer one understandable SLO definition containing its measurement, target, window and policies. Advanced queries and optional reusable measurement contracts must not complicate the default workflow.
- Abstract ordinary measurement authoring through guided intent and templates while keeping generated queries and measurement semantics inspectable.
- Preserve revisioned same-name SLO edits, retained evidence, stale-write rejection and tenant isolation. Resource names and opaque operation identifiers serve different purposes; audit and operation identifiers use proper UUIDs.
- Separate tenants from projects. Tenant boundaries, RBAC and trusted connection configuration constrain every interface, including agents and automation.
- Review-first changes need reasons, evidence, audit records, explicit approval and separately reviewed rollback. Restoring configuration cannot recover consumed error budget.
- Support configurable objective windows, severity-specific burn-rate policies and Prometheus/Mimir rule destinations. A written YAML file alone is not a deployed or evaluating rule group.
- Service dependency maps combine reviewed relationships and qualified observed evidence. Missing instrumentation, sample continuity and service health are different facts. A dependency map does not prove causality or end-to-end journey availability.
- Agent, LLM and inference SLOs must distinguish task outcomes, evaluator coverage, latency and budget. Model confidence about definition clarity is not measured service reliability.
- Declarative SLO **extensions**, including pinned Git repositories and built-in templates, produce ordinary versioned resources. Avoid a separate privileged execution mechanism for templates.
- MCP is stdio and read-only by default. Opt-in mutations still require server authorization. Do not imply remote MCP HTTP/OAuth exists.
- Nines is a pre-release MVP under active development. Stable API compatibility, production-scale readiness and HA collection are not established claims. Model workflows and session replay remain experimental where documented.

## Brand Commitments

The product name is **Nines**. Reuse its existing SVG logo consistently across the portal, README and public website. Preserve shadcn/ui as the component foundation and the incumbent interface identity during refinements.

The voice is precise, restrained and useful to engineers. Use plain labels, familiar terminology and concrete explanations. Avoid em dashes, promotional filler, invented claims and generic AI copy. Enterprise quality means coherent navigation, consistent panels and forms, readable charts and predictable behavior.

Credit Sloth, Pyrra and both editions of *Site Reliability Engineering* in the README. Apply practical engineering recommendations without distributing the user's purchased book or copying paid source material into the public repository. Keep unrelated projects and competitor comparisons out of product documentation.

## Evidence on Hand

- Product scope, credentials, deployment boundaries and acknowledgements: `README.md`.
- Hosted documentation sources: `docs/public/`; chapter navigation: `web/site/docs/navigation.ts`.
- Portal implementation and shared controls: `web/src/`, including `web/src/components/ui/`.
- Existing identity assets and architecture illustration: `docs/public/brand/`.
- Native YAML examples and optional instrumented workloads: `examples/`.
- Reproducible acceptance fixtures: `integration/`; documented evidence and limitations: `docs/public/verification.md` and `docs/public/experimental.md`.
- Release checks and artifact provenance: `scripts/release/` and `.github/workflows/`.

Synthetic playgrounds, controlled identity fixtures and model advice are demonstrations with stated limits. They are not customer evidence, production benchmarks, provider certification or proof of task correctness. Do not invent testimonials, adoption figures, prices or readiness claims.

## Product Principles

1. Make the measurement contract understandable before asking for a target or approval.
2. Keep human review and deterministic execution separate from model recommendations.
3. Show the evidence and its limitations; preserve distinctions between missing data, partial coverage, stale samples and verified results.
4. Keep tenant permissions, credentials and trusted destinations explicit across every client.
5. Make the UI, CLI, Go client, operator and documentation express the same product model.

## Accessibility & Inclusion

WCAG accessibility, readable text and responsive charts are explicit requirements. Use the existing WCAG AA checks, including WCAG 2.2 coverage, as a verification target rather than claiming blanket certification.

Support keyboard operation, visible focus, meaningful labels, sufficient contrast, equivalent chart data tables and understandable non-color status cues. Drag-and-drop workflows need keyboard/form alternatives. Panels, headers, footers, tabs and forms should fit their viewport and maintain consistent dimensions and spacing. Dense engineering information must remain readable at narrow widths and browser zoom.
