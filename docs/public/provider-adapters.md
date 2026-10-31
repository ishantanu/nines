# Telemetry providers

Nines loads compiled Go telemetry adapters through a versioned registry. Prometheus, Mimir and experimental Datadog, New Relic, VictoriaMetrics, Dash0 and Google Cloud adapters are included. Imported/manual counts remain available. These adapters read the vendor APIs: you can still create and update SLOs in Nines, evaluate burn policies and use Nines notification delivery. Datadog, New Relic, Dash0 and Google Cloud additionally support reviewed native SLO publishing; native alert-condition publishing is not included. The other experimental adapters collect telemetry only.

## Capabilities

| Provider | Measurement API / query language | Native publication | Connection setup |
| --- | --- | --- | --- |
| Prometheus / Mimir | Prometheus HTTP API / PromQL; intervals, whole-window counts, discovery, dependencies and cohorts | Recording and burn-rate rules through files/reload, Mimir ruler or operator | Portal or tenant file |
| Datadog (experimental) | Metrics API / bounded count queries; interval counts | Metric SLO API; 7 or 30 days | Portal or tenant file; native publishing opt-in |
| New Relic (experimental) | NerdGraph / bounded NRQL event counts; interval counts | NerdGraph service-level API; existing entity required | Portal or tenant file; approved account/entity |
| VictoriaMetrics (experimental) | Prometheus-compatible API / bounded PromQL or MetricsQL; interval counter estimates | None | Portal or tenant file |
| Dash0 (experimental) | Dash0 Prometheus API / PromQL; interval and whole-window counts | Dash0 SLO API / OpenSLO; 28 days | Portal for collection; tenant file for publication |
| Google Cloud Monitoring (experimental) | Monitoring v3 / bounded DELTA-count filters; interval counts | Monitoring v3 SLO API; existing service required | Tenant file; ADC or short-lived access token |

- All configured providers appear by name in SLO connection selectors. Guided inline measurement supports Prometheus and Mimir; metric browsing/discovery is limited to Prometheus and Mimir. Use provider-specific YAML or custom count queries for other adapters. Selecting a connection does not translate a query between languages.
- The SVG and connection name beside an SLO identify **where its telemetry is read**. They do not prove a native vendor SLO exists. Open the SLO's rule/publication panel to review the destination, apply the plan and inspect publication evidence.
- Native publishing does not create Datadog monitors, New Relic alert conditions, Dash0 check rules or Google Cloud alert policies. Nines burn policies and notification delivery are separate.
- Live synthetic acceptance has exercised Datadog and New Relic collection and native SLO creation, and disposable-container acceptance has exercised VictoriaMetrics collection. Dash0 and Google Cloud are covered by synthetic API fixtures; live account acceptance remains outstanding. These checks do not certify every account, window or alert-delivery configuration.

- `nines capabilities --offline --agent` lists adapters installed in the CLI build. `GET /api/connections` returns installed provider descriptors, query languages and capability flags. Individual configured connections include the same capability information.
- Unsupported rule deployment is reported as `unsupported`, never deployed or ready. Nines can still calculate SLI, coverage, error budgets and burn alerts from collected interval counts.
- Model-assisted discovery and verified inline measurement compilation currently require Prometheus/Mimir metadata discovery. Dash0 accepts inline counter measurements through YAML, without the guided metadata-verification flow. Use explicit reviewed count queries for Datadog, New Relic, VictoriaMetrics and Google Cloud; Dash0 also accepts inline PromQL measurements.
- The [declarative extension ecosystem](slo-extensions.md) distributes measurement recipes and presets. Provider adapters implement I/O; these are separate extension points.

## Google Cloud Monitoring

- [Configure Google Cloud](google-cloud.md) for project-pinned DELTA counts, explicit ADC/access-token authentication and reviewed immutable native SLO revisions.
- The `googlecloud` query language validates a bounded Monitoring-filter subset. It does not use PromQL, MQL, automatic counter conversion or interpolation.
- Nines burn policies work from collected counts; native Google alert-policy publishing remains separate and unsupported.

## Dash0 and OpenSLO

- [Configure Dash0](dash0.md) for dataset-pinned collection and reviewed native SLO publication.
- [Export OpenSLO](openslo.md) offline with explicit warnings about omitted Nines semantics.
- Dash0 supports interval and whole-window collection plus a query probe. Discovery, cohorts, dependencies, Nines recording-rule publication and native check-rule publication are not implemented. The separate `nativeSLOPublishing` capability identifies native SLO publication; it does not imply `ruleDeployment`.

## Query validation

All SLO creation and editing paths use the same installed query validators: offline CLI validation, server plan/apply and connected agent workflows. Connection compatibility is checked separately by the server before using a provider.

| Query language | Offline validation | Runtime checks |
| --- | --- | --- |
| `promql` or omitted | Pinned Prometheus parser and instant-vector type checker | One aggregate series, aligned intervals and valid good/total counts |
| `datadog` | Bounded count grammar, exact tag filters and rollup equal to the SLO step | COUNT/RATE metadata, normalization interval, returned rollup and matched buckets |
| `nrql` | Bounded event-count grammar; no user time windows, grouping or extra statements | Pinned account, confirmed window, complete paired bucket results and no GraphQL errors |
| `googlecloud` | Exact metric/resource predicates and AND-connected label equalities | Project pinning, raw aligned DELTA count intervals, complete series/pairs and bounded pagination |
| `metricsql` | Official VictoriaMetrics parser plus a restricted counter-count AST | Complete query responses, one aggregate series and matched interval counts |

- Datadog and NRQL validation cover the supported SLO query subsets, not the vendors' complete query languages.
- MetricsQL validation requires `sum(increase(metric[STEP]))`, `sum(increase_pure(metric[STEP]))` or `sum(increase_prometheus(metric[STEP]))`. Lookbehind must be explicit, fixed and equal to `stepSeconds`. Implicit windows, interval-relative durations, `WITH`, grouping, limits, offsets, timestamp overrides, subqueries, arithmetic and other functions are refused in this initial subset.
- Syntax validation cannot establish that a metric is a counter, that filters describe the correct population, or that instrumentation is complete. Query permissions, retained data and measurement semantics still require provider evidence and engineering review.

## Manage connections in the portal

- Under **Telemetry connections**, tenant owners can add Prometheus, Mimir, Datadog, New Relic, Dash0 and VictoriaMetrics connections. Select a provider to see its credential and routing fields. Saving a connection does not probe the provider or deploy resources.
- Datadog uses an API key and a read-scoped application key. New Relic uses a user API key and an approved account ID. Dash0 uses a read bearer token and an approved dataset. Use official regional API origins.
- The administrator must configure `NINES_SOURCE_HOSTS` and `NINES_SOURCE_ENCRYPTION_KEY`. Host restrictions apply to every managed connection; provider adapters additionally enforce official HTTPS endpoints. Credentials are encrypted at rest, never returned by the API, and blank edit fields retain existing values. Partial key rotation preserves other keys; clearing required credentials requires complete replacements.
- New Relic account IDs, Dash0 datasets and VictoriaMetrics cluster routes come from the tenant's trusted `managedProviderRoutes` policy, or existing server-configured connections of that provider. They are not selected freely by models or unapproved client input. Example tenant fields:

  ```yaml
  managedProviderRoutes:
    newrelic: ["123456"]
    dash0: ["default", "production"]
    victoriametrics: ["42", "42:7"]
  ```

- Managed connections support `options.accountID` for New Relic, `options.dataset` for Dash0, and an approved `options.accountID`/`options.projectID` pair for VictoriaMetrics clusters. Referenced endpoint/account/dataset identities cannot change in place; review a new connection and updated SLO revisions. Credential rotation remains supported and changes the measurement source fingerprint.
- Choose **Manual** collection for a bounded trial. Explicit connection tests and collection still contact the provider. Automatic collection follows the normal server schedule. Only the owner can create, edit or delete connections; agent/editor/viewer roles cannot supply credentials or alter upstream routes.
- Recording-rule controls appear only for managed Prometheus and Mimir connections. Managed Datadog and New Relic connections collect telemetry for Nines SLOs and burn alerts. Managed Dash0 connections also collect telemetry; its separate reviewed native SLO publisher remains server-configured with a distinct publish credential.

## Configure Datadog

1. Choose the official HTTPS API origin for your Datadog site, such as `https://api.datadoghq.eu` or `https://api.datadoghq.com`.
2. Supply the API key and an application key through a secret manager or the server environment. Use a least-privilege service account with metric metadata and timeseries query read permissions. Never put keys in an SLO or YAML file.
3. Configure a named connection using [the tenant configuration example](../../examples/datadog/tenants.yaml):

```yaml
tenants:
  - id: production
    name: Production
    access:
      - tokenEnv: NINES_PRODUCTION_TOKEN
        role: owner
    source:
      kind: manual
    connections:
      - name: datadog-prod
        kind: datadog
        url: https://api.datadoghq.eu
        credentialsEnv:
          apiKey: DD_API_KEY
          applicationKey: DD_APP_KEY
```

4. Start the server with `TENANTS_FILE` pointing at that file, alongside your normal database and identity configuration. Environment references are resolved by the server; credentials are excluded from serialized resolved configuration and connection responses.
5. Use [the Datadog SLO example](../../examples/datadog/api-availability.slo.yaml). Replace its metrics and exact tags with your reviewed population and successful-event definitions. The total must include successful and unsuccessful eligible events.

```yaml
apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: datadog-api-availability
  project: production
spec:
  displayName: API availability
  service: api
  source: datadog-prod
  queryLanguage: datadog
  target: 0.999
  windowSeconds: 86400
  stepSeconds: 60
  goodQuery: sum:api.requests.good{service:api,env:production}.as_count().rollup(sum,60)
  totalQuery: sum:api.requests.total{service:api,env:production}.as_count().rollup(sum,60)
```

6. Validate locally, plan against the intended tenant, review and apply:

```sh
bin/nines validate --offline -f examples/datadog/api-availability.slo.yaml --agent
bin/nines plan --context production -f examples/datadog/api-availability.slo.yaml --agent
bin/nines apply --context production -f examples/datadog/api-availability.slo.yaml --expected-revision 0 --agent
bin/nines collect datadog-api-availability --recent --context production --agent
```

- `production` here is an existing CLI context selecting the intended server and tenant. Nines authentication remains separate from upstream credentials.
- The portal YAML editor, CLI and resource API share the same manifest. Tenant owners can create managed Datadog connections in the portal after the server administrator allows the API hostname. Credentials are encrypted; the form defaults hosted-provider connections to manual collection.
- The connection test validates the configured key pair through `/api/v2/validate_keys`. It does not prove metric permissions, traffic coverage or good/total semantics.

## Datadog count semantics

- Queries must be `sum:metric{exact:tags}.as_count().rollup(sum,STEP)`, with `STEP` equal to `stepSeconds`. A whole-population `{*}` filter is supported. Grouping, arithmetic, wildcard tag values, percentiles, interpolation and arbitrary functions are refused.
- Metric metadata must identify COUNT or RATE. RATE requires a positive sampling interval in metadata; a supplied interval must divide `stepSeconds`. Gauges, distribution percentiles and vendor SLO attainment percentages are not event counts and are refused. Nines computes attainment from the summed good and total events.
- The adapter uses the documented v1 timeseries query API, requesting bounded chunks of at most 500 buckets. Datadog recommends v2 for new general-purpose timeseries clients; this narrow adapter uses v1's explicit returned rollup metadata to reject coarser aggregation. See [query API](https://docs.datadoghq.com/api/latest/metrics/query-timeseries-points/) and [type modifiers](https://docs.datadoghq.com/metrics/custom_metrics/type_modifiers/).
- Bucket start timestamps are converted to Nines interval-end timestamps. Alignment, range, duplicate timestamps and the returned interval are checked. A changed rollup is refused rather than interpolated.
- Missing paired buckets stay missing. A present denominator without a matching numerator fails collection rather than becoming zero errors. Explicit zero/zero traffic is retained as zero traffic. Coverage reflects observed intervals, not a synthetic full window.
- Rate limits and provider errors fail the collection safely. There is no immediate automatic retry loop. Existing collection scheduling can try again; prior observations are not replaced by invented data.
- Endpoint paths, query parameters, redirect destinations and upstream account overrides cannot be selected by SLOs. Datadog connections accept official API origins only. The API/application key pair selects the upstream organization; use separate credentials and connections for separate organizations.
- A Datadog credential change alters its source fingerprint. Recollect and review measurement evidence after changing account credentials.

## Publish a native Datadog SLO

- Collection and native publication are independent. Creating a Nines SLO does not automatically create a Datadog SLO.
- In **Telemetry connections**, create a Datadog connection and opt into **Enable reviewed native SLO publication**. Its application key needs `slos_read` and `slos_write` in addition to collection permissions. Keys remain encrypted. For a server-configured connection, add:

  ```yaml
  rules:
    mode: native
    namespace: production
  ```

- The API/application key pair on that connection is explicitly used for publication. Choose a dedicated organization and least-privilege keys. Native publication does not accept bearer tokens, a ruler URL, an operator publisher or automatic deployment.
- Only exact **7 day** (`604800`) and **30 day** (`2592000`) rolling windows can be published. Datadog's other supported native window, 90 days, exceeds the current Nines window limit. A one-day or 28-day SLO is rejected without rewriting its window.
- Use the SLO deployment panel to review the generated native document, then choose **Publish reviewed SLO**. The CLI exposes the same reviewed operation:

  ```sh
  nines rules plan --context production --id api-availability --agent
  nines rules apply --context production --id api-availability --expected-digest REVIEWED_DIGEST --agent
  ```

- Planning checks COUNT/RATE metric metadata before publication; gauges and distributions are refused. Nines removes only the validated `.rollup(sum,STEP)` suffix from its count queries. The native numerator and denominator retain `sum`, exact tags and `.as_count()`. The fractional target is converted to a percentage. Datadog evaluates the native ratio independently; its percentage never replaces Nines interval counts or coverage.
- A successful apply returns the native ID and a Datadog link after reading back the exact stored document. Ownership and revision tags allow the mapping to be recovered by subsequent plans after a Nines restart. Native storage is separate from provider evaluation, notification delivery and Nines SLI.
- Revisions are immutable: a changed Nines definition creates another owned native SLO and retains older revisions. Nines refuses conflicting content and duplicate ownership matches. It does not overwrite or delete an existing Datadog SLO. **Verify provider removal** verifies administrator-performed removal of all owned revisions.
- Datadog does not document an idempotency key or a conditional-write precondition for this endpoint. Run one publisher for an ownership scope. Nines serializes publication through its manager and never retries a failed create automatically. If a create outcome is unverified, inspect Datadog before refreshing or retrying. Concurrent independent publishers are not an exactly-once contract.
- Native burn-rate monitors and notification destinations are **not implemented**. On-demand native attainment/history readback is available through the supported profile below; this does not verify native monitor evaluation or alert delivery. Nines burn policies and configured notifications continue to operate independently. Kubernetes operator publication of Datadog SLOs is not yet supported.
- API contract: [create](https://docs.datadoghq.com/api/latest/service-level-objectives/create-an-slo-object/), [list](https://docs.datadoghq.com/api/latest/service-level-objectives/get-all-slos/) and [metric-based SLO semantics](https://docs.datadoghq.com/service_level_objectives/metric/).

## Configure New Relic

- See the [New Relic guide](new-relic.md) for finding custom-event data, approved entity routing and reviewed native SLO publication.

1. Supply a New Relic **user API key**, not an ingest/license key, through the server environment or secret manager. Its identity must have read access to the configured account and NRQL data.
2. Configure the official NerdGraph endpoint for your region: `https://api.newrelic.com/graphql` (US), `https://api.eu.newrelic.com/graphql` (EU), or `https://api.jp.newrelic.com/graphql` (JP).
3. Pin one account in the connection's server-owned `options`. The SLO cannot select another account, key or endpoint. See [tenant configuration](../../examples/newrelic/tenants.yaml):

```yaml
tenants:
  - id: production
    name: Production
    access:
      - tokenEnv: NINES_PRODUCTION_TOKEN
        role: owner
    source:
      kind: manual
    connections:
      - name: newrelic-prod
        kind: newrelic
        url: https://api.eu.newrelic.com/graphql
        options:
          accountID: "123456"
        credentialsEnv:
          userKey: NEW_RELIC_USER_KEY
```

4. Set `TENANTS_FILE` to that file alongside your existing database and identity configuration. Replace `123456` with your actual account ID. The account must be a single positive GraphQL integer; cross-account routing is disabled.
5. Start with [availability](../../examples/newrelic/api-availability.slo.yaml) or [latency](../../examples/newrelic/api-latency.slo.yaml). These examples assume you already send unsampled `NinesRequest` events with the shown attributes. The adapter does not ingest events or install a New Relic agent.

```yaml
apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: newrelic-api-availability
  project: production
spec:
  displayName: API availability
  service: api
  source: newrelic-prod
  queryLanguage: nrql
  target: 0.999
  windowSeconds: 86400
  stepSeconds: 60
  goodQuery: SELECT count(*) FROM NinesRequest WHERE service = 'api' AND success = true
  totalQuery: SELECT count(*) FROM NinesRequest WHERE service = 'api'
```

6. Validate, plan, review and apply through the normal CLI workflow. The example files include critical and warning burn policies evaluated by Nines:

```sh
bin/nines validate --offline -f examples/newrelic/api-availability.slo.yaml --agent
bin/nines plan --context production -f examples/newrelic/api-availability.slo.yaml --agent
bin/nines apply --context production -f examples/newrelic/api-availability.slo.yaml --expected-revision 0 --agent
bin/nines collect newrelic-api-availability --recent --context production --agent
```

- The current NRQL subset accepts `SELECT count(*) FROM EventType`, optionally followed by up to 20 `AND`-connected attribute comparisons (`=`, `!=`, `<`, `<=`, `>`, `>=`). Literal values are bounded plain strings, booleans or numbers. String literals accept letters, digits, spaces and `_ . : / @ -`; quoted/backtick identifiers, escapes and other NRQL syntax are not supported in this initial adapter.
- Both queries must count the same event type. `Metric` row counts, averages, percentiles, facets, joins, subqueries, arithmetic, aliases, user-selected time windows, sliding windows and extrapolation are refused. For latency, count events meeting the reviewed duration threshold rather than averaging p95/p99 values.
- Nines owns the result alias, exact epoch-millisecond time bounds and fixed `TIMESERIES` step. One NerdGraph request contains the paired queries and at most 300 buckets per query, below New Relic's 366-bucket maximum. Returned metadata must confirm the exact time window; each bucket's start/end must match the SLO interval.
- GraphQL errors cause the whole response to fail even when partial data is present. Null result branches, any metadata messages, shifted ranges, duplicate/unaligned timestamps and mismatched count pairs are refused. Null paired counts stay missing; explicit zero/zero is zero traffic. Nines does not fill gaps.
- NRQL may return explicit zero-count buckets for windows without stored events. Such buckets cannot establish instrumentation health. The adapter counts stored records and does not detect sampling, compensate for dropped events or prove that every request became an event. Review upstream sampling, retention and loss; prefer a known unsampled population for request-count SLOs. These provider limitations are also included in Nines measurement-quality reports for portal, CLI and agent readers.
- The connection test checks key/account access only. Successful probing does not prove NRQL permissions, event coverage or measurement correctness. Rate limits, network errors and timeouts fail safely without logging keys, queries or raw vendor errors.
- Account, endpoint or key changes alter the source fingerprint. Read fresh evidence after changing connection identity.
- New Relic connections can be server-configured or created by tenant owners in the portal using approved accounts. Reviewed native service-level publication additionally requires an approved entity. Metric discovery, structured measurement compilation and New Relic alert-condition deployment are not implemented. Nines SLO lifecycle, deterministic burn policies, audit and configured notifications remain available.
- Protocol references: [NerdGraph endpoints and authentication](https://docs.newrelic.com/docs/apis/nerdgraph/get-started/introduction-new-relic-nerdgraph/), [NRQL queries](https://docs.newrelic.com/docs/apis/nerdgraph/examples/nerdgraph-nrql-tutorial/), [time range semantics](https://docs.newrelic.com/docs/nrql/using-nrql/query-time-range/), and [NRQL bucket limits](https://docs.newrelic.com/docs/nrql/nrql-syntax-clauses-functions/).

## Configure VictoriaMetrics

VictoriaMetrics is available in **Telemetry connections → Add connection**. Enter its HTTP(S) origin and an optional bearer token. Select single-node routing or a cluster route approved by the administrator in `managedProviderRoutes.victoriametrics`; existing server-configured cluster routes are also authorized. Cluster values use `accountID` or `accountID:projectID`. Host allowlisting, encrypted credentials, owner permissions and immutable referenced routes apply as for other managed connections. Basic authentication remains available through server configuration. Collection does not deploy recording rules or configure `vmalert`.


1. Use [single-node configuration](../../examples/victoriametrics/tenants.yaml) with an HTTP(S) origin such as `http://victoriametrics:8428`. The adapter adds the query API path. The unencrypted example is for a trusted local/private test network; use HTTPS for remote credentials.
2. For clusters, use [cluster configuration](../../examples/victoriametrics/tenants-cluster.yaml) with a fixed account and optional project:

```yaml
connections:
  - name: vm-prod
    kind: victoriametrics
    url: https://vmselect.example.com
    options:
      accountID: "42"
      projectID: "7"
    credentialsEnv:
      bearerToken: VM_QUERY_TOKEN
```

3. The server constructs `/select/42:7/prometheus/api/v1/query_range`. Account/project IDs must be canonical unsigned 32-bit integers; zero is allowed. Omitting `projectID` uses the account route. `projectID` requires `accountID`. Nines tenant names do not automatically map to VictoriaMetrics tenant numbers: this connection is the explicit mapping.
4. Do not include `/select/...` in the configured URL. Endpoint path/query overrides and `multitenant` routing are rejected. Incoming Nines headers and SLO query text cannot choose a different tenant route.
5. When the deployment has a bearer-authenticated gateway, supply `credentialsEnv.bearerToken`. For Basic authentication use `credentialsEnv.basicUsername` and `credentialsEnv.basicPassword`; both values are environment references. Existing server-owned `basicUsername`/`basicPasswordEnv` configuration also works. Omit credentials only for an intentionally unauthenticated local backend. Conflicting bearer/Basic settings are refused.
6. Use the [availability example](../../examples/victoriametrics/api-availability.slo.yaml) with your metric name and reviewed labels:

```yaml
apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: vm-api-availability
  project: production
spec:
  displayName: API availability
  service: api
  source: vm-prod
  queryLanguage: metricsql
  target: 0.999
  windowSeconds: 86400
  stepSeconds: 60
  goodQuery: sum(increase_prometheus(http_requests_total{service="api",code="200"}[1m]))
  totalQuery: sum(increase_prometheus(http_requests_total{service="api"}[1m]))
```

```sh
bin/nines validate --offline -f examples/victoriametrics/api-availability.slo.yaml --agent
bin/nines plan --context production -f examples/victoriametrics/api-availability.slo.yaml --agent
bin/nines apply --context production -f examples/victoriametrics/api-availability.slo.yaml --expected-revision 0 --agent
bin/nines collect vm-api-availability --recent --context production --agent
```

- The example file includes critical and warning burn policies. Nines evaluates those policies and can deliver its configured notifications. Initial VictoriaMetrics support does not deploy rules or manage `vmalert`; external deployment status is `unsupported`.
- `queryLanguage: promql` (or omitted) is also accepted and uses the existing Prometheus syntax/type checker. These queries still execute on VictoriaMetrics, so backend counter behavior can differ. Custom PromQL must return disjoint interval counts, not raw counters, rates or percentiles.
- The example uses `increase_prometheus` deliberately. VictoriaMetrics' `increase`, `increase_pure` and `increase_prometheus` have different boundary/initial-sample behavior. Choose deliberately and compare the returned estimates with known traffic. Validation does not establish equivalent values across engines.
- Every query requests `deny_partial_response=1`. Responses marked `isPartial: true`, warnings, multiple series, non-finite values, duplicate/out-of-range timestamps or unmatched good/total buckets fail collection. Missing data is not converted to zero. Returned points are sorted by timestamp.
- The connection probe runs only constant-vector queries against the configured route; it does not establish real metric coverage. Endpoint/tenant/authentication changes alter the source fingerprint. Counter and coverage limitations appear in measurement-quality reports.
- Connection creation is currently server-configured. Inline measurement compilation, discovery, whole-window collection and Kubernetes operator/vmalert publishing are not implemented for this adapter. Use explicit queries through the server/CLI/YAML editor; do not configure a rule destination on this provider.
### Run the disposable backend acceptance test

```sh
make test-victoriametrics
```

- Requires Docker and Go. Starts a digest-pinned VictoriaMetrics v1.126.0 container on a random loopback port, stores synthetic data in temporary memory and removes its own container on exit. Existing playground services are untouched.
- Exercises the actual Nines adapter and SLO service with 15-second counter samples: 90 good / 100 total per minute, 90% SLI, complete coverage, burn firing, healthy recovery, counter resets, zero traffic and missing numerator/absent telemetry behavior.
- Pins backend boundary behavior: on this fixture, `increase_pure` and `increase` return 90/100 per minute; `increase_prometheus` returns 67.5/75. The ratio matches, but count and minimum-event policies can differ. These are fixture results, not a claim of equivalent semantics across engines, scrape patterns or backend versions.
- Confirms external rule deployment remains `unsupported`. This test does not exercise `vmalert`, a multi-node cluster or vendor-hosted services.

- Protocol references: [query APIs](https://docs.victoriametrics.com/victoriametrics/url-examples/), [cluster tenants](https://docs.victoriametrics.com/cluster-victoriametrics/), and [MetricsQL differences](https://docs.victoriametrics.com/metricsql/). Offline syntax validation uses the pinned [official Go parser](https://github.com/VictoriaMetrics/metricsql).

## Add trial SLOs to an existing Mimir and Jev playground

To opt the generated Datadog connection into reviewed native publication, set `NINES_DD_NATIVE_PUBLISH=1` when running `playground-config`. This permits reviewed publication but sends no writes by itself; use a 7 or 30 day SLO and the publication workflow above.

The isolated provider fixture on port 18180 has its own memory store. Creating a resource there does not add it to the normal playground on port 18080. Selecting the generated tenant file adds Datadog UK and New Relic EU connections to an existing tenant while retaining normal local-account login, PostgreSQL, Mimir and Jev.

1. With the query credentials exported privately, generate a tenant file containing environment references. This example matches the base + Mimir + Jev combination; use the corresponding base tenant file for other combinations:

   ```sh
   export NINES_TRIAL_BASE_TENANTS=deploy/playground/tenants-mimir-jev.yaml
   export NINES_PROVIDER_TENANTS_FILE="${TMPDIR:-/tmp}/nines-playground-providers/tenants.yaml"
   export NINES_TRIAL_TENANT=mimir
   go run ./integration/providertrial playground-config
   ```

2. Select the private configuration once. The launcher remembers its path and backend; later `./playground up` calls reuse it:

   ```sh
   ./playground up --mimir --jev --providers "$NINES_PROVIDER_TENANTS_FILE"
   ```

3. To reuse a completed synthetic ingestion run without sending more telemetry, copy its run state and generate playground manifests:

   ```sh
   export NINES_TRIAL_STATE="${TMPDIR:-/tmp}/nines-playground-trial-state"
   export NINES_TRIAL_MANIFEST_DIR="${TMPDIR:-/tmp}/nines-playground-trial-manifests"
   mkdir -p "$NINES_TRIAL_STATE"
   docker cp nines-provider-trial-seed-1:/state/. "$NINES_TRIAL_STATE"
   go run ./integration/providertrial playground-manifests
   bin/nines login --method sso --server http://127.0.0.1:18080 --tenant mimir
   ```

4. Plan and apply each generated SLO, then the CUJ. `--expected-revision 0` creates resources; if a name already exists, review its current revision rather than overwriting it:

   ```sh
   bin/nines plan -f "$NINES_TRIAL_MANIFEST_DIR/datadog.slo.yaml" --server http://127.0.0.1:18080 --tenant mimir
   bin/nines apply -f "$NINES_TRIAL_MANIFEST_DIR/datadog.slo.yaml" --expected-revision 0 --server http://127.0.0.1:18080 --tenant mimir
   bin/nines plan -f "$NINES_TRIAL_MANIFEST_DIR/newrelic.slo.yaml" --server http://127.0.0.1:18080 --tenant mimir
   bin/nines apply -f "$NINES_TRIAL_MANIFEST_DIR/newrelic.slo.yaml" --expected-revision 0 --server http://127.0.0.1:18080 --tenant mimir
   bin/nines plan -f "$NINES_TRIAL_MANIFEST_DIR/request.journey.yaml" --server http://127.0.0.1:18080 --tenant mimir
   bin/nines apply -f "$NINES_TRIAL_MANIFEST_DIR/request.journey.yaml" --expected-revision 0 --server http://127.0.0.1:18080 --tenant mimir
   ```

5. Read `from` and `to` in the copied `run.json`, then explicitly collect that exact six-minute range for each SLO with `bin/nines collect ID --from RFC3339 --to RFC3339 --server http://127.0.0.1:18080 --tenant mimir`. Keep the range within the SLO's retention. Both should contain 570 good / 600 total events, or 95%, combining the failure and recovery phases.

- Refresh `http://127.0.0.1:18080/` and select **Playground Mimir**. The SLOs and CUJ now live in that tenant's PostgreSQL-backed store, with the existing username/password login.
- These connections set server-owned `collectionMode: manual`. The scheduler skips provider collection, while explicit CLI/API collection and connection tests remain available. Omission or `automatic` preserves normal scheduled collection. The primary Prometheus/Mimir connection is unchanged. Manual mode is not a quota or request cap for explicit user actions.
- The generated SLOs use a 24-hour window to retain historical test evidence, with `lifecycle: experimental` to suppress paging from synthetic tests. Six measured minutes produce **0.4167% window coverage**. This is partial data, not proof that the whole day met the objective. The clock in the normal playground advances; measurements eventually become stale or leave the window unless more data is deliberately collected.
- Selecting the provider tenant file performs no ingestion and does not require the New Relic ingest key in the Nines server. The generated tenant file contains the account route and credential environment references, never credential values. Keep it outside version control.

## Run a bounded live Datadog and New Relic test

This opt-in Compose fixture sends real synthetic telemetry to hosted trials. It uses the production Nines adapters, HTTP resource API and portal, with an ephemeral store and a fixed acceptance clock. It is not the normal playground server and does not run background collection or notification delivery.

1. Export credentials privately. Datadog UK needs `DD_API_KEY` and a `DD_APP_KEY` scoped to `metrics_read` and `timeseries_query`. New Relic EU needs `NEWRELIC_ACCOUNT_ID`, a query-capable user key in `NEWRELIC_API_KEY`, and a separate ingest/license key in `NEWRELIC_INGEST_KEY`. The compose file mounts keys as secrets, not inline YAML values. Never commit keys or run `docker compose config` into a public report.
2. Explicitly enable both providers and start the one-shot ingestion job:

   ```sh
   NINES_TRIAL_NEWRELIC=1 ./integration/providertrial/run.sh up
   ```

   Omit `NINES_TRIAL_NEWRELIC=1` for Datadog only. If Datadog has already been seeded, `./integration/providertrial/run.sh add-newrelic` adds exactly one New Relic batch without resending Datadog.

3. Once ingestion has had time to become queryable, create the SLOs and critical user journey and verify both phases:

   ```sh
   ./integration/providertrial/run.sh verify
   ./integration/providertrial/run.sh logs
   ```

   Run verification once per fresh fixture. It creates `datadog-trial-availability`, optional `newrelic-trial-availability`, and `complete-synthetic-request` in tenant `provider-trial`. The SLO target is 99%, with a three-minute rolling window, one-minute steps, and separate critical and warning burn policies. Both policies should fire for 270/300 good events and resolve for 300/300. A CUJ link records component evidence; this does not establish measured end-to-end journey success.

4. Inspect the portal at `http://127.0.0.1:18180/`. This fixture uses a generated API token rather than the normal playground's `admin/admin` login. The private token file defaults to `${TMPDIR:-/tmp}/nines-provider-trial-token`; use its value in the portal token login. For the CLI:

   ```sh
   export NINES_API_KEY="$(cat "${NINES_TRIAL_TOKEN_FILE:-${TMPDIR:-/tmp}/nines-provider-trial-token}")"
   bin/nines get slos --server http://127.0.0.1:18180 --tenant provider-trial
   bin/nines get journeys --server http://127.0.0.1:18180 --tenant provider-trial
   bin/nines status datadog-trial-availability --server http://127.0.0.1:18180 --tenant provider-trial
   bin/nines events --server http://127.0.0.1:18180 --tenant provider-trial
   ```

5. Stop only this fixture:

   ```sh
   ./integration/providertrial/run.sh down
   ```

- **Ingestion budget:** one Datadog request containing two COUNT series and twelve points; optionally one compressed New Relic request containing 600 small events. The two phases occupy six historical minutes and share a random run identifier. No user or production data is sent.
- **Query budget:** at most three collection attempts per provider per phase, with a 30-second delay between attempts. The server also enforces an overall cap of 24 Datadog and eight New Relic read requests for its lifetime. There is no automatic collector, connection polling or vendor-native SLO/alert publishing. Keep credentials restricted to the trial accounts.
- **Restart behavior:** resources are memory-only; the volume retains the synthetic run, generated manifests and one-shot markers. An ingestion attempt is never retried automatically, including after an ambiguous response. Restarting the server resets its read budget and loses saved SLOs/CUJs. The fixed acceptance clock keeps the saved result inspectable; it does not claim continuously fresh production telemetry.
- **Cleanup:** stopping the fixture does not delete vendor telemetry. Let the trial retention policy expire it. The wrapper retains the local state volume and private token file; do not delete the state and re-seed unless another billable ingestion run is intentional.

## Temporary Datadog acceptance environment

- Datadog currently offers a [14-day trial with no credit card required](https://www.datadoghq.com/free-datadog-trial/). Create a dedicated test organization through that page.
- Configure `DD_API_KEY` and `DD_APP_KEY` privately using the Datadog connection above. The application key needs metadata and timeseries read permissions. Keep ingest credentials and any production account separate from acceptance fixtures.
- Send only synthetic COUNT/RATE metrics into the test organization. Compare emitted good/total counts with collected Nines intervals, confirm rollup units and bucket boundaries, exercise zero traffic and missing data, and verify Nines burn-policy transitions.
- No trial was created and no vendor credentials were used as part of these changes. Live acceptance remains opt-in; do not paste keys into chat or commit them.

## Add a compiled provider extension

The public Go entry point is `github.com/ishantanu/nines/provider`; the current contract is `nines.dev/provider/v1alpha1`.

1. Implement `provider.Collector`: deterministic `ValidateSLO`, an opaque `SourceFingerprint`, and context-bounded `Collect` returning interval good/total `provider.Point` values.
2. For a new query language, register a pure bounded validator with `provider.RegisterQueryLanguage` before registering the adapter. Omitted `spec.queryLanguage` continues to mean PromQL. A non-PromQL language requires explicit interval queries and cannot use Prometheus measurements or bindings.
3. Register a unique provider kind, supported query languages, truthful capability descriptor and factory with `provider.Register` during host startup. Link the extension into your custom server build. The factory receives only trusted connection routing, bounded non-secret `options` and resolved credentials.
4. Optional collection-window, preview, discovery, dependency, cohort, probe and measurement-limitation interfaces remain separate. Return the actual adapter so the registry preserves these interfaces. Adapters without external rule deployment should implement `SupportsRuleDeployment() bool` returning false.
5. Configure its `kind`, `url`, optional `options` and `credentialsEnv` in server-owned tenant configuration. An uninstalled kind fails server startup. Duplicate registrations cannot replace built-ins.
6. Add contract tests for timestamp/count semantics, missing data, bounds, partial responses, rate limits, credential redaction, tenant/account routing, cancellation and configuration changes.

```go
err := provider.Register(provider.Descriptor{
    APIVersion: provider.APIVersion,
    Kind: "example",
    QueryLanguages: []string{"promql"},
    Capabilities: provider.Capabilities{IntervalCounts: true},
}, func(cfg provider.Configuration) (provider.Collector, error) {
    return newExampleAdapter(cfg)
})
```

- The snippet assumes an adapter constructor and normal Go error handling in the host.
- Registration loads compiled trusted code. It does not download executable plugins, run scripts from Git or accept an agent-selected binary. Go extensions need a custom build; compatibility across future alpha versions is not guaranteed.
- External rule generation/deployment remains Prometheus/Mimir-specific. Declaring a provider capability does not install a rule publisher.
- Agent review, RBAC, revision/digest checks, audit, measurement quality and error-budget policy operate on normalized evidence and remain shared.

## Verification limits

- Synthetic tests cover registry extension behavior, preservation of optional interfaces, YAML round trips, account credential isolation, count collection through the SLO service, New Relic burn-policy firing, missing pairs, chunk boundaries, invalid aggregation and provider failures.
- On 30 October 2026, bounded live acceptance passed against Datadog UK and New Relic EU using the Compose fixture above. Two Datadog COUNT series (twelve points) and 600 New Relic events produced exact three-bucket counts of 270/300 followed by 300/300, each at 100% coverage. Nines critical and warning burn alerts fired and resolved for both providers. The production HTTP API created two SLOs and one linked critical user journey; the existing `bin/nines` CLI independently read back the saved resources, final SLI and all eight firing/resolution events. There were no vendor-native SLO/alert writes or notification sends. This is bounded synthetic acceptance, not scale, continuous-operation, notification-delivery or business-semantics certification.
- An earlier read-only probe found that Datadog UK returns rollup intervals in seconds despite the documented milliseconds. The adapter accepts either exact requested representation; regression tests still reject finer/coarser intervals and validate millisecond point timestamps. Missing data is never filled with invented successes.
- VictoriaMetrics has real single-node container acceptance on v1.126.0, including synthetic ingestion, exact counts, counter resets, zero traffic, missing data, SLI coverage, burn firing and recovery. Cluster routing, authentication and partial-response failures have local HTTP fixture coverage; a real multi-node cluster run remains outstanding.

## Native results and reviewed import

Datadog and New Relic have three independent evidence paths:

- **Nines measurement:** collect raw good/total counts and calculate Nines attainment, coverage, error budgets and burn alerts.
- **Native publication:** review and publish an immutable vendor SLO revision, then verify its stored definition.
- **Native results:** explicitly read the current published revision's vendor history. This never supplies points to the Nines engine, changes a definition or proves notification delivery.

### Read native results

In the portal, open an SLO, select its rule/publication tab and find **Native provider results**. Choose UTC bounds and select **Read provider results**. There is no automatic polling. Native results require an exact content-verified publication of the current Nines revision; drift, duplicates or missing revisions fail closed.

```sh
# NINES_URL, NINES_TENANT and NINES_API_KEY identify your authorized workspace.
nines native results api-availability \
  --from 2026-10-30T00:00:00Z --to 2026-10-30T12:00:00Z --output json
```

- Ranges must be in the past, use whole seconds and span at most 31 days. Choose bounds containing traffic **after native publication**. A successful read with no events returns null attainment and budget, never 100%.
- **Datadog:** metric SLO history API, at most 2,000 points, provider-reported aggregate attainment and remaining budget for the requested range. Corrections are explicitly disabled. Monitor/time-slice SLOs, previews, warnings, incomplete responses and invalid count relationships are refused. Requires `slos_read`; listing also requires access to native SLO definitions.
- **New Relic:** NerdGraph reads pinned to the configured account/entity and a verified native indicator. Recognizes the current generated SUM good/valid metric profile and constructs fixed NRQL; arbitrary returned query text is never executed. At most 300 buckets in one metric request. Bounds are clipped inward to complete intervals and returned explicitly. Budget is calculated from native counts as `(attainment - target) / (1 - target)`, not represented as a separately reported vendor field. Unknown query profiles fail closed.
- New Relic's derived metrics begin after configuration and can lag by two minutes. Earlier custom events are not native history. Zero traffic stays unavailable; missing intervals remain visible as gaps.
- All ratios in API/JSON use 0..1. Budget can be negative. History points are **interval attainment**, not rolling-window compliance. Range aggregates are not full-window SLO compliance unless the selected range matches that window. Nines coverage, alert state and native notification delivery are separate.
- Each read makes bounded external requests, including current-definition verification. Avoid repeated reads on paid trials. There is no durable native-result cache or continuous vendor synchronization.

### Discover and prepare an import

```sh
nines native list datadog-production --output json
nines native list newrelic-production --output json

# Use an exact native ID from the catalog. This writes only a local YAML draft.
nines native draft datadog-production \
  --native-id YOUR_NATIVE_SLO_ID --name imported-availability \
  --project production --service api > imported-availability.slo.yaml
nines validate -f imported-availability.slo.yaml
nines plan -f imported-availability.slo.yaml
# Review the plan before applying; 0 requires a previously absent Nines name.
nines apply -f imported-availability.slo.yaml --expected-revision 0
```

- These commands require an administrator-configured **native destination**. They cannot choose vendor credentials, endpoints, accounts or associated entities.
- Datadog catalogs are bounded to fewer than 1,000 entries in the configured credential scope. New Relic lists at most 500 indicators on the configured associated entity/account. Truncated/ambiguous responses fail; unsupported entries are marked non-importable.
- Supported drafts: Datadog metric ratios with exactly one 7/30-day threshold and supported count queries; New Relic same-account, same-event COUNT good/valid definitions with one supported rolling-day objective. More complex monitor, faceted, bad-event, mixed-aggregation and time-based definitions require manual authoring.
- Drafts use `nines.dev/v1alpha1`, `lifecycle: experimental` and a 60-second collection step. They contain no burn policies. Review event semantics, telemetry sampling and time windows before committing them.
- Import means a **new Nines definition**, not adopting or synchronizing ownership of the existing vendor SLO. No vendor resource is changed. No alert policies, corrections or notification settings are copied. A later native publication creates a separately owned revision; native-results reads then require that new published mapping.
- `--agent` returns a JSON draft envelope including limitations. Go clients expose `NativeSLOResults`, `NativeSLOCatalog` and `NativeSLODraft`. MCP exposes `native_slo_results`, `list_native_slos` and `draft_native_slo_import`; all three are read-only. Existing plan/apply approval controls remain in force.

API references: [Datadog SLO history](https://docs.datadoghq.com/api/latest/service-level-objectives/get-an-slos-history/) and [New Relic service-level results](https://docs.newrelic.com/docs/service-level-management/consume-slm/). These experimental readers have synthetic fixtures and bounded live synthetic Datadog UK / New Relic EU acceptance; other account/query profiles are not certified.
