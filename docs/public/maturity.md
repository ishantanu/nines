# Feature maturity

Each Nines capability has one maturity level. The level is a compatibility promise, not a quality grade: it tells you what can change under you and how much notice you get. The portal marks experimental pages, `nines capabilities --agent` reports every level, and this page lists them all.

## Levels

| Level        | Promise                                                                                                                                                                   |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stable       | Covered by the compatibility and deprecation policy. Definitions, API fields, CLI flags and behavior change only through announced deprecation.                           |
| Beta         | Feature complete and tested end to end. From the first beta release, breaking changes go through deprecation; these features become stable with the first stable release. |
| Experimental | Opt-in and may change or be removed in any release without deprecation. Use it to evaluate, not to depend on.                                                             |

Nines has not had a stable release, so no capability is stable yet. The beta features below form the stable core: they become stable together with the first stable release, under the [compatibility and deprecation policy](https://github.com/ishantanu/nines/issues/70).

## The stable core (beta)

| Feature                                                                   | Level |
| ------------------------------------------------------------------------- | ----- |
| [SLO definitions and SLO types](slo-types.md)                             | Beta  |
| [SLO lifecycle, editing and retirement](slo-retirement.md)                | Beta  |
| [SLO templates](slo-templates.md)                                         | Beta  |
| [Critical user journeys and run objectives](critical-user-journeys.md)    | Beta  |
| [Recording rules and burn-rate alerts](deployment/metrics-and-rules.md)   | Beta  |
| [Error budget policies](error-budget-policy.md)                           | Beta  |
| [Labels, per-customer reports and cohorts](measurement-quality.md)        | Beta  |
| [Declared service dependency maps](service-dependencies.md)               | Beta  |
| [Prometheus](provider-adapters.md)                                        | Beta  |
| [Grafana Mimir](provider-adapters.md)                                     | Beta  |
| [Web portal](index.md)                                                    | Beta  |
| [CLI](../cli.md)                                                          | Beta  |
| [HTTP API and Go client](api.md)                                          | Beta  |
| [MCP server over stdio](mcp.md)                                           | Beta  |
| [Local accounts, OIDC, LDAP and RBAC](deployment/identity-and-tenants.md) | Beta  |
| [Tenants and telemetry connections](configuration.md)                     | Beta  |
| [Audit log](operations.md)                                                | Beta  |
| [Kubernetes operator](deployment/operator.md)                             | Beta  |

## Experimental

Experimental features are opt-in. Turn them on deliberately, expect changes, and do not build automation that depends on their output.

| Feature                                                        | Level        | Before it can be beta                                                                            |
| -------------------------------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------ |
| [Create from a goal](intents.md)                               | Experimental | Freeze the intent schema and pass real-provider acceptance for model drafting (#34).             |
| [Reliability reviews](architecture.md)                         | Experimental | Settle the review schema and its link to incidents and postmortems.                              |
| [Workflows and webhooks](workflow-webhooks.md)                 | Experimental | Freeze the workflow event contract and accept it with a real webhook consumer.                   |
| [Agent evidence and visual evaluations](visual-evaluations.md) | Experimental | Freeze the run evidence schema and accept it with real agent workloads.                          |
| [Session replay](session-replay.md)                            | Experimental | Browser and platform compatibility, a masking and privacy review, and offline-player acceptance. |
| [Git delivery](git-delivery.md)                                | Experimental | Acceptance against live GitHub and GitLab, not only API fakes.                                   |
| [Migration from Sloth and Pyrra](migration.md)                 | Experimental | Acceptance with real-world Sloth and Pyrra files beyond the projects' published examples.        |
| [Observed dependency evidence](service-dependencies.md)        | Experimental | Instrumentation coverage and production-scale validation.                                        |
| [Remote MCP over HTTP with OAuth](remote-mcp.md)               | Experimental | Hosted agent-client interoperability and production issuer acceptance.                           |
| [Immutable audit archival](deployment/audit-archive.md)        | Experimental | Production IAM and KMS, high availability and real AWS acceptance.                               |
| [GitHub review comments](github-reviews.md)                    | Experimental | Settle together with Git delivery so one GitHub integration remains.                             |
| [SLO template extensions](slo-extensions.md)                   | Experimental | A stable extension contract; Go extensions need a custom build today.                            |
| [OpenSLO export](openslo.md)                                   | Experimental | Round-trip acceptance with OpenSLO tooling.                                                      |
| [Jev advice](system-one.md)                                    | Experimental | Real-provider acceptance and calibration evidence.                                               |
| [VictoriaMetrics](provider-adapters.md)                        | Experimental | Interval and throughput acceptance on a real cluster, including MetricsQL.                       |
| [Datadog](provider-adapters.md)                                | Experimental | Opt-in live-account acceptance for collection and native publication.                            |
| [New Relic](new-relic.md)                                      | Experimental | Opt-in live-account acceptance for collection and native publication.                            |
| [Dash0](dash0.md)                                              | Experimental | Opt-in live-account acceptance, including sessions and interval SLOs.                            |
| [Google Cloud Monitoring](google-cloud.md)                     | Experimental | Opt-in live-project acceptance for collection and native publication.                            |

## How a feature becomes beta

An experimental feature is promoted when all of these hold:

- Its definitions and API fields have been reviewed and frozen, and are covered by the OpenAPI contract where they are HTTP operations.
- It is tested end to end in CI, and anything that talks to an outside service has passed acceptance against the real service, not only a fake.
- Its guide documents behavior, limits and failure modes.
- It has no open P0 or P1 bugs, and at least one real deployment has used it.

Promotions and any change of level are announced in the release notes. A beta feature that cannot keep its promise is moved back to experimental only with notice in the release notes.
