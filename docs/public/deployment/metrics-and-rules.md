# Prometheus, Mimir and rule destinations

Nines queries your configured metrics backend and calculates SLO state. It does not replace application instrumentation, a Prometheus scraper, a remote-write pipeline or a full metrics store. The live Mimir integration fixture uses synthetic ingestion for testing; it is not a production ingestion gateway.

For a complete synthetic local flow, use the [self-contained Compose playground](playground.md). It uses Prometheus by default, with an optional Mimir profile, automatic rule deployment after SLO saves, Prometheus reload handling and a burn-alert/recovery verifier.

## Choose a source and its tenant

For a single-source evaluation, configure either `PROMETHEUS_URL` or the legacy `MIMIR_URL`/`MIMIR_TENANT_ID` variables. For named connections and rule deployment, use [recording-connections.yaml](../../../examples/recording-connections.yaml):

```yaml
tenants:
  - id: commerce
    name: Commerce
    access:
      - tokenEnv: COMMERCE_OWNER_TOKEN
        role: owner
    connections:
      - name: production
        kind: mimir
        url: https://mimir.example.com/prometheus
        mimirOrgID: commerce-production
        bearerTokenEnv: MIMIR_QUERY_TOKEN
        rules:
          namespace: nines
          rulerURL: https://mimir-ruler.example.com/prometheus
          bearerTokenEnv: MIMIR_RULES_TOKEN
```

Set every referenced credential through the server environment and mount this configuration as `TENANTS_FILE`. Select the connection with `spec.source: production` in the SLO. The administrator fixes URLs, credentials, Mimir organization and rule namespace. Neither a caller's tenant header nor model output can choose a different provider organization.

Include the deployment's Prometheus HTTP prefix in both the query and ruler URLs. The example uses `/prometheus`; the manager appends `/config/v1/rules/...` to the configured ruler base. Verify the actual gateway routes in your Mimir installation. Use a query credential and a separate scoped ruler-write credential where your gateway supports that separation.

## Automatic or manual delivery

Enable `rules.autoDeploy: true` per source to reconcile saved SLOs automatically. Direct Prometheus deployment also requires `rules.reload: true` and a shared rule directory. Mimir uses its ruler API; operator mode publishes owned CRs. The playground enables automatic mode by default. Existing sources remain manual until an owner opts in. See [automatic deployment, status and pause/resume](automatic-rules.md).

## Recording and burn-rate rules

Offline generation writes reviewed YAML without contacting a provider:

```sh
nines rules generate --offline -f examples/api-availability.slo.yaml --tenant commerce
```

For a connected stored SLO, inspect CLI help and plan against the configured destination:

```sh
nines rules plan --help
nines rules plan --context commerce --id api-availability --agent
# Review the returned destination, group and digest before applying.
nines rules apply --context commerce --id api-availability --expected-digest REVIEWED_DIGEST --agent
```

Check the SLO detail Integration status panel, or run `nines integration SLO_ID --agent`. It reports definition saved, recent provider collection, destination content, live external evaluation and query-dataset output separately. Inspection matches the loaded rule expressions, labels and interval to the current generated group. Recent successful evaluator health does not establish measurement coverage, recorded-series presence or notification delivery. Unsupported/unavailable runtime APIs remain unverified. Prometheus inspection uses its query endpoint; Mimir inspection uses its configured ruler endpoint with pinned organization and rule credentials.

The configured source, namespace and target determine where recording and alert rules are deployed. A successful apply proves provider acceptance, not that samples or alerts are being evaluated. Confirm the rule group in the intended Mimir organization/namespace, inspect evaluation errors and verify that the configured Alertmanager path receives alerts.

The MVP supports up to eight named short/long burn policies per SLO, with critical, warning or info severity. Each policy has independent thresholds, pending duration, coverage guards and alert state. The compiler deduplicates shared recording windows and labels alerts with `nines_policy` and `severity`. Use `spec.policies`; the existing single `spec.policy` remains accepted, but the two forms cannot be combined. More windows increase the generated rule count. Check Mimir rule-group limits before deployment; the synthetic playground allows 128 rules per group. Generation does not deploy alert receiver configuration or create a metrics ingestion tenant. Use [operational limits](../operations.md) when setting a retention period, long window or sparse-traffic objective.

## Prometheus rule files and Operator resources

A Prometheus connection can specify `rules.directory`. That directory must be writable by Nines and readable by the intended Prometheus server. Nines writes the files atomically. Include them in Prometheus `rule_files` through a shared mount; choose manual reload or enable the native reviewed reload below. A private file inside the Nines container is not automatically visible to Prometheus.

For Prometheus Operator output, review generated `PrometheusRule` resources and their namespace/labels against the target Prometheus selectors before applying them through your normal Kubernetes workflow. Exporting a resource does not prove the operator selected it. Do not let both Nines and the Kubernetes operator own the same group.

## Automatic backend activation

Rule generation and plan review do not write to a deployment endpoint. Manual deployment requires an owner to apply the exact reviewed digest. When the owner enables `rules.autoDeploy`, the background reconciler plans and applies the current saved revision instead; approved intent changes retain their review requirements. The apply response separates `state` (configuration persistence), `activation` (reload/ruler acknowledgement) and `evaluation` (current runtime evidence). Integration status labels matching Prometheus content **File written**, matching Mimir content **Stored in ruler**, and an operator child **Resource configured**. None of these labels means the SLO is healthy.

For standalone Prometheus, opt into native reload on the trusted connection:

```yaml
connections:
  - name: production
    kind: prometheus
    url: http://prometheus:9090
    rules:
      namespace: nines
      directory: /rules
      reload: true
```

This snippet goes inside a tenant in `TENANTS_FILE`. Mount the same directory read-only into Prometheus and configure `rule_files` to match `*.yaml`. Enable Prometheus `--web.enable-lifecycle`. Nines makes one authenticated `POST /-/reload` to that connection's query origin and HTTP prefix after the atomic write. It uses the connection's rule credential (falling back to its source credential), refuses redirects, and bounds the request. There is no caller-supplied reload URL. A tenant owner can also select **Reload Prometheus after reviewed rule apply** when adding or editing a managed Prometheus connection.

Reload affects the full Prometheus configuration. Limit access to this endpoint to trusted clients and grant the configured rule credential the required permission. A bad configuration elsewhere can cause reload to fail even if the Nines file is valid. The file is retained and the response shows the failure with corrective guidance. Nines does not claim activation, delete the file or retry the POST blindly. An ambiguous response is checked against the exact current runtime group; an already matching, successfully evaluating group skips a repeat reload on unchanged apply. Enabling reload changes the plan digest and requires a new review.

With `reload` absent or false, apply reports manual activation and still checks runtime evidence. A private file inside the Nines container will remain unloaded until Prometheus can read it. The Compose playground enables native reload and the shared mount by default; it needs no separate file watcher.

Mimir accepts the owned group through its ruler configuration API and activates it through ruler polling. Nines checks the namespace-scoped runtime API after apply; it never sends a Prometheus reload request to Mimir. Slow polling or a first evaluation can remain **Not loaded** or **Pending**. The Integration status panel checks again automatically every 15 seconds, and immediately after an apply. Ruler polling/remote-write settings remain administrator-owned.

On Kubernetes, configure the [portal publishing bridge](operator.md#publish-portal-slos-through-the-operator) to publish owner-reviewed portal SLOs to a trusted namespace, or use standalone GitOps CRs. Install the [Nines operator](operator.md) to continuously reconcile those SLO CRs into `PrometheusRule` resources or an owned Mimir group. The Prometheus Operator selects its resources and handles Prometheus activation. This route does not use standalone file reload. Keep a single owner for each group.

A saved definition, loaded rules and SLO attainment are different facts. **Unknown** SLO health can persist if measurement queries return no intervals, collection fails, samples are stale, or window coverage is insufficient. The collection and rule-output rows explain those conditions separately. Rule activation does not instrument an application, scrape its metrics or manufacture traffic coverage.

The HTTP apply endpoint can return HTTP 200 with `state: written` and `activation.state: failed`, because the file mutation succeeded. CLI/MCP consumers must inspect activation and evaluation fields before treating backend activation as successful. A successful reload acknowledgement alone is also insufficient; require current `evaluation.state: evaluating` for runtime proof and `ruleOutput.state: verified` from integration status for output visibility.

## Verify the measurement

Check the original query directly in the intended backend, then check Nines sample freshness and coverage. Confirm the error numerator, traffic denominator, selectors and instrumentation semantics. SLO YAML does not prove those metrics exist. Missing data stays unknown, and a provider accepting a query is not evidence that the query represents successful user journeys.

Nines validates input PromQL and every generated recording/alert expression offline before export or deployment. Syntax/type validation does not verify metric availability, query cost, result cardinality or backend-version support. See [PromQL validation](../promql-validation.md).

## Deployment diagnostics and safe retries

`nines integration SLO_ID --context commerce --agent` is read-only. Rule deployment/evaluation stages include an optional `code` and `nextAction` in addition to `state` and `message`. The UI presents the next step alongside the affected stage. This text is operator guidance, not an executable instruction or permission to mutate a resource; reviewed digests and server roles still apply. A stage can remain unverified while another has current evidence; collected measurements do not imply loaded rules.

| Diagnostic code | Meaning and corrective action |
| --- | --- |
| `destination_not_configured` | Ask a tenant owner to configure a trusted destination before reviewing a rule plan. |
| `rule_file_missing` / `rule_group_missing` | No definition was found at the configured destination. Verify the mount or Mimir HTTP prefix/namespace, then review and deploy the current plan. A ruler 404 alone cannot distinguish a missing group from a gateway routing problem. |
| `rule_file_unreadable` | Check permissions, regular-file type, the 4 MiB limit and single-document YAML with exactly one group. Symlinks, additional YAML documents and unsupported fields are refused. |
| `rule_ownership_conflict` | Resolve ownership with an administrator. The API must not overwrite another tenant/SLO or an operator-owned group. |
| `stored_rules_drifted` | Stored content differs from the current definition. Review a new digest and destination before explicitly applying it. |
| `ruler_access_denied` / `evaluator_access_denied` | Check server-owned credentials, permissions and the configured Mimir organization. Provider error bodies are not returned. |
| `ruler_unavailable` / `evaluator_unavailable` | Check backend availability and gateway routing. Availability failures are not proof that rules are missing. |
| `evaluator_api_unavailable` | Verify the evaluator HTTP prefix and API support before treating runtime evidence as available. |
| `prometheus_lifecycle_unavailable` | The file is written but reload is unavailable. Enable lifecycle on the trusted endpoint or arrange manual reload. |
| `prometheus_reload_access_denied` / `prometheus_reload_failed` | The file is retained. Check reload permissions and full Prometheus configuration, then inspect runtime before retrying. |
| `reload_not_authorized` / `activation_plan_changed` | Configuration was saved but activation was blocked by changed authority or destination. Refresh and review the current plan. |
| `runtime_group_missing` | Prometheus: check the shared `rule_files` mount and arrange a reload. Prometheus Operator: check `ruleSelector` labels and `ruleNamespaceSelector` namespace labels. Mimir: check namespace and ruler polling delay. |
| `ruler_namespace_mismatch` / `ruler_namespace_unverified` | Matching group names in another namespace, or responses without namespace evidence, do not prove the intended Mimir group is evaluating. |
| `loaded_rules_drifted` | Compare loaded expressions, labels, interval and alert pending durations with the current plan; reload or reconcile explicitly. |
| `evaluator_response_incomplete` | Provider warnings, partial pagination, invalid/oversized responses or failed envelopes leave evaluation unverified. |
| `rule_evaluation_pending` / `rule_evaluation_stale` / `rule_evaluation_failed` | Wait for the configured evaluation interval or inspect backend scheduling, time synchronization and rule errors. These are distinct from insufficient traffic. |

Mimir inspection scopes the runtime request to the configured group and namespace and checks the namespace in the response. Prometheus container file paths can differ from Nines paths, so loaded content is compared to the reviewed group; this does not prove that two independently configured endpoints share one underlying dataset. Query/ruler routing remains an administrator responsibility. Namespace checks do not establish recorded-series presence or end-to-end measurement semantics.

A reviewed apply rechecks the configured destination immediately before writing and respects an optional caller-provided lease/authorization guard. Prometheus replacement uses an atomic rename and the same tenant/SLO/resource ownership checks as Mimir. The supported deployment still uses a single collecting server; a local mutex is not distributed locking or a compare-and-swap guarantee against another independent rule writer.

If a Mimir POST times out, loses its response or returns a server error, Nines performs one bounded GET from the exact reviewed organization, namespace and group when the request context still permits it. An exact match returns `state: verified`: destination content is verified, not evaluation or notification delivery. It never blindly repeats that POST. If readback is unavailable or does not match, the API returns an actionable unavailable error; the outcome remains unverified. A cancelled request may have committed remotely. Inspect before retrying, retain the reviewed digest, and obtain a fresh plan if the definition or destination changed. A new process can recognize an already matching group and return `unchanged` without another write. An individual manual recovery does not overwrite foreign content or silently repair drift. With automatic deployment enabled, later attempts take a fresh plan and inspect ownership/current destination again; they may repair drift in the owned group. Unsupported stored options, such as Mimir `source_tenants`, are rejected instead of being silently dropped from comparisons or overwritten.

These APIs follow the [Prometheus rules API](https://prometheus.io/docs/prometheus/latest/querying/api/#rules) and [Mimir ruler API](https://grafana.com/docs/mimir/latest/references/http-api/#ruler). Backend responses and capabilities must still be verified for the deployed version.

## Alertmanager boundary

Nines generates recording rules and severity-based burn-rate alert rules. The Prometheus server or Mimir ruler evaluates them and sends alerts through its configured Alertmanager integration. Receivers, routing trees, grouping, inhibition, silences and notification retry/delivery policies belong to Alertmanager and the surrounding deployment.

A healthy rule group, a pending alert or a firing `ALERTS` series does not prove that a receiver was notified. Test the evaluator-to-Alertmanager connection and each intended receiver separately, including critical/warning routing, silence/inhibition behavior and recovery notifications. Changing an SLO or deploying its rules does not create or modify Alertmanager configuration.

## Verify output through the query dataset

The Integration status panel and `nines integration SLO_ID --agent` include a separate `ruleOutput` stage. The compiler emits one `nines:rule_definition:info` recording rule per SLO group, with value `1`, definition digest, revision, tenant, source, SLO and optional Kubernetes resource UID. The server checks this marker through the collector's configured query URL, organization and **query credentials**. It never copies a ruler-write credential onto the query route.

This matters when the ruler API is healthy but its recordings are written to a different organization, cluster or remote-write destination from the query API. `evaluation: evaluating` and `ruleOutput: not_observed` can both be correct. The marker establishes that recent output for the reviewed definition is visible through the query source; it does not identify physical storage topology, establish useful traffic, validate the SLI's business meaning or prove notification delivery.

| Output code | Meaning and corrective action |
| --- | --- |
| `rule_output_verified` | A recent stored marker matches the current definition digest, revision and ownership scope. Check SLI traffic and coverage separately. |
| `rule_output_missing` | No marker was observed. Older generated groups need a newly reviewed plan and explicit deployment to add it. Otherwise allow evaluation/ingestion delay, then inspect query/ruler routing, organization, retention and remote-write. Absence cannot distinguish these causes. |
| `rule_output_out_of_date` | Only markers for other digests or revisions are visible. Compare loaded content and query routing before explicitly reconciling. |
| `rule_output_stale` | A matching marker exists but its stored sample is old or future-dated. Inspect scheduling, configured evaluation offsets, ingestion delay and clocks. |
| `rule_output_ambiguous` | Multiple current outputs match, potentially from duplicate rule owners or extra label dimensions. Resolve the duplicate route before relying on the evidence. |
| `rule_output_unavailable` / `rule_output_source_changed` | The query evidence is unavailable, incomplete, unsupported or changed during inspection. Retry against the current trusted source. |

The read-only query uses `timestamp(marker) and (marker == 1)`, not the instant response's evaluation timestamp. A backend can return an old sample with a new query timestamp. Current freshness requires a stored sample within the greater of three SLO steps or one minute, plus one minute for Mimir's default ruler evaluation delay, allowing at most five seconds of future clock skew. A longer custom evaluation offset or ingestion delay can exceed that bound and leave output stale even when runtime evaluations are healthy. Inspect that offset rather than weakening the SLO or assuming a routing failure. The response is bounded to eight series; warnings, truncation, malformed data or extra matches leave evidence unverified.

Adding the marker changes generated group content and the reviewed deployment digest. Existing rules follow the configured automatic or manual deployment policy; offline generation alone performs no deployment. Offline generation, server deployment and operator rendering share this marker. Budget one additional recording rule per group and review backend group limits.

## Repeatable acceptance checks

```sh
make test-prometheus
make test-mimir
make test-operator
```

The Prometheus fixture writes to a real shared mount, confirms file-only apply remains unloaded, then enables reviewed native reload and verifies the exact runtime group and current output marker. A repeated unchanged apply skips another reload. It uses a disposable pinned container and never accesses an existing Prometheus.

The Mimir fixture uses an isolated, digest-pinned container and synthetic input. It verifies separate organizations, healthy ruler evaluation with missing output in another query dataset, old-revision rejection, and critical/warning policies with independent pending durations and recovery on one unchanged reviewed SLO revision. Its second-scale windows use an explicit zero evaluation delay and fast ruler polling; these are test settings, not production defaults. The harness observes pending `activeAt`, checks firing against each configured duration, reduces burn so critical recovers while warning remains firing, then restores healthy traffic and checks warning recovery.

The operator fixture runs an agentless K3s API with verified TLS on a loopback-only port, the real upstream Prometheus Operator, the Nines controller under its shipped namespace Role, and disposable Mimir. It verifies both selector dimensions, CRD admission, RBAC, reconciliation, current-generation status, garbage collection, lease failover and guarded Mimir cleanup. It also checks the packaged nonroot container's service-account authentication, probes, sample reconciliation and shutdown. No host kubeconfig/context or existing cluster is used. It does not schedule workload pods or establish production scale. See [operator acceptance](operator.md#run-operator-acceptance-locally) for exact scope. Fixtures remove only resources they create.
