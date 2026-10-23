# SLO measurements

An SLO answers three questions: **what counts as success, how often it must succeed, and over what window**. Define these together in one `ServiceLevelObjective`. Its `spec.measurement` selects the metric and eligible events; the target, window and burn-rate policies describe the objective. A critical user journey links the SLOs that protect a user experience.

There is no separate measurement to name, save or version in the default flow. Measurement edits create a new revision of the same SLO. [Previous revisions](slo-editing.md) retain their original definitions and collected evidence.

## Create an SLO in the portal

1. Open **Service level objectives → Define objective → Measurement**.
2. Enter the SLO identity, project, service and configured data source. Choose the metric and the labels selecting eligible events.
3. Choose successful-event labels, or a classic histogram and latency threshold. State where events are observed and what that observation misses. Set measurement and scrape intervals.
4. Set the target percentage, rolling window and any burn-rate alert policies. Each policy has its own severity, short and long windows, threshold and pending duration.
5. Select **Verify telemetry and review SLO**. Nines checks recent metric metadata and observed series and generates one SLO YAML definition. Nothing is saved by this preview.
6. Review the exact definition, telemetry interval and warnings, acknowledge the measurement semantics, then select **Save reviewed SLO**.
7. The data source policy controls delivery: with **automatic deployment** enabled, saving queues its rules without another apply action. Otherwise use **Rules → Apply reviewed rules**. Inspect configuration and runtime evidence separately. See [automatic deployment](deployment/automatic-rules.md).

Edit an existing structured measurement in the same **Measurement** tab. The reviewed update keeps `metadata.name`, increments its revision and resets active measurement evidence when semantics change. A manual edit through this form omits earlier intent provenance; earlier revisions retain it. Use an intent conversation for a new agent-assisted proposal with fresh confirmation and approval.

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
      shortWindowSeconds: 300
      longWindowSeconds: 3600
      burnRate: 14.4
      forSeconds: 120
      minCoverage: 1
    - name: sustained-burn
      severity: warning
      shortWindowSeconds: 1800
      longWindowSeconds: 21600
      burnRate: 6
      forSeconds: 300
      minCoverage: 1
```

`source` identifies a tenant-configured connection, not a provider URL or credential. Label filters are exact string matches. `labels` selects all eligible events; `successLabels` adds the matches for good events. Availability currently supports positive equality success criteria, not regex/negation. Use custom queries when the measurement requires more expressive logic. `countFrom: histogram` selects a classic histogram's `_count` for event counting. Latency uses the exact `thresholdSeconds` bucket and `_count` total; missing buckets, quantiles and native histograms are rejected.

`stepSeconds` must be 1–3600 and divide the rolling window. A rolling window may be at most 31 days and contain at most 100,000 intervals; 28 days with 60-second intervals is 40,320 intervals. The positive scrape interval must satisfy the provider-window constraints. Short/long burn windows must align with the measurement interval. PromQL selectors and window calculations are generated deterministically; do not supply `providerWindow`, custom queries or `measurementBinding` alongside `measurement`.

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

`POST /api/slos/compile` accepts one versioned SLO as `application/yaml` or `application/json`. It returns the unchanged self-contained manifest/YAML, compiler version, measurement digest, manifest digest, bounded telemetry evidence and warnings. It writes no resource and creates no hidden binding. The Go client exposes `CompileMeasurement`; MCP exposes read-only `verify_slo_measurement`.

For intent workflows, put the same `measurement` and `stepSeconds` inside the [intent](../../examples/api-success.intent.yaml). The deterministic planner verifies that its outcome, entire population and latency threshold match the goal. In a conversation, an owner/editor confirms the exact `measurementDigest` and `intentDigest` at the current `expectedVersion`, then separately reviews and approves the frozen change-set digest before apply. A model cannot confirm a measurement or approve/apply changes. Editing the goal or measurement invalidates prior selection, evidence and approval. [Governance and rollback](intent-governance.md) remain explicit.

The operator accepts the same inline measurement in SLOs and [intent CRs](../../examples/operator/intent.yaml). It renders/reconciles without reading a binding CR. Standalone Kubernetes RBAC and GitOps govern changes; server conversation approvals are not implicitly attached to arbitrary GitOps edits.

## Advanced and existing definitions

Direct `goodQuery`/`totalQuery` and `providerWindow` definitions remain available through Settings/YAML for advanced queries. An explicitly authored legacy `MeasurementBinding` and revision-pinned reference remain accepted for existing shared contracts, but they are not a prerequisite and have no primary navigation entry. Nothing silently creates or updates one during the default flow.

Telemetry metadata and observed series verify instrument compatibility. They do not prove business correctness, excluded requests, current traffic, complete historical coverage or end-to-end user success. Missing evidence remains unknown. The observation point, retries and eligible-event semantics require human review.

## Reading observed SLI attainment

The portal shows the measured SLI percentage next to the SLO target in objective lists, headers and linked-objective views. For example, `99.950% SLI` with a `99.900% target` and `Partial window` means that the ratio is available but the full rolling window has not yet been observed. The SLI chart uses a dashed amber rolling line for partial windows and a solid line for a complete window. Coverage reports how much of that window contains measurements; newly created long-window SLOs can have valid ratios while history accumulates.

`No SLI data` means usable measurements are missing. `No traffic` means samples were received but their total eligible-event count is zero, so no attainment percentage can be calculated. Missing budget data is not displayed as an exhausted progress bar. Alert state and recording-rule deployment are separate from measured attainment. Showing partial measurements does not weaken alert coverage requirements or establish full-window compliance.

## Read measurements from a data source

Nines reads existing application metrics through the configured Prometheus or Mimir query API. The background collector runs every 15 seconds for SLOs with a source and saved measurement queries. Nines is not a replacement for scraping applications or sending telemetry into those systems.

Open an SLO and use **Source measurements → Read latest measurements** to collect a bounded recent range immediately. The source, credentials and Mimir organization come from trusted server configuration. The saved SLO defines the good/total selectors; the server chooses completed, step-aligned times with a one-step delay. The recent range covers up to 15 minutes, or one step for larger steps, within the SLO window. Collected values refresh attainment and charts; this operation does not deploy rules or edit the SLO.

Use **Browse metrics in this source** to list up to 100 recent metric names, optionally narrowed by prefix. Select a metric to inspect its observed labels. The browser uses bounded 15-minute index lookups, so series presence alone does not prove fresh traffic or a valid success definition. Browsing never substitutes a metric into your SLO. Editors can open the existing edit/review flow after inspecting the evidence; viewers can browse but cannot collect or edit.

If a connection works but the saved selectors match no data, the panel reports an empty result. For example, the integration playground exports `playground_requests_total{code="200"}` and `playground_requests_total{code="500"}`. An SLO querying `http_requests_total` will not measure that fixture. Use the metric that actually represents your intended application and review its labels; do not substitute an unrelated metric merely to produce a percentage.

For a recent read from the CLI:

```sh
nines collect api-availability --recent --tenant production --agent
```

The equivalent API call is `POST /api/slos/api-availability/collect` with `{"recent":true}`. The existing explicit `from`/`to` body remains available for backfills. Do not combine the two modes. Collection commits measurement data, requires an editor or owner, and still checks tenant authorization, CSRF for cookie sessions and measurement revision safety. An empty successful query is not an attained SLO; zero-traffic samples also do not imply success.
