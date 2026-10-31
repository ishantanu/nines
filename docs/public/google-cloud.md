# Google Cloud Monitoring

Experimental Google Cloud support reads project-scoped event counts, evaluates Nines SLOs and burn policies, and publishes reviewed native Cloud Monitoring SLO revisions. Stored definitions, native evaluation evidence and Nines SLI are separate states.

## Configure authentication and project routing

1. Enable the Cloud Monitoring API in your test project and select an existing Cloud Monitoring service if you need native SLO publishing. Nines does not create projects, enable APIs or create services.
2. Prefer Application Default Credentials (ADC) using an attached workload identity or Workload Identity Federation. For local development, configure ADC outside Nines. Select it explicitly with `options.auth: adc`.
3. Configure [the ADC tenant example](../../examples/googlecloud/tenants.yaml):

```yaml
connections:
  - name: google-checkout
    kind: googlecloud
    url: https://monitoring.googleapis.com
    options:
      projectID: nines-demo-project
      serviceID: checkout
      auth: adc
    rules:
      mode: native
      namespace: nines-production
```

- This is a tenant-file excerpt. Start the server with the complete configuration using `-tenants-file`.
- Project routing comes from server-owned configuration, not the SLO's business `metadata.project`, caller headers or model output. Every metric filter also includes that project to constrain a broader metrics scope.
- `serviceID` maps the connection to an existing native service. Use separate connections for different native service routes; Nines does not infer this identifier from `spec.service`.
- Project IDs or canonical project numbers are accepted. Canonical project-number responses are normalized to the configured project alias while preserving service/resource identity; mixed canonical projects and unrelated project names are refused.
- The official Monitoring HTTPS origin is fixed. Arbitrary hosts, URL prefixes, embedded credentials and project/service path overrides are refused.
- `namespace` is a native publication ownership scope, not a Google or Kubernetes namespace. Choose a unique scope for independently managed Nines installations sharing a Google project/service.
- The optional native destination is manual. `autoDeploy`, Kubernetes operator mode, ruler URLs, directories and reload options are refused. Google Cloud connection creation is server-managed; configure its project, service and authentication in the tenant file. The portal can select existing Google Cloud connections for SLOs. Other managed providers are listed in the [provider support matrix](provider-adapters.md#capabilities).

### Short-lived access tokens

Use [the access-token configuration](../../examples/googlecloud/tenants-access-token.yaml) when ADC is unsuitable:

```yaml
options:
  projectID: nines-demo-project
  serviceID: checkout
  auth: access-token
credentialsEnv:
  accessToken: GCM_READ_ACCESS_TOKEN
rules:
  mode: native
  namespace: nines-production
  bearerTokenEnv: GCM_PUBLISH_ACCESS_TOKEN
```

- Inject token values privately through your environment or secret manager. Do not put them in YAML, command arguments, prompts or Git.
- An explicit access token is not refreshed automatically. Refresh it and restart/reload the server configuration as needed. ADC delegates refresh to Google's OAuth library.
- ADC and explicit tokens cannot be combined for collection. ADC publication uses the process's configured ADC identity unless a separate publish-token environment reference is provided.
- Tokens, credential files and ADC configuration are never selected by an SLO or agent. Native publication digests include credential configuration identity; rotating an access token invalidates an old plan. Routine ADC access-token refresh does not.

### IAM permissions

- Collection calls `monitoring.timeSeries.list`. Connection probes additionally call `monitoring.metricDescriptors.list`; a successful probe establishes API access, not usable telemetry.
- Native publication needs `monitoring.slos.get`, `monitoring.slos.create` and `monitoring.slos.list`; native health inspection also reads time series. The implementation never calls native SLO PATCH or DELETE.
- Prefer a custom IAM role with the necessary permissions. Monitoring Viewer and Monitoring Services Editor are broader predefined alternatives; grant only what the deployment needs. OAuth scopes do not grant IAM permissions.
- An ADC file's valid `quota_project_id` is sent as `X-Goog-User-Project` for quota/billing. It does not change the server-pinned metric or native SLO project.
- ADC requests the Monitoring OAuth scope. Short-lived tokens must also carry a compatible Monitoring or cloud-platform scope.

## Define the measurement

Use [the complete availability example](../../examples/googlecloud/api-availability.slo.yaml):

```yaml
spec:
  source: google-checkout
  queryLanguage: googlecloud
  target: 0.999
  windowSeconds: 2419200
  stepSeconds: 60
  goodQuery: 'metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global" AND metric.labels.result = "good"'
  totalQuery: 'metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global"'
```

- The initial filter grammar requires exact `metric.type` and `resource.type` predicates with optional exact metric/resource label predicates, joined by `AND`. The good filter must refine the total population. Project selectors, OR, regex/functions, duplicate predicates and query-language expressions are refused.
- Queries are Monitoring filters, not PromQL or MQL. Offline validation checks this supported grammar; it cannot establish metric existence, type, instrumentation or complete eligibility.
- Source metrics must be scalar `DELTA` `INT64` or `DOUBLE` event counts. Each raw point must span exactly one configured interval, align to UTC step boundaries and lie inside the requested range. Steps are 60–3600 seconds.
- No alignment, rate conversion, cumulative differencing or interpolation is requested. Cumulative counters, gauges, distributions and percentages are refused. Explicit zero counts stay zero; missing data stays missing.
- Returned labels, resource types, timestamp pairs and good/total populations are checked. Each selected series must cover every returned slot; disappearing replicas fail conservatively instead of contributing an assumed zero.
- Limits: 31 days, 100,000 requested intervals, 100,000 source points per query, 512 series, 32 pages and 8 MiB per response. Individual and aggregate counts above 2^53 are refused to keep integer counts within the exact float64 range. Partial failures, pagination cycles and duplicate observations fail collection.
- Metric discovery, inline Prometheus measurements, cohort/dependency inspection and general OpenSLO export of Google filters are not implemented in this profile.

## Collect and inspect Nines SLI

Configure the CLI's Nines URL, tenant and API credential first:

```sh
bin/nines validate --offline -f examples/googlecloud/api-availability.slo.yaml --agent
bin/nines plan -f examples/googlecloud/api-availability.slo.yaml --agent
bin/nines apply -f examples/googlecloud/api-availability.slo.yaml --expected-revision 0 --agent
bin/nines collect checkout-availability --recent --agent
bin/nines status checkout-availability --agent
```

- Revision zero creates the resource. Updates require its current Nines revision.
- Nines calculates attainment, coverage, budgets and critical/warning burn policies from the collected counts. Its configured notification delivery remains available.
- Verify that your actual telemetry emits these interval counts before adopting this example. Creating the SLO does not ingest application telemetry into Google Cloud.

## Publish a native SLO revision

```sh
bin/nines rules plan --id checkout-availability --agent
# Review nativeGoogleCloud.document, warnings, project, service, resource and digest.
bin/nines rules apply --id checkout-availability --expected-digest REVIEWED_DIGEST --agent
bin/nines integration checkout-availability --agent
```

- The existing reviewed deployment API/CLI is reused. Plans contain `nativeGoogleCloud`, native Monitoring YAML and an empty recording-rule list. The portal labels the resource as a Google Cloud native SLO.
- Supported native definitions use a good/total request ratio, whole-day rolling windows up to 30 days and a goal at most 99.99%. Filters are project-pinned. Distribution-cut and built-in native SLIs are not mapped in this initial version.
- The native SLO API has no documented version/ETag precondition. Nines therefore creates an immutable, ownership-labelled resource tied to the Nines definition and revision. It does not PATCH an existing resource. A conflict at the same resource identity is refused.
- Review and apply still require the current digest, owner authorization, destination pinning and write guards. Native creation uses an explicit resource ID, then read-back verifies the desired content. Uncertain acknowledgements require inspection and a fresh plan.
- Publishing a new Nines revision retains earlier native revisions. Account for native SLO quotas and storage/evaluation costs; this is not automatic garbage collection. Manage retained resources in Google Cloud.
- `rules withdraw` verifies administrator-performed removal of **all** native revisions in the ownership scope before clearing the pinned route. It does not delete them. Archival cleanup stays pending until that absence can be verified, including resources whose ownership labels were changed. Previously emitted metrics may remain retained.

## Native evaluation and alerts

- Integration inspection can read `select_slo_health` for the exact native SLO resource. A single valid ratio signal no older than five minutes supplies native evaluation evidence. Missing, stale, partial or invalid responses remain unverified.
- This native ratio is not converted to event counts or inserted into the Nines SLI engine. Provider aggregation, settling behavior and coverage can differ from Nines' discrete interval calculation.
- Native Cloud Monitoring alert-policy publishing is not implemented. Nines burn alerts still work; configure and test Google-native burn-rate policies and notification channels separately if required. Native SLO publication alone never reports native alerts or notification delivery as verified.
- Native Monitoring documents use Google's API schema. [OpenSLO](openslo.md) remains a separate bounded interchange format; Nines does not pretend Google consumes OpenSLO directly.

## Verification

- Synthetic HTTP tests cover fixed routing/auth, exact counts, label checks, incomplete/invalid evidence, pagination failure, immutable publication, read-back, drift, reviewed digests, write guards, cleanup and native health freshness. SLO-service tests exercise burn firing and recovery.
- No real Google Cloud credentials, projects or telemetry were used. Live IAM/ADC refresh, canonical resource naming, actual metric intervals, native evaluation and notification delivery remain unverified. Do not treat fixture tests as production acceptance.
- References: [Monitoring filters](https://docs.cloud.google.com/monitoring/api/v3/filters), [native SLO API](https://docs.cloud.google.com/monitoring/api/ref_v3/rest/v3/services.serviceLevelObjectives), [SLO signals](https://docs.cloud.google.com/stackdriver/docs/solutions/slo-monitoring/api/timeseries-selectors), [ADC](https://docs.cloud.google.com/docs/authentication/application-default-credentials), [Monitoring IAM](https://docs.cloud.google.com/iam/docs/roles-permissions/monitoring).
