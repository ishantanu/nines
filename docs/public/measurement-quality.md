# Measurement quality

Measured attainment and the quality of its evidence answer different questions.

- Nines keeps the SLI visible while explaining coverage, freshness, eligible traffic and measurement-contract verification.
- It does not assign a statistical confidence score.

## Read the evidence

- Open an SLO to see **Measurement quality**.
  - The report includes the evaluated cutoff, definition revision, source fingerprint, observed event count and configured count requirement.
  - The same report is available through the Go client, `slo_quality` MCP tool and:

```sh
nines quality api --expected-revision 3
```

- This reads saved evidence and makes no provider query. Its states describe quantitative evidence:

| State         | Meaning                                                                                                         |
| ------------- | --------------------------------------------------------------------------------------------------------------- |
| `usable`      | The available quantitative checks pass. Observation completeness and business correctness still require review. |
| `limited`     | Coverage, count requirements, collection or contract verification needs attention.                              |
| `unavailable` | Usable counts are absent, stale, or blocked by a measurement/source mismatch.                                   |

- Missing data and zero traffic never establish success.
  - Counts and coverage do not establish statistical independence, representative sampling or statistical confidence.
  - Custom queries and imported counts do not receive automatic structured metadata verification.
  - A service-side observation cannot prove that requests never reaching the service succeeded.
- Collection evidence includes an opaque source fingerprint and the contiguous time span verified from that source.
  - Route or credential identity changes invalidate old lineage.
  - For interval-count measurements, backfill the complete SLO window or wait for full fresh coverage before a budget gate can proceed.
  - Whole-window collection verifies its requested full windows directly.
  - Manual imports into a queried SLO invalidate collector lineage until re-collected.
  - A failed query never stamps old samples with a new source identity.

## Set requirements in the SLO

Add this optional section alongside `measurement`, `target` and `policies` in an existing SLO. The portal exposes the same fields under **Measurement quality requirements**.

```yaml
spec:
  quality:
    minEvents: 100
    maxSampleAge: 3m
    cohortLabels: [region, customer_tier]
```

- `minEvents` is a minimum eligible-event count for the rolling SLO window and an annotation on low-volume cohort rows.
  - It is separate from a burn policy's `minEvents`, which gates both alert windows.
  - Choose counts for the service; these example values are not universal defaults.
- An omitted or zero sample age uses twice the measurement interval plus the collector scheduling allowance when applicable.
  - An explicit age must be at least the SLO `step` and no more than 31 days.
  - These requirements create a reviewed configuration revision without reinterpreting or discarding existing samples.
  - They also pass through intent compilation and Kubernetes CRDs.

## Report an SLO for key customers

In the portal, open **Optional settings** in **Define objective** and choose **Add per-customer report**, or write `spec.customers` in YAML. A service-wide SLO averages every customer together. A single large customer can be failing badly while the average stays inside budget, and the average cannot tell you which. For a short list of customers who matter, add `spec.customers` to an SLO that uses an inline `measurement`.

```yaml
spec:
  customers:
    label: account
    customers:
      - value: acme-01
        name: Acme
      - value: globex-7
        target: 0.995
```

- `label` is the metric label that identifies a customer. It must vary across customers, so it cannot be a label the measurement pins in its population or uses to classify success.
- List up to 20 customers. Each `value` is the label value, `name` is an optional display name, and `target` optionally replaces the SLO target for that customer.
- Reporting is an explicit, bounded provider read, like cohort inspection: choose **Inspect customers** on the SLO page, run `nines quality ID --customers`, or call `GET /api/slos/{id}/customers`. The query matches only the listed values as exact alternatives, so cost does not grow with the number of customers you serve.
- For each customer it shows the SLI, target, burn rate and event count, next to the service-wide burn. A customer burning faster than sustainably while the service-wide burn is not is marked **hidden by the average**.
- A customer with no matching series is shown as **no data**, a missing numerator stays unknown, and a customer below the quality `minEvents` is marked low traffic. None of them is reported as healthy.
- The label values you list are part of the SLO definition, which anyone who can read the tenant can see. Use account or tenant identifiers, never personal data. Logs and audit entries refer to a customer by an opaque alias, not the raw value.
- It does not create per-customer alerts, change the SLO's own measurement, budget or alerts, or promise a customer a service level. A provider that cannot group by label reports `unsupported` rather than an empty list. Figures are observed estimates at one cutoff, and small customers are noisy.

## Inspect cohorts

Configure one to three metric labels, then choose **Inspect cohorts** or run:

```sh
nines quality api --expected-revision 3 --cohorts
```

- This performs an explicit read against the configured provider using the SLO's population, rolling window and completed cutoff. Prometheus and Mimir support provider-window measurements. Interval custom queries and providers without the optional cohort capability report `unsupported`.
- The report lists observed attainment and event counts for each group, ordered with missing numerators first and then lowest attainment.
  - A healthy aggregate can therefore coexist with a failing region or user tier.
  - Missing good-event series remain unknown; they are never zero-filled.
  - Missing label values appear as an explicit missing-value group.
- Results are limited to 100 cohorts.
  - Higher cardinality is rejected as a complete inspection rather than presenting a partial worst-cohort ranking. Requests are time and response-size bounded.
  - Definition or source changes during the query invalidate the response.
  - No samples, definitions, rules or alert states are written.
- Cohort estimates have no independently verified coverage or confidence interval.
  - The report separately states whether their good/total sums reconcile with the saved aggregate.
  - A mismatch can reflect missing numerator series, collection timing, late data or instrumentation semantics. Review it before drawing conclusions.
- Cohort analysis is diagnostic.
  - It does not silently introduce per-cohort targets or modify aggregate SLI arithmetic, burn alerts or budget decisions.
  - Define explicit SLOs for populations that need separate operational guarantees, and link them to the relevant [critical user journey](critical-user-journeys.md).

### See where failures concentrate

A single SLO percentage can look healthy while one cohort fails constantly, because the failures are a small share of all events. When cohort inspection is available, the report also shows **failure concentration** beside the aggregate.

- For each cohort it compares the share of failures with the share of traffic. A cohort with 0.05% of traffic and 70% of failures is flagged by a large ratio.
- It summarizes the spread with the largest cohort's share of failures and the number of _effective cohorts_, which is 1 divided by the sum of squared failure shares. One means a single cohort holds every failure, and four means failures are spread evenly over four cohorts. Entropy from 0 to 1 is returned too.
- The verdict is **concentrated** when the largest cohort holds at least half the failures while failing at least twice its traffic share, and **spread** otherwise. These are fixed, documented thresholds, not tuned per SLO.
- It reports nothing when the evidence is thin. With no failures there is nothing to show, with fewer than 30 failures no pattern is reported because small counts concentrate by chance, and if any cohort has an unknown numerator the result is incomplete rather than guessed.
- It describes where failures landed, not why, and only for the declared cohort labels. It is diagnostic: it never changes targets, burn alerts or budget decisions. If cohort counts do not reconcile with the saved aggregate, the result says so.
- It is part of the `quality` response (`cohorts.concentration`), available through the same CLI command, API and portal view.

### Compare cohorts with their peers

Beside failure concentration, the cohort report adds a peer comparison under `outliers`. It asks a different question from the target: is one cohort significantly worse than the rest, even while the SLO is met?

- Each cohort is compared with all other cohorts combined using a two-proportion z-test. A cohort with a z-score of −3 or lower is listed, with its reliability, its peers' reliability, its events and the score.
- Cohorts, or peer groups, with fewer than 100 events are not compared, and fewer than three cohorts is reported as too few to compare. An unknown numerator makes the whole comparison incomplete instead of guessed.
- Only cohorts worse than their peers are listed, at most ten, most severe first.
- Checking many cohorts makes some chance flags likely, so a flag is a reason to look, not proof of a fault.
- It is evidence for review. It does not alert, does not measure the SLO target and changes no rule. A relative-latency alert rule is not built.

## APIs

- `GET /api/slos/{id}/quality` reads stored evidence.
  - Add `cohorts=true` for explicit provider inspection and `expectedRevision=N` to reject stale definition reads.
  - Existing tenant authorization applies; viewers and agents can read their tenant's evidence. Source credentials remain server-side.
- See [error-budget policies](error-budget-policy.md) for operational decisions built on these checks, and [provider adapters](provider-adapters.md) for extension boundaries.
