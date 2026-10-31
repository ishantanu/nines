# SLO measurements

An SLO answers three questions: **what counts as success, how often it must succeed, and over what window**. Define these together in one `ServiceLevelObjective`.

- Its `spec.measurement` selects the metric and eligible events; the target, window and burn-rate policies describe the objective.
- A critical user journey links the SLOs that protect a user experience.
- There is no separate measurement to name, save or version in the default flow. Measurement edits create a new revision of the same SLO. [Previous revisions](slo-editing.md) retain their original definitions and collected evidence.

## Create an SLO in the portal

1. Open **Service level objectives → Define objective → Measurement**.
2. Enter the SLO identity, project, service and configured telemetry connection. Choose the metric and the labels selecting eligible events.
3. Choose successful-event labels, or a classic histogram and latency threshold. State where events are observed and what that observation misses. Set measurement and scrape intervals.
4. Set the target percentage, rolling window, lifecycle and any burn-rate alert policies. Only committed SLOs emit burn alerts; other lifecycles keep measuring. Each policy has its own severity, short and long windows, threshold and pending duration.
5. Select **Verify telemetry and review SLO**. Nines checks recent metric metadata and observed series and generates one SLO YAML definition. Nothing is saved by this preview.
6. Review the exact definition, telemetry interval and warnings, acknowledge the measurement semantics, then select **Save reviewed SLO**.
7. The telemetry connection policy controls delivery: with **automatic deployment** enabled, saving queues its rules without another apply action. Otherwise use **Rules → Apply reviewed rules**. Inspect configuration and runtime evidence separately. See [automatic deployment](deployment/automatic-rules.md).

- Edit an existing structured measurement in the same **Measurement** tab.
  - The reviewed update keeps `metadata.name`, increments its revision and resets active measurement evidence when semantics change.
  - A manual edit through this form omits earlier intent provenance; earlier revisions retain it.
  - Use an intent conversation for a new agent-assisted proposal with fresh confirmation and approval.
- See [measurement quality](measurement-quality.md) for count/freshness requirements and cohort inspection, and [error-budget response policies](error-budget-policy.md) for operational decisions.

## Lifecycle and alert eligibility

- `spec.lifecycle` accepts `experimental`, `aspirational`, `committed` or `deprecated`. Omitting it preserves committed behavior. Choose experimental while validating instrumentation, aspirational for a target you are working toward, committed for an operational commitment, and deprecated while retiring an objective.
- All four lifecycles retain SLI measurements, error-budget arithmetic, history and recording rules.
  - Only committed generates burn alert rules and emits Nines burn events.
  - Noncommitted objectives display **Alerts disabled** instead of suggesting a successful alert evaluation. Lifecycle changes create ordinary reviewed revisions.
  - They preserve measurement evidence; a change of population, source or measurement semantics still starts fresh coverage.
  - Promoting to committed produces a review warning.
- A definition save does not instantly disable alerts in an external evaluator.
  - With automatic rule delivery, wait for the revised group to be loaded and evaluated.
  - In manual mode, apply the reviewed rules.
  - Until that succeeds, the previous Prometheus/Mimir group can still alert.
  - Nines does not manage existing Alertmanager notifications or silences.
- Each burn policy may set `minEvents` to a nonnegative count.
  - Both its short and long windows must contain at least this many eligible events, in addition to sufficient coverage and nonzero traffic.
  - Omit it or use zero for the existing behavior.
  - This is a traffic guard, not a statistical confidence interval; scrape-based counter increases can be fractional estimates.
  - Choose it from the service's traffic and response needs rather than copying a universal value.

For example, add these fields to the complete YAML below:

```yaml
spec:
  lifecycle: experimental
  policies:
    - name: fast-burn
      severity: critical
      shortWindow: 5m
      longWindow: 1h
      burnRate: 14.4
      forSeconds: 120
      minCoverage: 1
      minEvents: 100
```

- This excerpt illustrates the optional fields; it is not a complete SLO.
  - Thresholds above `1 / (1 - target)` are rejected because even 100% failed events cannot reach them.
  - For a 90% target, the maximum burn rate is 10, so a copied 14.4 threshold would never fire.

The offline CLI accepts the same fields. For a single-policy starter:

```sh
nines init slo --name api --lifecycle experimental --min-events 100 --out api.slo.yaml
nines validate --offline -f api.slo.yaml
```

- Edit named `policies` in YAML; single-policy override flags deliberately reject ambiguous updates to a list. `--no-policy` removes either form. When an SLO retains intent provenance, its lifecycle and policy fields must agree with that reviewed intent.
- Historical burn crossings remain numerical previews even when lifecycle disables alerts.
  - In Nines, insufficient coverage or traffic interrupts pending and never establishes recovery of a firing episode.
  - Prometheus/Mimir use their native alert-state behavior: an absent alert expression can resolve an alert.
  - Validate external notification routing and missing-data monitoring separately.

## One YAML definition

See the complete [availability SLO](../../examples/request-success.slo.yaml) and [latency SLO](../../examples/request-latency.slo.yaml), including two severity-based burn policies. For example:

```yaml
apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: request-success
  project: production
spec:
  displayName: API request success
  service: api
  source: primary
  target: 0.999
  windowSeconds: 2419200 # Rolling 28 days
  stepSeconds: 60
  measurement:
    type: availability
    metric: http_requests_total
    labels:
      service: api
      environment: production
    successLabels:
      status: "200"
    scrapeIntervalSeconds: 15
    observationPoint: Completed API requests, including failures. Requests that never reach the server are not observed.
  policies:
    - name: fast-burn
      severity: critical
      shortWindow: 5m
      longWindow: 1h
      burnRate: 14.4
      forSeconds: 120
      minCoverage: 1
    - name: sustained-burn
      severity: warning
      shortWindow: 30m
      longWindow: 6h
      burnRate: 6
      forSeconds: 300
      minCoverage: 1
```

- `source` identifies a tenant-configured connection, not a provider URL or credential.
  - Label filters are exact string matches. `labels` selects all eligible events; `successLabels` adds the matches for good events.
  - Availability currently supports positive equality success criteria, not regex/negation.
  - Use custom queries when the measurement requires more expressive logic. `countFrom: histogram` selects a classic histogram's `_count` for event counting.
  - Latency uses the exact `thresholdSeconds` bucket and `_count` total; missing buckets, quantiles and native histograms are rejected.
- `stepSeconds` must be 1–3600 and divide the rolling window.
  - A rolling window may be at most 31 days and contain at most 100,000 intervals; 28 days with 60-second intervals is 40,320 intervals.
  - The positive scrape interval must satisfy the provider-window constraints.
  - Short/long burn windows must align with the measurement interval.
  - PromQL selectors and window calculations are generated deterministically; do not supply `providerWindow`, custom queries or `measurementBinding` alongside `measurement`.

## CLI, API and agents

```sh
# Offline validation and rule generation require no server or model.
nines validate --offline -f examples/request-success.slo.yaml
nines rules generate --offline -f examples/request-success.slo.yaml --tenant production

# Connected read-only verification against the tenant's configured source.
nines telemetry verify -f examples/request-success.slo.yaml --context production --agent

# Review and apply the same SLO. Zero means create; edits need the current revision.
nines plan -f examples/request-success.slo.yaml --context production --agent
nines apply -f examples/request-success.slo.yaml --expected-revision 0 --context production --agent
```

- `POST /api/slos/compile` accepts one versioned SLO as `application/yaml` or `application/json`.
  - It returns the unchanged self-contained manifest/YAML, compiler version, measurement digest, manifest digest, bounded telemetry evidence and warnings.
  - It writes no resource and creates no hidden binding.
  - The Go client exposes `CompileMeasurement`; MCP exposes read-only `verify_slo_measurement`.
- For intent workflows, put the same `measurement` and `stepSeconds` inside the [intent](../../examples/api-success.intent.yaml).
  - The deterministic planner verifies that its outcome, entire population and latency threshold match the goal.
  - In a conversation, an owner/editor confirms the exact `measurementDigest` and `intentDigest` at the current `expectedVersion`, then separately reviews and approves the frozen change-set digest before apply.
  - A model cannot confirm a measurement or approve/apply changes.
  - Editing the goal or measurement invalidates prior selection, evidence and approval. [Governance and rollback](intent-governance.md) remain explicit.
- The operator accepts the same inline measurement in SLOs and [intent CRs](../../examples/operator/intent.yaml).
  - It renders/reconciles without reading a binding CR.
  - Standalone Kubernetes RBAC and GitOps govern changes; server conversation approvals are not implicitly attached to arbitrary GitOps edits.

## Advanced and existing definitions

- Direct `goodQuery`/`totalQuery` and `providerWindow` definitions remain available through Settings/YAML for advanced queries.
  - An explicitly authored legacy `MeasurementBinding` and revision-pinned reference remain accepted for existing shared contracts, but they are not a prerequisite and have no primary navigation entry.
  - Nothing silently creates or updates one during the default flow.
- Telemetry metadata and observed series verify instrument compatibility.
  - They do not prove business correctness, excluded requests, current traffic, complete historical coverage or end-to-end user success. Missing evidence remains unknown.
  - The observation point, retries and eligible-event semantics require human review.

## Reading observed SLI attainment

- The portal shows the measured SLI percentage next to the SLO target in objective lists, headers and linked-objective views.
  - For example, `99.950% SLI` with a `99.900% target` and `Partial window` means that the ratio is available but the full rolling window has not yet been observed.
  - The chart shows one **Measured SLI** series against the **SLO target**. It measures the rolling objective window.
  - Dashed segments mean that the window has incomplete history; solid segments have complete history.
  - Incomplete history is a qualification of that series, not a separate metric.
  - Interval SLI remains in the selected-interval details.
  - Coverage reports how much of that window contains measurements; newly created long-window SLOs can have valid ratios while history accumulates.
- `No SLI data` means usable measurements are missing. `No traffic` means samples were received but their total eligible-event count is zero, so no attainment percentage can be calculated.
  - Missing budget data is not displayed as an exhausted progress bar.
  - Alert state and recording-rule deployment are separate from measured attainment.
  - Showing partial measurements does not weaken alert coverage requirements or establish full-window compliance.

## Read measurements from a telemetry connection

- Nines reads existing application metrics through the configured Prometheus or Mimir query API.
  - The background collector runs every 15 seconds for SLOs with a source and saved measurement queries.
  - Nines is not a replacement for scraping applications or sending telemetry into those systems.
- Open an SLO and use **Source measurements → Read latest measurements** to collect a bounded recent range immediately.
  - The source, credentials and Mimir organization come from trusted server configuration.
  - The saved SLO defines the good/total selectors; the server chooses completed, step-aligned times with a one-step delay.
  - The recent range covers up to 15 minutes, or one step for larger steps, within the SLO window.
  - Collected values refresh attainment and charts; this operation does not deploy rules or edit the SLO.
- Use **Browse metrics in this source** to list up to 100 recent metric names, optionally narrowed by prefix.
  - Select a metric to inspect its observed labels.
  - The browser uses bounded 15-minute index lookups, so series presence alone does not prove fresh traffic or a valid success definition.
  - Browsing never substitutes a metric into your SLO.
  - Editors can open the existing edit/review flow after inspecting the evidence; viewers can browse but cannot collect or edit.
- If a connection works but the saved selectors match no data, the panel reports an empty result.
  - For example, the integration playground exports `playground_requests_total{code="200"}` and `playground_requests_total{code="500"}`.
  - An SLO querying `http_requests_total` will not measure that fixture.
  - Use the metric that actually represents your intended application and review its labels; do not substitute an unrelated metric merely to produce a percentage.

For a recent read from the CLI:

```sh
nines collect api-availability --recent --tenant production --agent
```

- The equivalent API call is `POST /api/slos/api-availability/collect` with `{"recent":true}`.
  - The existing explicit `from`/`to` body remains available for backfills. Do not combine the two modes.
  - Collection commits measurement data, requires an editor or owner, and still checks tenant authorization, CSRF for cookie sessions and measurement revision safety.
  - An empty successful query is not an attained SLO; zero-traffic samples also do not imply success.

### Chart accessibility

- Hover or tap the chart to inspect an observation. Keyboard users can focus the chart directly; no separate time-point scrubber is required.
  - Arrow keys select adjacent observations; Home/End select the first/last, and Page Up/Down move through the range.
  - The chart announces the selected timestamp, measured value, incomplete-window coverage when applicable, and target. A compact readout shows the selected time and values; raw per-sample tables are not part of the chart interface. UTC is retained consistently with the date-range controls.
  - Burn-rate windows use different stroke patterns as well as color; below-target complete-window values use triangular markers.
- Chart labels and controls use theme-aware colors checked against [WCAG 2.2 contrast requirements](https://www.w3.org/TR/WCAG22/#contrast-minimum).
  - Panels grow with their text rather than clipping readouts.
  - Date/time fields, focus indicators and tables remain available for keyboard use.
  - Automated semantic checks and light/dark token-contrast tests cover these chart components; they do not establish conformance of every product screen or assistive-technology combination.

## Burn alert window durations

- Prefer `shortWindow: 5m` and `longWindow: 1h` in policies. `300s` and `1h30m` are also accepted.
- Use positive whole-number `h`, `m` and `s` components. Day units, datetime strings, fractional components and subsecond units are not accepted for these windows.
- Existing `shortWindowSeconds` and `longWindowSeconds` inputs remain supported. Specify exactly one representation per window; supplying both is an error.
- YAML exports use readable duration fields. Runtime JSON responses, stored policy values and deterministic calculations retain numeric seconds. `forSeconds`, `windowSeconds` and other existing fields keep their current names and units.
- The CLI, API, MCP resource apply and Kubernetes operator use the same parser and policy constraints. Duration syntax does not change burn-rate thresholds, alignment requirements or evaluation behavior.
