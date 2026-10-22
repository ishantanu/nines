# Prometheus, Mimir and rule destinations

Nines queries your configured metrics backend and calculates SLO state. It does not replace application instrumentation, a Prometheus scraper, a remote-write pipeline or a full metrics store. The live Mimir integration fixture uses synthetic ingestion for testing; it is not a production ingestion gateway.

For a complete synthetic local flow, use the [self-contained Compose playground](playground.md). It includes both backends, explicit rule deployment, Prometheus reload handling and a burn-alert/recovery verifier.

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

Check the SLO detail Integration status panel, or run `nines integration SLO_ID --agent`. It reports definition saved, recent provider collection, destination content and live external evaluation separately. Inspection matches the loaded rule expressions, labels and interval to the current generated group. Recent successful evaluator health does not establish measurement coverage, recorded-series presence or notification delivery. Unsupported/unavailable runtime APIs remain unverified. Prometheus inspection uses its query endpoint; Mimir inspection uses its configured ruler endpoint with pinned organization and rule credentials.

The configured source, namespace and target determine where recording and alert rules are deployed. A successful apply proves provider acceptance, not that samples or alerts are being evaluated. Confirm the rule group in the intended Mimir organization/namespace, inspect evaluation errors and verify that the configured Alertmanager path receives alerts.

The MVP supports up to eight named short/long burn policies per SLO, with critical, warning or info severity. Each policy has independent thresholds, pending duration, coverage guards and alert state. The compiler deduplicates shared recording windows and labels alerts with `nines_policy` and `severity`. Use `spec.policies`; the existing single `spec.policy` remains accepted, but the two forms cannot be combined. More windows increase the generated rule count. Check Mimir rule-group limits before deployment; the synthetic playground allows 128 rules per group. Generation does not deploy alert receiver configuration or create a metrics ingestion tenant. Use [operational limits](../operations.md) when setting a retention period, long window or sparse-traffic objective.

## Prometheus rule files and Operator resources

A Prometheus connection can specify `rules.directory`. That directory must be writable by Nines and readable by the intended Prometheus server. Nines writes the files; the operator must include them in Prometheus `rule_files` and arrange a reviewed reload. A private file inside the Nines container is not automatically visible to Prometheus.

For Prometheus Operator output, review generated `PrometheusRule` resources and their namespace/labels against the target Prometheus selectors before applying them through your normal Kubernetes workflow. Exporting a resource does not prove the operator selected it. Do not let both Nines and the experimental reconciler own the same group.

## Verify the measurement

Check the original query directly in the intended backend, then check Nines sample freshness and coverage. Confirm the error numerator, traffic denominator, selectors and instrumentation semantics. SLO YAML does not prove those metrics exist. Missing data stays unknown, and a provider accepting a query is not evidence that the query represents successful user journeys.

Nines validates input PromQL and every generated recording/alert expression offline before export or deployment. Syntax/type validation does not verify metric availability, query cost, result cardinality or backend-version support. See [PromQL validation](../promql-validation.md).
