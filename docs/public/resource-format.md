# Resource format (v1beta1)

SLOs, critical user journeys, measurement bindings and dependency maps are written as `nines.dev/v1beta1` YAML. It is the format the portal shows, `nines export` prints and the [examples](../../examples/request-success.slo.yaml) use.

- Write only what you decide. Window, step, scrape interval and burn-rate alerts have standard defaults, and Nines tells you which defaults it applied.
- Durations are readable: `28d`, `6h`, `5m`, `15s`, `300ms`.
- `nines.dev/v1alpha1` is still accepted and stored identically. Planning and validation add a deprecation note; nothing else changes.

## A minimal SLO

```yaml
apiVersion: nines.dev/v1beta1
kind: ServiceLevelObjective
metadata:
  name: request-success
  project: production
spec:
  displayName: API request success
  service: api
  target: 0.999
  measurement:
    type: availability
    metric: http_requests_total
    labels:
      service: api
    successLabels:
      status: "200"
    observationPoint: Completed API requests, including failures.
```

This SLO has a 28-day rolling window, one-minute steps, a 15-second scrape interval and the standard multiwindow burn-rate alerts. `nines validate --offline -f` and planning report the window and alert defaults they applied under `notes`.

## Defaults

| Field | Default | Notes |
| --- | --- | --- |
| `spec.window` | `28d` | Rolling window; at most 31 days. |
| `spec.step` | `1m` | Must divide the window. |
| `spec.connection` | `primary` | A tenant-configured connection name, never a URL or credential. |
| `measurement.scrapeInterval` | `15s` | Set it to your scrape cadence. |
| `policies[].minCoverage` | `1` | Both windows fully covered. |
| `spec.policies` | Standard alerts, below | `policies: []` turns alerting off. |

When `policies` is omitted, Nines uses the multiwindow, multi-burn-rate alerts from the [SRE workbook](https://sre.google/workbook/alerting-on-slos/):

| Name | Severity | Long window | Short window | Burn rate |
| --- | --- | --- | --- | --- |
| `fast-burn` | critical | 1h | 5m | 14.4 |
| `sustained-burn` | critical | 6h | 30m | 6 |
| `slow-burn` | warning | 3d | 6h | 1 |

A policy whose windows do not fit the SLO's window or step is left out, with a note saying why. When you list more than one policy, give each a `name` and `severity`.

## What an SLO counts

Use at most one of these:

- `measurement`: one metric family; Nines generates the queries. See [SLOs and measurements](measurements.md).
- `queries`: `good` and `total` in the connection's own query language, with optional `language`. `queries.window` reads whole-window counter increases from selectors instead of per-step queries.
- `binding`: a reference to a reviewed `MeasurementBinding`.

With none, the SLO is measured from samples pushed to the API.

`intervals` judges intervals instead of events: `errorRate` (with an optional `minEvents`) for good-minute SLOs, or `minGoodEvents` for throughput SLOs. See [SLO types](slo-types.md).

## Critical user journeys

A journey link's `dimension` follows the linked SLO's type: latency and freshness SLOs are `latency`; correctness, durability and coverage SLOs are `correctness`; others are `availability`. Omit it unless the link is `cost` or `safety`, or the SLO has no type. A dimension that contradicts a typed SLO is refused.

```yaml
apiVersion: nines.dev/v1beta1
kind: CriticalUserJourney
metadata:
  name: complete-checkout
  project: demo
spec:
  displayName: Complete checkout
  description: A customer can submit a cart, pay, and receive confirmation.
  owner: team-commerce
  criticality: critical
  slos:
    - id: api-availability
      required: true
  steps:
    - id: submit-cart
      name: Submit cart
      actor: user
      sloRefs: [api-availability]
```

`runObjective.window` and `agentic.maxDuration` are durations. Every SLO a step refers to must be listed in `slos`.

## Field changes from v1alpha1

| v1alpha1 | v1beta1 |
| --- | --- |
| `windowSeconds: 2419200` | `window: 28d` |
| `stepSeconds: 60` | `step: 1m` |
| `source` | `connection` |
| `goodQuery`, `totalQuery`, `queryLanguage` | `queries.good`, `queries.total`, `queries.language` |
| `providerWindow` | `queries.window` |
| `measurementBinding` | `binding` |
| `badInterval` | `intervals` |
| `policy` (single) or `policies` | `policies` |
| `forSeconds` | `for` |
| `measurement.thresholdSeconds` | `measurement.threshold` |
| `measurement.scrapeIntervalSeconds` | `measurement.scrapeInterval` |
| `quality.maxSampleAgeSeconds` | `quality.maxSampleAge` |
| Binding `source`, `measurement`, `thresholdSeconds`, `scrapeIntervalSeconds`, `stepSeconds` | `connection`, `type`, `threshold`, `scrapeInterval`, `step` |
| Dependency map `observed.source`, `windowSeconds`, `scrapeIntervalSeconds`, `maxSampleAgeSeconds` | `observed.connection`, `window`, `scrapeInterval`, `maxSampleAge` |
| Journey `runObjective.windowSeconds`, `agentic.maxDurationSeconds` | `runObjective.window`, `agentic.maxDuration` |

v1alpha1 field names are rejected in a v1beta1 document, so a half-converted file fails instead of being partly read.

## Convert existing definitions

- Export a stored definition: `nines export slo <id>` (or `journey`, `binding`, `dependency-map`). The YAML view in the portal shows the same thing.
- Exporting and applying the result changes nothing; the plan reports `unchanged`.
- To read v1alpha1 from the HTTP API, add `?apiVersion=nines.dev/v1alpha1` to a manifest request.
- Template drafts, native imports and compiled bindings are written as v1beta1. An SLO compiled from an intent stays v1alpha1, because only v1alpha1 can carry its `spec.intent` provenance.
- `nines plan` and `nines apply` send a file as written, so the server resolves journey dimensions and binding measurements against what it has stored.
- Intents, extension bundles, CLI configuration and other operational documents remain `nines.dev/v1alpha1`.
