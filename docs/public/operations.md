# Calculation and operational limits

An interval sample represents good and total event counts over `(T-step, T]`, not a cumulative counter or arbitrary rate. Timestamps are aligned to UTC steps. Counts must be finite and nonnegative, with good no greater than total. Reimporting a timestamp replaces its counts.

For observed traffic, reliability is `sum(good) / sum(total)`. Error budget uses observed total and `1-target`; burn rate divides observed error ratio by `1-target`. Missing data remains unknown. Zero traffic is not reported as 100% success. Coverage and negative remaining budgets remain visible. Targets are fractions strictly between zero and one.

Rolling windows use `(at-window, at]`, with up to 31 days and bounded retention. Two rolling windows of interval samples are retained. History uses the current SLO configuration, and increasing a window does not restore data already pruned. Measurement/source changes need a new SLO identity; target/window/policy edits require the expected revision.

Provider-window measurements evaluate configured selectors over complete windows instead of summing independently extrapolated short-window increases. Prometheus `increase()` remains a scrape-based estimate. Sparse traffic, missed instrumentation, resets between scrapes, provider retention and long-range query cost remain limits. A declared measurement or a model's recommendation does not prove end-to-end journey success.

One burn policy per SLO compares both short and long windows against the threshold and minimum coverage. Pending duration requires consecutive observations. Missing data resets pending and does not resolve a firing alert. Historical evaluations do not emit historical notifications. Rule generation, provider acceptance, actual rule evaluation and observed SLO health are separate states.

Review `rules plan` and its destination before `rules apply --expected-digest ...`. Prometheus file output, Prometheus Operator resources and Mimir ruler datasets have different ownership and loading requirements. Deploying a YAML file does not prove a Prometheus server selected it. Do not let both the platform and Kubernetes operator own the same rule group.

The MVP targets one collecting server process. Reads and transactional writes are bounded, but high-cardinality scale, distributed scheduling, queued large backfills and HA operation are not release claims. No SLO deletion API, calendar windows or silences are offered. Up to eight named severity policies have independent alert lifecycles; notification grouping and inhibition remain external Alertmanager configuration. PostgreSQL is required for durable normal deployments. Back up storage and validate restore procedures before retaining important data.

CUJ health rolls up explicit required links conservatively; advisory links remain visible. New or insufficiently observed objectives stay unknown. CUJ run verdicts and inferred dependency impact do not alter SLO arithmetic.
