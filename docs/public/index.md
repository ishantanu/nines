# Nines documentation

Nines manages service level objectives and critical user journeys as code.

- Define successful events, connect a supported [telemetry provider](provider-adapters.md), and inspect reliability, error budgets and burn alerts through the portal, CLI or Go client.
- Agent workflows help prepare changes; deterministic measurement and explicit review remain the basis for applying them.
- Read [measurement quality](measurement-quality.md), [error-budget policies](error-budget-policy.md) and [provider adapters](provider-adapters.md) for operational evidence and backend support.
- Configure [Google Cloud Monitoring](google-cloud.md) for project-scoped counts and native SLO revision publishing.
- Publish reviewed native SLOs through [Datadog](provider-adapters.md#publish-a-native-datadog-slo), [New Relic](new-relic.md), [Dash0](dash0.md) or [Google Cloud Monitoring](google-cloud.md). Each has explicit query/window limits; native vendor alert publication is not included.
- Configure [New Relic](new-relic.md) for event-count collection, native service levels and metric query inspection.
- Configure [Dash0](dash0.md) or review [OpenSLO interoperability](openslo.md) before publishing native objectives.

## Try the complete flow

- The [local playground](deployment/playground.md) runs Nines, PostgreSQL, Prometheus and a synthetic checkout service. It automatically deploys the sample SLO's recording and alert rules, and includes a verifier that introduces failures and checks recovery.

```sh
python3 scripts/setup-local.py --playground
docker compose -p nines-playground -f compose.playground.yaml up --build -d
```

- Open `http://127.0.0.1:18080` and sign in with **admin / admin**.
  - The setup command generates a private database password on a fresh checkout; it does not overwrite an existing `.env`.
  - These public web credentials belong only to the loopback playground.

## Choose your next step

- **Learn how to define an SLO**: [SLO guide](slo-guide.md)
- **Choose a categorized SLO starting point**: [SLI menus and templates](slo-templates.md)
- **Measure agents, LLM outputs and inference**: [AI workload SLOs](agent-slos.md)
- **Define what successful requests look like**: [SLOs and measurements](measurements.md)
- **Set a latency objective and understand its ratios**: [Latency SLOs](latency-slos.md)
- **Discover Kubernetes links and edit service maps**: [Service dependencies](service-dependencies.md)
- **Connect reliability to a user outcome**: [Critical user journeys](critical-user-journeys.md)
- **Integrate over HTTP**: [HTTP API](api.md)
- **Use the CLI without a running server**: [CLI reference](../cli.md)
- **Review an SLO proposal in a pull request**: [GitHub reviews](github-reviews.md)
- **Integrate CI and incident workflows**: [Headless workflows](headless-workflows.md) and [signed notifications](workflow-webhooks.md)
- **Draft and review a change with an agent**: [SLO intents](intents.md)
- **Connect Prometheus or Mimir and load generated rules**: [Metrics and rules](deployment/metrics-and-rules.md)
- **Run Nines in Kubernetes**: [Server deployment](deployment/kubernetes.md) or [operator](deployment/operator.md)
- **Configure Entra SSO, LDAP and tenant roles**: [Identity and tenants](deployment/identity-and-tenants.md)

## Understand the measurements

- An SLO specifies a target and evaluation window for a measured population.
  - The SLI is the observed fraction of successful events.
  - A critical user journey describes the user outcome, its steps and the SLOs on which those steps depend.
  - Linking SLOs to a journey does not establish that every end-to-end journey succeeded.
- Nines keeps definition storage, measurement collection, rule deployment, backend evaluation and observed attainment separate. A saved definition or accepted rule upload cannot manufacture useful telemetry. Missing or insufficient evidence remains unknown.

- **Review a screenshot against a journey outcome**: [Visual journey evidence](visual-evaluations.md)

## Choose an operating model

- Use the **server** for the portal, tenant users, PostgreSQL-backed history and interactive review.
- Use the **CLI offline** for YAML validation and recording-rule generation in CI or deterministic skills. Connected commands additionally use a fixed server and tenant.
- Use the **Kubernetes operator** when Kubernetes and GitOps own the definitions. Portal-owned SLOs can also publish through the configured operator bridge.
- Use **MCP** for scoped agent tools. Models are optional; mutation permissions and human approval remain explicit.
- See [architecture](architecture.md), [installation](deployment.md) and [current limits](operations.md). Model-assisted workflows, replay and other experimental features retain their documented acceptance boundaries.
- Read the [session replay guide](session-replay.md) to capture the current tab, inspect masked playback and export a portable HTML recording. Replay remains experimental; browser acceptance is recorded separately.
- Owners can configure [immutable audit archival](deployment/audit-archive.md) and inspect delivery/retention receipts in Audit log or with `nines audit status`. Amazon S3 and explicitly configured S3-compatible gateways are supported by the adapter; Ceph RGW is the documented open-source target, with disposable container acceptance available through `make test-ceph`. Archival is disabled by default.
- Optional [System One advice](system-one.md) uses Jev to suggest template categories and assess definition clarity, with separate consent and audit receipts.
- The [platform admin console](platform-administration.md) separates global account administration from tenant ownership, with user/session controls, Jev request policies and platform audit records.

- [Headless workflows](headless-workflows.md): operate from CLI, API and MCP with durable review gates and no required dashboard.

- [Remote MCP with OAuth](remote-mcp.md): opt-in authenticated HTTP transport, fixed-tenant bindings and client conformance limits.
