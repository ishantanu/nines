# Latency SLOs

A latency SLO counts requests that completed within a duration threshold. The example below means **99.9% of the selected API requests should take at most 300 ms over a rolling 28 days**.

## Define the objective

- The metric name is the histogram's base name, without `_bucket`, `_count` or `_sum`. The source must expose a classic histogram in seconds, with the exact threshold bucket for every matching population.

<!-- example: examples/request-latency.slo.yaml -->

- [Download the complete latency SLO](../../examples/request-latency.slo.yaml).

## How the queries are derived

- `measurement` deterministically supplies the good and total populations. Explicit `goodQuery` and `totalQuery` are unnecessary and cannot be combined with this inline contract.

| Population     | Derived selector                                                                        |
| -------------- | --------------------------------------------------------------------------------------- |
| Good requests  | `http_request_duration_seconds_bucket{service="api",environment="production",le="0.3"}` |
| Total requests | `http_request_duration_seconds_count{service="api",environment="production"}`           |

- The bucket is cumulative: `le="0.3"` counts requests with durations less than or equal to 0.3 seconds. Nines selects an existing bucket; it does not configure instrumentation or create bucket boundaries.

For each evaluation window, the basic calculation is:

```promql
sum(increase(http_request_duration_seconds_bucket{
  service="api", environment="production", le="0.3"
}[1h]))
/
sum(increase(http_request_duration_seconds_count{
  service="api", environment="production"
}[1h]))
```

- This illustrates the one-hour window.
  - Generated rules additionally align the completed cutoff, validate the populations and require nonzero traffic.
  - Increases are calculated per series before summing, so counter resets and instance identity are retained.
  - Nines evaluates each long and short window independently; it does not average overlapping window ratios into the 28-day result.
- `stepSeconds: 60` controls evaluation resolution. It does not replace the rolling 28-day measurement window or the configured burn-alert windows.

## Read the recording metrics

| Metric                             | Meaning                                                         |
| ---------------------------------- | --------------------------------------------------------------- |
| `nines:sli:ratio`                  | Good requests divided by total requests for the labeled window  |
| `nines:slo_target:ratio`           | The configured target, here `0.999`                             |
| `nines:slo_burn_rate:ratio`        | `(1 - SLI) / (1 - target)`                                      |
| `nines:slo_budget_remaining:ratio` | `1 - burn rate` for that window; can be negative                |
| `nines:slo_coverage:ratio`         | Observed scrape coverage and freshness, separately from success |

- For example, an observed SLI of 99.8% against a 99.9% target produces a 2x burn rate.
  - Filter recordings by tenant, source, SLO, revision and window.
  - Query results through the provider's query API, not its `/metrics` instrumentation endpoint.

## Where p95 and p99 fit

- A percentile asks which duration covers a fraction of requests.
  - This SLO asks which fraction completed within a fixed duration.
  - Nines does not automatically create p95 or p99 recordings for this definition.

A supplementary diagnostic p99 query for a classic histogram is:

```promql
histogram_quantile(0.99,
  sum by (le) (rate(http_request_duration_seconds_bucket{
    service="api", environment="production"
  }[5m]))
)
```

- That estimates a duration in seconds. Do not substitute it for the good-event count or average per-instance percentiles to calculate the error budget. See [Prometheus histogram guidance](https://prometheus.io/docs/practices/histograms/).

## Instrumentation and coverage requirements

- The current verifier requires `_count`, the `+Inf` bucket and the exact `le="0.3"` boundary for every matching population.
  - A missing boundary is rejected rather than replaced with a nearby bucket.
  - Native histograms and summary quantiles are currently unsupported by this measurement compiler.
- The selected population includes every completed request with the declared labels, regardless of response status.
  - If success also requires a particular status, model that requirement explicitly through an appropriate measurement/query contract.
  - Review the observation point, retries and requests that never reach this server.
- See [measurements](measurements.md), [rule deployment](deployment/automatic-rules.md) and [PromQL validation](promql-validation.md).
- Integer histogram thresholds match the two equivalent canonical boundary labels, such as `le="2"` and `le="2.0"`, through a generated bounded matcher.
  - This supports legacy text exposition and Prometheus normalization without interpolation.
  - Verification requires exactly one matching threshold bucket, `_count` and `+Inf` per observed population; duplicate equivalent buckets are rejected. Other population labels retain equality matchers.
