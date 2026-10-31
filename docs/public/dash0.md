# Dash0

Dash0 support is experimental. Nines can query a configured Dash0 dataset, calculate its own SLI and burn alerts, and publish a supported native Dash0 SLO through the existing reviewed deployment flow.

## Configure the connection

1. Create a Dash0 Reading token for telemetry queries. Native SLO publishing requires a separate token with Dash0's All permission; omit the `rules` section if you only need collection.
2. Supply both credentials privately through environment variables or a secret manager. Never put token values in an SLO, prompt, command argument or repository.
3. Copy [the tenant configuration example](../../examples/dash0/tenants.yaml) and select your official regional API origin and explicit dataset:

```yaml
connections:
  - name: dash0-production
    kind: dash0
    url: https://api.eu-west-1.aws.dash0.com
    options:
      dataset: production
    credentialsEnv:
      bearerToken: DASH0_READ_TOKEN
    rules:
      mode: native
      namespace: nines-production
      bearerTokenEnv: DASH0_PUBLISH_TOKEN
```

- This is an excerpt of a tenant configuration, not a standalone file. Start Nines with the complete file through `-tenants-file`.
- Read and publish tokens must belong to the intended Dash0 organization and dataset. Organization access follows the configured tokens; Nines cannot infer that two independently supplied credentials belong to the same organization. Credential rotation invalidates reviewed publication digests.
- Nines never infers a Dash0 dataset from a team name or incoming request header. The tenant-owned connection pins it explicitly.
- Supported API regions are AWS Ireland, Germany, Northern Virginia and Oregon, and GCP Netherlands. Arbitrary hosts, HTTP, embedded credentials and query/path overrides are refused.
- `namespace` is the publication ownership scope. Choose a unique scope for independently managed Nines deployments. Native origins also include the Nines tenant, connection, dataset and SLO identity.
- Native publication is manual. `autoDeploy`, Kubernetes operator publication, file paths, ruler URLs and reload options are rejected for this destination. Portal-managed Dash0 connections support telemetry reads. Configure native publishing in the server tenant file with a distinct publish credential.

## Define and collect an SLO

Use [the availability example](../../examples/dash0/api-availability.slo.yaml). Inline measurements and `providerWindow` use Nines' PromQL counter-window calculation; explicit reviewed interval-count queries are also supported for collection.

```sh
bin/nines validate --offline -f examples/dash0/api-availability.slo.yaml --agent
bin/nines plan -f examples/dash0/api-availability.slo.yaml --agent
bin/nines apply -f examples/dash0/api-availability.slo.yaml --expected-revision 0 --agent
bin/nines collect api-availability --recent --agent
bin/nines status api-availability --agent
```

- Configure the connected CLI's Nines URL, tenant and API key first. `--expected-revision 0` creates the example; updates require the current revision.
- Collection uses Dash0's GET Prometheus range-query API with a fixed dataset. Syntax/type validation is local; it does not prove instrumentation, counter type or population correctness.
- Unmatched good/total intervals, invalid counts, multiple aggregate series and provider warnings fail collection. Missing telemetry is not filled with successful events.
- Native SLO publication is separate from collection. Reading data does not create a native SLO, deploy recording rules or configure Dash0 alerting.

## Review and publish a native SLO

```sh
bin/nines rules plan --id api-availability --agent
# Review nativeSLO.document, nativeSLO.warnings, dataset, version and digest.
bin/nines rules apply --id api-availability --expected-digest REVIEWED_DIGEST --agent
bin/nines integration api-availability --agent
```

- The existing API paths and CLI `rules` group are retained for reviewed deployment. A Dash0 plan contains `nativeSLO` and OpenSLO YAML, with an empty recording-rule list. The portal displays a native SLO review panel.
- An owner must apply the exact plan digest. Nines pins the destination and includes the definition revision, routing, credential identity and upstream snapshot in the digest. Existing request audit, authorization and write guards still apply.
- New resources use POST with a unique origin. Updates send the version obtained during review. Concurrent creation, version changes, foreign ownership and changed plans are refused.
- Successful publication means the desired definition was read back. Runtime evaluation and notification delivery remain unverified. A lost acknowledgement or mismatched read-back produces an error, not a ready status.
- Refresh the plan after every mutation. `rules withdraw` disables the owned native SLO with a version check and verifies that setting; it retains the definition. It does not issue an unconditional DELETE or prove that previously emitted metrics have expired. Archival uses the same disable operation.

## Supported native subset and evaluation differences

- Native publication supports one occurrence-based objective, a 28-day rolling window and a target below 100%, with bare good/total counter selectors. Explicit interval expressions, referenced bindings, other windows and other query languages are refused for publication rather than rewritten.
- Dash0 processes native counter selectors over five-minute windows and applies a five-minute settling delay. Nines uses its configured evaluation windows and coverage rules. Counts, coverage, burn rates and attainment need not match across the two evaluators.
- Native Dash0 check-rule publishing is not implemented in this slice. Nines burn policies remain in Nines. Dash0 accepts but discards OpenSLO `alertPolicies`, so Nines does not send that field or claim native alerts were configured. Configure and verify Dash0 check rules separately.
- [OpenSLO export](openslo.md) is an interoperability boundary. Nines YAML remains authoritative for critical user journeys, approvals, measurement quality, lifecycle and error-budget governance.

## Verification and limits

- Synthetic HTTP tests exercise dataset routing, GET queries, counts, ownership, version-fenced creation/update/disable, plan digests, credential rotation, write guards, failed read-back, redaction and unsupported configurations.
- No Dash0 account, credentials or production telemetry were used. Live account acceptance, native evaluation semantics and alert delivery remain unverified. There is no local Dash0 container acceptance equivalent to the VictoriaMetrics test.
- References: [Dash0 SLO API](https://www.dash0.com/docs/dash0/monitoring/alerting/slos-api), [Prometheus range query](https://www.dash0.com/docs/api-reference/operations/get/api/prometheus/api/v1/query_range), [Dash0 API schemas](https://www.dash0.com/docs/api-reference).

## Portal-managed read connections

Tenant owners can select **Dash0** in **Telemetry connections** once the server allows the regional hostname and dataset. Supply a read bearer token; it is encrypted and never returned. The dataset must appear in the tenant's `managedProviderRoutes.dash0` policy or an existing server-configured Dash0 connection. Manual collection is the default in the creation form. Native SLO publishing still requires the separate server-owned publisher configuration above and a distinct publish credential. See [managed connection setup](provider-adapters.md#manage-connections-in-the-portal).
