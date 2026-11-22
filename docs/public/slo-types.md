# SLO types

An SLO answers one question about reliability, and different questions need different SLIs. Nines expresses the SLI kinds from the [Google SRE book](https://sre.google/sre-book/service-level-objectives/), plus a few for client and agent workloads. Pick the one that matches what your users experience, then add the optional reporting that fits.

## The kinds of SLO

| Type                                                                                        | What it counts                                                       | Choose it when                                                                | Defined in             |
| ------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------- | ---------------------- |
| [Availability](measurements.md#create-an-slo-in-the-portal)                                 | The share of eligible events that succeeded.                         | Each request, job or call either succeeds or fails.                           | Guided form            |
| [Latency](latency-slos.md)                                                                  | The share of events that finished under a duration threshold.        | You have a histogram and care how fast, not only whether.                     | Guided form            |
| [Correctness](#correctness-durability-and-coverage)                                         | The share of results that passed validation.                         | A result can arrive on time and still be wrong.                               | Guided form            |
| [Durability](#correctness-durability-and-coverage)                                          | The share of stored objects verified intact.                         | Storage must keep what it acknowledged.                                       | Guided form            |
| [Freshness](#freshness)                                                                     | The share of data younger than an age threshold.                     | Stale data is the failure, as with pipelines and replicas.                    | Guided form            |
| [Coverage](#correctness-durability-and-coverage)                                            | The share of received records that were processed.                   | A job can succeed while silently skipping part of its input.                  | Guided form            |
| [Sessions](measurements.md#measure-sessions-instead-of-requests)                            | The share of sessions that did not hit a bad event.                  | A client-side signal such as a crash has no stable request denominator.       | Guided form            |
| [Good-minute](measurements.md#judge-intervals-instead-of-events)                            | The share of judged intervals whose error ratio stayed under a line. | A steady noise floor makes an event ratio too loose to catch a real incident. | Guided form (Settings) |
| [Throughput](#throughput)                                                                   | The share of intervals that processed at least a minimum volume.     | Too little work getting done is the failure.                                  | Guided form (Settings) |
| [Journey run objective](critical-user-journeys.md#measure-an-agentic-journey-from-its-runs) | The share of an agentic journey's judged runs that passed.           | An agent journey records runs against a success contract.                     | Journey form           |
| [Custom queries](slo-guide.md)                                                              | Counts from your own good and total queries or imported points.      | The guided measurement cannot express the SLI.                                | Guided form (Settings) |

Agent, LLM and inference SLOs are not a separate type. They are availability or latency SLOs started from the templates in [agent, LLM and inference SLOs](agent-slos.md).

### Which SLIs fit which system

The SRE book suggests SLIs by the kind of system. Each one is a Nines type:

| System                         | SLIs to consider                                    |
| ------------------------------ | --------------------------------------------------- |
| User-facing serving            | Availability, latency, throughput                   |
| Storage                        | Availability, latency, durability                   |
| Pipelines and batch processing | Freshness, coverage, correctness, throughput        |
| Clients and apps               | Sessions                                            |
| Any system                     | Correctness, when a fast success can still be wrong |

## See the type in the portal

- Each objective card, table row and detail page shows a badge naming what it counts, derived from its saved definition. An objective with a customer list also shows a per-customer badge.
- The objective list can be filtered by type when a workspace uses more than one.
- The portal uses the type names in the table above everywhere: the **SLO type** select in **Define objective**, badges, filters and Home.
- **Define objective** asks one question, **SLO type**, and defaults to **Availability**. The other choices are **Latency**, **Correctness**, **Durability**, **Freshness**, **Coverage**, **Sessions**, **Good-minute**, **Throughput** and **Per-customer**.
  - Each type labels its fields for what it counts. Correctness asks for the labels of correct results, durability for checks that found the object intact, coverage for processed records, and freshness for a data-age histogram and the oldest acceptable age.
  - Good-minute and throughput open **Settings** with custom queries and interval judging on. Choose the error rate for good-minute, or the minimum good events per interval for throughput.
  - **Per-customer** is an add-on, not a separate type: choosing it turns on the per-customer report for the selected type and opens it under **Optional settings**. A session SLO switches to availability, because sessions cannot be grouped by customer. **Labels** are also under **Optional settings**.
  - A journey run objective belongs to a journey, not an SLO. Set it on **Critical user journeys**, in an agentic journey: target, window and minimum judged runs.
  - **Start from a template** is collapsed. It offers only templates of the chosen SLO type, grouped by workload. For example, **Latency** offers request, inference and time-to-first-token templates, and hides **Storage**, which has none. **Freshness**, **Correctness**, **Durability** and **Coverage** each offer a pipeline or storage template. Sessions, good-minute and throughput have no templates. A template fills the form, and you adapt it.
  - The YAML equivalent of every kind is in the [guides](#the-kinds-of-slo) and in the YAML tab.
- Home counts the types in use and can open the same guide.

## Correctness, durability and coverage

These count good events over eligible events from a counter, exactly like availability. They differ in what "good" means, and the type records that meaning in the definition, the portal badge and every report.

| Type        | Eligible events                                           | Good events                              | Common mistake                                            |
| ----------- | --------------------------------------------------------- | ---------------------------------------- | --------------------------------------------------------- |
| Correctness | Every result that received a validation verdict           | Results that passed validation           | Counting only results that were checked successfully      |
| Durability  | Every scheduled integrity check of a stored object        | Checks that found the object intact      | Treating a write acknowledgement as proof of durability   |
| Coverage    | Every record received, counted once on arrival            | Records that reached the output          | Counting at the output, where dropped records are missing |

```yaml
spec:
  target: 0.999
  measurement:
    type: correctness # or durability, coverage
    metric: pipeline_records_total
    countFrom: counter
    labels:
      pipeline: billing
    successLabels:
      verdict: pass
    observationPoint: Every record receiving a validation verdict; invalid and unevaluable records are counted.
```

- `successLabels` select the good events and must add to the population labels, not repeat them.
- Unknown verdicts belong in the population as not good. A missing series leaves the SLI unknown, never perfect.
- Durability measured by sampled checks says how often checks find data intact. It does not prove that every unchecked object is intact; report check coverage separately.

## Freshness

Freshness judges a data-age histogram at a threshold, exactly like latency judges a duration histogram. Each observation is the age of a record or response when it is served or delivered, measured from when the data was produced.

```yaml
spec:
  target: 0.99
  measurement:
    type: freshness
    metric: pipeline_record_age_seconds
    labels:
      pipeline: search-index
    threshold: 5m
    observationPoint: Age of each record when served, relative to its event time.
```

- Use a classic histogram in seconds with a bucket at exactly `threshold`. Quantiles and native histograms are not supported.
- Data that never arrives produces no age observation. Pair freshness with coverage so missing data is counted.

## Throughput

Throughput judges each measurement interval by volume: an interval is good when it records at least `minGoodEvents` good events. The SLI is the share of observed intervals that were good, so a stalled or slowed pipeline spends budget even when nothing fails.

```yaml
spec:
  target: 0.99
  window: 30d
  queries:
    good: sum(increase(pipeline_records_processed_total[1m]))
    total: sum(increase(pipeline_records_received_total[1m]))
  intervals:
    minGoodEvents: 500
```

- The good-events query counts the work done in each interval. An observed interval below the minimum is bad, including one where nothing was processed. An interval with no data is not judged and stays unknown.
- `intervals` takes either `minGoodEvents` (throughput) or `errorRate` with an optional `minEvents` (good-minute), not both.
- Every interval weighs the same. If your normal volume varies, for example overnight, choose a minimum that a healthy quiet interval still meets, or use separate SLOs.
- Nines' engine and its generated recording rules judge intervals the same way; a test evaluates both against the same series with the real PromQL engine.

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

| Provider             | Event and threshold types | Sessions | Good-minute and throughput | Per-customer and cohorts | Confidence in generated rules |
| -------------------- | ------------------------- | -------- | -------------------------- | ------------------------ | ----------------------------- |
| Prometheus and Mimir | Guided form               | Guided   | Yes                        | Yes                      | Yes                           |
| Dash0                | YAML or queries           | YAML\*   | PromQL\*                   | No                       | No                            |
| VictoriaMetrics      | Queries                   | No       | PromQL†                    | No                       | No                            |
| Datadog              | Queries                   | No       | No                         | No                       | No                            |
| New Relic            | Queries                   | No       | No                         | No                       | No                            |
| Google Cloud         | Queries                   | No       | No                         | No                       | No                            |

"Event and threshold types" are availability, correctness, durability and coverage (counter ratios) and latency and freshness (histogram thresholds). A test requires every provider to accept each new type exactly where it accepts availability or latency. "Queries" means interval-count queries in the provider's own language. \* Accepted by validation, not run against a live account. † Accepted only as PromQL, not tested.

Every provider supports labels, budget response levels, the adoption review, the dependency ceiling, the target-change check, burn-alert confidence in Nines' own alert state, and journey run objectives.

- **Sessions** compile to whole-window counter reads, so they need a provider with whole-window counts and PromQL. Datadog, New Relic, VictoriaMetrics and Google Cloud accept interval-count queries only and refuse an inline measurement.
- **Good-minute and throughput** judge per-interval counts and are accepted only for PromQL. Datadog, New Relic and Google Cloud queries are in their own languages and are refused, as is MetricsQL.
- **Per-customer reports, failure concentration and peer comparison** read grouped counts, which only Prometheus and Mimir provide. Elsewhere the report says `unsupported` instead of returning partial results.
- **Labels, budget response levels, the adoption review, the dependency ceiling and the target-change check** use saved definitions and collected status, so they work with any provider that has data. **Journey run objectives** use recorded runs, not telemetry.
- **Confidence** is evaluated by Nines for any provider's collected counts. Only Prometheus and Mimir get it in generated rules. Native publication to Datadog, New Relic, Dash0 or Google Cloud creates a vendor SLO, not a vendor monitor, so no vendor alert uses it.
- **Native publication** covers plain good-over-total SLOs, including correctness, durability, coverage and freshness, which export as ratios. Sessions, good-minute and throughput SLOs cannot be exported to OpenSLO, so Nines evaluates them itself and does not publish them natively.
- **Git delivery** and **migration** work with every provider. Sloth and Pyrra queries are PromQL, so an import is only usable with a Prometheus, Mimir, Dash0 or VictoriaMetrics connection. The import dialog warns when the chosen connection cannot run PromQL.

## Choose a type

1. Start with availability. Most user-facing services need only a good-over-total ratio.
2. Add latency when speed matters to users. A fast failure and a slow success are different experiences.
3. For storage, add durability. For pipelines, use freshness, coverage and correctness, and throughput when falling behind is the failure.
4. Use sessions when the signal comes from clients and counting events would need a threshold that drifts with usage.
5. Use good-minute when a noise floor you cannot remove hides real incidents.
6. Add a per-customer report when one customer could fail while the average stays healthy.
7. Use a journey run objective for agent workflows that record runs against a contract.

Few, user-focused objectives give clearer signals than many. See [keep objectives few, user-focused and linked](slo-guide.md#keep-objectives-few-user-focused-and-linked).

## Limits

- Every kind has a guided form, and the YAML equivalent stays available. Some combinations are refused with a reason instead of partly working, such as per-customer reports on sessions, good-minute or throughput on provider-evaluated windows, and both judging rules in one `intervals`. Each guide lists what it does not support.
- The type shown in the portal is derived from the saved definition. It never infers a type from metric names.
