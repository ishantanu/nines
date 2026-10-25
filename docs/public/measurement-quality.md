# Measurement quality

Measured attainment and the quality of its evidence answer different questions. Nines keeps the SLI visible while explaining coverage, freshness, eligible traffic and measurement-contract verification. It does not assign a statistical confidence score.

## Read the evidence

Open an SLO to see **Measurement quality**. The report includes the evaluated cutoff, definition revision, source fingerprint, observed event count and configured count requirement. The same report is available through the Go client, `slo_quality` MCP tool and:

```sh
nines quality api --expected-revision 3
```

This reads saved evidence and makes no provider query. Its states describe quantitative evidence:

| State | Meaning |
| --- | --- |
| `usable` | The available quantitative checks pass. Observation completeness and business correctness still require review. |
| `limited` | Coverage, count requirements, collection or contract verification needs attention. |
| `unavailable` | Usable counts are absent, stale, or blocked by a measurement/source mismatch. |

Missing data and zero traffic never establish success. Counts and coverage do not establish statistical independence, representative sampling or statistical confidence. Custom queries and imported counts do not receive automatic structured metadata verification. A service-side observation cannot prove that requests never reaching the service succeeded.

Collection evidence includes an opaque source fingerprint and the contiguous time span verified from that source. Route or credential identity changes invalidate old lineage. For interval-count measurements, backfill the complete SLO window or wait for full fresh coverage before a budget gate can proceed. Whole-window collection verifies its requested full windows directly. Manual imports into a queried SLO invalidate collector lineage until re-collected. A failed query never stamps old samples with a new source identity.

## Set requirements in the SLO

Add this optional section alongside `measurement`, `target` and `policies` in an existing SLO. The portal exposes the same fields under **Measurement quality requirements**.

```yaml
spec:
  quality:
    minEvents: 100
    maxSampleAgeSeconds: 180
    cohortLabels: [region, customer_tier]
```

`minEvents` is a minimum eligible-event count for the rolling SLO window and an annotation on low-volume cohort rows. It is separate from a burn policy's `minEvents`, which gates both alert windows. Choose counts for the service; these example values are not universal defaults.

An omitted or zero sample age uses twice the measurement interval plus the collector scheduling allowance when applicable. An explicit age must be at least `stepSeconds` and no more than 31 days. These requirements create a reviewed configuration revision without reinterpreting or discarding existing samples. They also pass through intent compilation and Kubernetes CRDs.

## Inspect cohorts

Configure one to three metric labels, then choose **Inspect cohorts** or run:

```sh
nines quality api --expected-revision 3 --cohorts
```

This performs an explicit read against the configured provider using the SLO's population, rolling window and completed cutoff. Prometheus and Mimir support provider-window measurements. Interval custom queries and providers without the optional cohort capability report `unsupported`.

The report lists observed attainment and event counts for each group, ordered with missing numerators first and then lowest attainment. A healthy aggregate can therefore coexist with a failing region or user tier. Missing good-event series remain unknown; they are never zero-filled. Missing label values appear as an explicit missing-value group.

Results are limited to 100 cohorts. Higher cardinality is rejected as a complete inspection rather than presenting a partial worst-cohort ranking. Requests are time and response-size bounded. Definition or source changes during the query invalidate the response. No samples, definitions, rules or alert states are written.

Cohort estimates have no independently verified coverage or confidence interval. The report separately states whether their good/total sums reconcile with the saved aggregate. A mismatch can reflect missing numerator series, collection timing, late data or instrumentation semantics. Review it before drawing conclusions.

Cohort analysis is diagnostic. It does not silently introduce per-cohort targets or modify aggregate SLI arithmetic, burn alerts or budget decisions. Define explicit SLOs for populations that need separate operational guarantees, and link them to the relevant [critical user journey](critical-user-journeys.md).

## APIs

`GET /api/slos/{id}/quality` reads stored evidence. Add `cohorts=true` for explicit provider inspection and `expectedRevision=N` to reject stale definition reads. Existing tenant authorization applies; viewers and agents can read their tenant's evidence. Source credentials remain server-side.

See [error-budget policies](error-budget-policy.md) for operational decisions built on these checks, and [provider adapters](provider-adapters.md) for extension boundaries.
