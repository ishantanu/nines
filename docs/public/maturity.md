# Feature maturity

Each Nines capability has one maturity level. The level is a compatibility promise, not a quality grade: it tells you what can change under you and how much notice you get. The portal marks experimental pages, `nines capabilities --agent` reports every level, and this page lists them all.

## Levels

| Level        | Promise                                                                                                                                                                   |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stable       | Covered by the [compatibility and deprecation policy](compatibility.md). Definitions, API fields, CLI flags and behavior change only through announced deprecation.                           |
| Beta         | Feature complete and tested end to end. From the first beta release, breaking changes go through deprecation; these features become stable with the first stable release. |
| Experimental | Opt-in and may change or be removed in any release without deprecation. Use it to evaluate, not to depend on.                                                             |

Nines has not had a stable release, so no capability is stable yet. The beta features below form the stable core: they become stable together with the first stable release, under the [compatibility and deprecation policy](compatibility.md).

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

| Feature                                                        | Level        | Turn on with                           | Before it can be beta                                                                            |
| -------------------------------------------------------------- | ------------ | -------------------------------------- | ------------------------------------------------------------------------------------------------ |
| [Create from a goal](intents.md)                               | Experimental | `NINES_FEATURES=create-from-goal`      | Freeze the intent schema and pass real-provider acceptance for model drafting (#34).             |
| [Reliability reviews](architecture.md)                         | Experimental | `NINES_FEATURES=reliability-reviews`   | Settle the review schema and its link to incidents and postmortems.                              |
| [Incident records](incidents.md)                               | Experimental | `NINES_FEATURES=incidents`             | Portal pages, postmortems and links to incident tools (#86-#90), and acceptance with a real incident workflow. |
| [Workflows and webhooks](workflow-webhooks.md)                 | Experimental | `NINES_FEATURES=workflows`             | Freeze the workflow event contract and accept it with a real webhook consumer.                   |
| [Agent evidence and visual evaluations](visual-evaluations.md) | Experimental | `NINES_FEATURES=agent-evidence`        | Freeze the run evidence schema and accept it with real agent workloads.                          |
| [Session replay](session-replay.md)                            | Experimental | `NINES_FEATURES=session-replay`        | Browser and platform compatibility, a masking and privacy review, and offline-player acceptance. |
| [Git delivery](git-delivery.md)                                | Experimental | Its own configuration                  | Acceptance against live GitHub and GitLab, not only API fakes.                                   |
| [Migration from Sloth and Pyrra](migration.md)                 | Experimental | `NINES_FEATURES=migration`             | Acceptance with real-world Sloth and Pyrra files beyond the projects' published examples.        |
| [Observed dependency evidence](service-dependencies.md)        | Experimental | `NINES_FEATURES=observed-dependencies` | Instrumentation coverage and production-scale validation.                                        |
| [Remote MCP over HTTP with OAuth](remote-mcp.md)               | Experimental | Its own configuration                  | Hosted agent-client interoperability and production issuer acceptance.                           |
| [Immutable audit archival](deployment/audit-archive.md)        | Experimental | Its own configuration                  | Production IAM and KMS, high availability and real AWS acceptance.                               |
| [GitHub review comments](github-reviews.md)                    | Experimental | Its own configuration                  | Settle together with Git delivery so one GitHub integration remains.                             |
| [SLO template extensions](slo-extensions.md)                   | Experimental | Its own configuration                  | A stable extension contract; Go extensions need a custom build today.                            |
| [OpenSLO export](openslo.md)                                   | Experimental | Its own configuration                  | Round-trip acceptance with OpenSLO tooling.                                                      |
| [Jev advice](system-one.md)                                    | Experimental | Its own configuration                  | Real-provider acceptance and calibration evidence.                                               |
| [VictoriaMetrics](provider-adapters.md)                        | Experimental | `NINES_FEATURES=victoriametrics`       | Interval and throughput acceptance on a real cluster, including MetricsQL.                       |
| [Datadog](provider-adapters.md)                                | Experimental | `NINES_FEATURES=datadog`               | Opt-in live-account acceptance for collection and native publication.                            |
| [New Relic](new-relic.md)                                      | Experimental | `NINES_FEATURES=newrelic`              | Opt-in live-account acceptance for collection and native publication.                            |
| [Dash0](dash0.md)                                              | Experimental | `NINES_FEATURES=dash0`                 | Opt-in live-account acceptance, including sessions and interval SLOs.                            |
| [Google Cloud Monitoring](google-cloud.md)                     | Experimental | `NINES_FEATURES=googlecloud`           | Opt-in live-project acceptance for collection and native publication.                            |

## Turn on experimental features

A server offers only the beta features unless you turn experimental ones on. Set `NINES_FEATURES` (or the server flag `-features`) to a comma-separated list of the IDs in the table above, or to `all`:

```sh
NINES_FEATURES=create-from-goal,session-replay nines-server ...
```

- Turned-off features disappear from the portal, and their API calls return HTTP 404 with `"code": "feature_disabled"` and the setting that turns them on.
- A tenant configured with a turned-off telemetry provider stops the server at startup with the same instruction, instead of being ignored.
- Features marked "Its own configuration" are already opt-in through their own settings, such as Git delivery or audit archival, and need no feature gate.
- The synthetic demo (`-demo`) and the Compose playground turn every experimental feature on.
- `nines capabilities --agent` lists every feature, and `GET /api/workspace` returns the ones a server has turned on.

## How a feature becomes beta

An experimental feature is promoted when all of these hold:

- Its definitions and API fields have been reviewed and frozen, and are covered by the OpenAPI contract where they are HTTP operations.
- It is tested end to end in CI, and anything that talks to an outside service has passed acceptance against the real service, not only a fake.
- Its guide documents behavior, limits and failure modes.
- It has no open P0 or P1 bugs, and at least one real deployment has used it.

Promotions and any change of level are announced in the release notes. A beta feature that cannot keep its promise is moved back to experimental only with notice in the release notes.
