# SLO types

An SLO answers one question about reliability, and different questions need different SLIs. Nines expresses several kinds of SLO. Pick the one that matches what your users experience, then add the optional reporting that fits.

## The kinds of SLO

| Type                                                                                        | What it counts                                                       | Choose it when                                                                | Defined in             |
| ------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------- | ---------------------- |
| [Availability](measurements.md#create-an-slo-in-the-portal)                                 | The share of eligible events that succeeded.                         | Each request, job or call either succeeds or fails.                           | Guided form            |
| [Latency](latency-slos.md)                                                                  | The share of events that finished under a duration threshold.        | You have a histogram and care how fast, not only whether.                     | Guided form            |
| [Sessions](measurements.md#measure-sessions-instead-of-requests)                            | The share of sessions that did not hit a bad event.                  | A client-side signal such as a crash has no stable request denominator.       | Guided form            |
| [Good-minute](measurements.md#judge-intervals-instead-of-events)                            | The share of judged intervals whose error ratio stayed under a line. | A steady noise floor makes an event ratio too loose to catch a real incident. | Guided form (Settings) |
| [Journey run objective](critical-user-journeys.md#measure-an-agentic-journey-from-its-runs) | The share of an agentic journey's judged runs that passed.           | An agent journey records runs against a success contract.                     | Journey form           |
| [Custom queries](slo-guide.md)                                                              | Counts from your own good and total queries or imported points.      | The guided measurement cannot express the SLI.                                | Guided form (Settings) |

Agent, LLM and inference SLOs are not a separate type. They are availability or latency SLOs started from the templates in [agent, LLM and inference SLOs](agent-slos.md).

## See the type in the portal

- Each objective card, table row and detail page shows a badge naming what it counts, derived from its saved definition. An objective with a customer list also shows a per-customer badge.
- The objective list can be filtered by type when a workspace uses more than one.
- The portal uses the type names in the table above everywhere: the **SLO type** select in **Define objective**, badges, filters and Home.
- **Define objective** asks one question, **SLO type**, and defaults to **Availability**. The other choices are **Latency**, **Sessions** and **Good-minute**. Good-minute opens **Settings** with custom queries and interval judging on, where you set the error rate and the minimum events per interval.
  - **Per-customer** and **Labels** are optional add-ons under **Optional settings**, not alternative types. Per-customer is not offered for sessions.
  - A journey run objective belongs to a journey, not an SLO. Set it on **Critical user journeys**, in an agentic journey: target, window and minimum judged runs.
  - **Start from a template** is collapsed. It offers only templates of the chosen SLO type, grouped by workload. For example, **Latency** offers request, pipeline freshness, inference and time-to-first-token templates, and hides **Storage**, which has none. Sessions and good-minute have no templates. A template fills the form, and you adapt it.
  - The YAML equivalent of every kind is in the [guides](#the-kinds-of-slo) and in the YAML tab.
- Home counts the types in use and can open the same guide.

## Add to any SLO

These do not change what an SLO counts. They add reporting or alerting to one that already exists.

| Add-on                 | What it adds                                                               | Guide                                                                                                 |
| ---------------------- | -------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Labels                 | Ownership labels with filters and roll-ups, such as team or product.       | [Classify objectives with labels](measurements.md#classify-objectives-with-labels)                    |
| Per-customer           | The same SLI for a short list of customers beside the service-wide figure. | [Report an SLO for key customers](measurement-quality.md#report-an-slo-for-key-customers)             |
| Failure concentration  | Whether failures cluster in a few cohorts.                                 | [Inspect cohorts](measurement-quality.md#inspect-cohorts)                                             |
| Peer comparison        | Cohorts significantly worse than their peers.                              | [Compare cohorts with their peers](measurement-quality.md#compare-cohorts-with-their-peers)           |
| Low-traffic confidence | Burn alerts that scale with traffic.                                       | [Measurements](measurements.md)                                                                       |
| Budget response levels | The response agreed at each level of remaining budget.                     | [Agree responses for each budget level](error-budget-policy.md#agree-responses-for-each-budget-level) |
| Dependency ceiling     | The availability a service's dependencies allow.                           | [Service dependencies](service-dependencies.md)                                                       |

## Which providers support which types

What a type needs from the telemetry provider decides where it works. The table is enforced by tests of each provider's validator and capability flags. Provider-specific setup is in [telemetry providers](provider-adapters.md).

| Provider             | Availability and latency | Sessions | Good-minute | Per-customer and cohorts | Confidence in generated rules |
| -------------------- | ------------------------ | -------- | ----------- | ------------------------ | ----------------------------- |
| Prometheus and Mimir | Guided form              | Guided   | Yes         | Yes                      | Yes                           |
| Dash0                | YAML or queries          | YAML\*   | PromQL\*    | No                       | No                            |
| VictoriaMetrics      | Queries                  | No       | PromQL†     | No                       | No                            |
| Datadog              | Queries                  | No       | No          | No                       | No                            |
| New Relic            | Queries                  | No       | No          | No                       | No                            |
| Google Cloud         | Queries                  | No       | No          | No                       | No                            |

"Queries" means interval-count queries in the provider's own language. \* Accepted by validation, not run against a live account. † Accepted only as PromQL, not tested.

Every provider supports labels, budget response levels, the adoption review, the dependency ceiling, the target-change check, burn-alert confidence in Nines' own alert state, and journey run objectives.

- **Sessions** compile to whole-window counter reads, so they need a provider with whole-window counts and PromQL. Datadog, New Relic, VictoriaMetrics and Google Cloud accept interval-count queries only and refuse an inline measurement.
- **Good-minute** judge per-interval counts and are accepted only for PromQL. Datadog, New Relic and Google Cloud queries are in their own languages and are refused, as is MetricsQL.
- **Per-customer reports, failure concentration and peer comparison** read grouped counts, which only Prometheus and Mimir provide. Elsewhere the report says `unsupported` instead of returning partial results.
- **Labels, budget response levels, the adoption review, the dependency ceiling and the target-change check** use saved definitions and collected status, so they work with any provider that has data. **Journey run objectives** use recorded runs, not telemetry.
- **Confidence** is evaluated by Nines for any provider's collected counts. Only Prometheus and Mimir get it in generated rules. Native publication to Datadog, New Relic, Dash0 or Google Cloud creates a vendor SLO, not a vendor monitor, so no vendor alert uses it.
- **Native publication** covers plain good-over-total SLOs. Sessions and good-minute SLOs cannot be exported to OpenSLO, so Nines evaluates them itself and does not publish them natively.
- **Git delivery** and **migration** work with every provider. Sloth and Pyrra queries are PromQL, so an import is only usable with a Prometheus, Mimir, Dash0 or VictoriaMetrics connection. The import dialog warns when the chosen connection cannot run PromQL.

## Choose a type

1. Start with availability. Most user-facing services need only a good-over-total ratio.
2. Add latency when speed matters to users. A fast failure and a slow success are different experiences.
3. Use sessions when the signal comes from clients and counting events would need a threshold that drifts with usage.
4. Use good-minute when a noise floor you cannot remove hides real incidents.
5. Add a per-customer report when one customer could fail while the average stays healthy.
6. Use a journey run objective for agent workflows that record runs against a contract.

Few, user-focused objectives give clearer signals than many. See [keep objectives few, user-focused and linked](slo-guide.md#keep-objectives-few-user-focused-and-linked).

## Limits

- Every kind has a guided form, and the YAML equivalent stays available. Some combinations are refused with a reason instead of partly working, such as per-customer reports on sessions and good-minute on provider-evaluated windows. Each guide lists what it does not support.
- The type shown in the portal is derived from the saved definition. It never infers a type from metric names.
