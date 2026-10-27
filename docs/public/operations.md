# Calculation and operational limits

An interval sample represents good and total event counts over `(T-step, T]`, not a cumulative counter or arbitrary rate. Timestamps are aligned to UTC steps.

- Counts must be finite and nonnegative, with good no greater than total. Reimporting a timestamp replaces its counts.
- For observed traffic, reliability is `sum(good) / sum(total)`.
  - Error budget uses observed total and `1-target`; burn rate divides observed error ratio by `1-target`. Missing data remains unknown.
  - Zero traffic is not reported as 100% success.
  - Coverage and negative remaining budgets remain visible.
  - Targets are fractions strictly between zero and one.
- Rolling windows use `(at-window, at]`, with up to 31 days and bounded retention.
  - Two rolling windows of interval samples are retained.
  - Current history uses the current SLO configuration; an explicit archived revision uses its original definition and retained evidence.
  - Increasing a window does not restore data already pruned.
  - Measurement/source changes keep the SLO name, archive the previous revision and start fresh active measurement coverage.
  - All definition edits require the expected revision. See [SLO editing and revision history](slo-editing.md).
- Provider-window measurements evaluate configured selectors over complete windows instead of summing independently extrapolated short-window increases. Prometheus `increase()` remains a scrape-based estimate.
  - Sparse traffic, missed instrumentation, resets between scrapes, provider retention and long-range query cost remain limits.
  - A declared measurement or a model's recommendation does not prove end-to-end journey success.
- Each configured burn policy compares both short and long windows against the threshold, minimum coverage and optional `minEvents` count. Only committed SLOs emit burn alerts.
  - Other lifecycles retain measurements and recording rules. See [lifecycle and eligibility](measurements.md#lifecycle-and-alert-eligibility). Pending duration requires consecutive observations.
  - Missing data resets pending and does not resolve a firing alert.
  - Historical evaluations do not emit historical notifications.
  - Rule generation, provider acceptance, actual rule evaluation and observed SLO health are separate states.
- Review `rules plan` and its destination before `rules apply --expected-digest ...`.
  - Prometheus file output, Prometheus Operator resources and Mimir ruler datasets have different ownership and loading requirements.
  - Deploying a YAML file does not prove a Prometheus server selected it.
  - Do not let both the platform and Kubernetes operator own the same rule group.
- The MVP targets one collecting server process.
  - Reads and transactional writes are bounded, but high-cardinality scale, distributed scheduling, queued large backfills and HA operation are not release claims. [Reviewed SLO archival and deletion](slo-retirement.md) retain audit receipts and require owned-rule cleanup.
  - Calendar windows and silences are not offered.
  - Up to eight named severity policies have independent alert lifecycles; notification grouping and inhibition remain external Alertmanager configuration.
  - PostgreSQL is required for durable normal deployments.
  - Back up storage and validate restore procedures before retaining important data.
- CUJ health rolls up explicit required links conservatively; advisory links remain visible.
  - New or insufficiently observed objectives stay unknown.
  - CUJ run verdicts and inferred dependency impact do not alter SLO arithmetic.

## Read SLO charts

- The Performance tab shares one time range across SLI/target, remaining error budget and burn-rate panels.
  - Use **Time range** for relative presets or an absolute start/end, then **Apply range**.
  - Date inputs, axes and chart inspection use UTC.
  - Editing an absolute range does not query the server until applied; Cancel preserves the existing range.
  - Absolute bounds align inward to complete measurement intervals.
  - Future or invalid ranges are rejected, and the server enforces retention and its latest completed evaluation cutoff.
  - History is retained for at most two SLO windows; data availability can be shorter.
- Panels resize with their container and stack when two columns would crowd the plots.
  - Hover, tap or focus a chart and use Left/Right, Home or End to inspect the same interval across panels.
  - Missing or zero-traffic intervals remain gaps; partial rolling coverage is shown separately and does not establish full-window compliance.
  - Burn threshold crossings and recorded alert transitions remain separate from measured attainment and notification delivery.
- Manage Prometheus and Mimir under **Telemetry connections**.
  - Existing `/data-sources` bookmarks remain accepted; new navigation uses `/telemetry-connections`.
  - This is a portal terminology change; API endpoints, CLI commands and YAML `source` fields retain their contracts.
