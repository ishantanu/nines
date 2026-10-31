# New Relic

Nines collects reviewed NRQL event counts from one configured New Relic account. It can also publish a native event-count service level through a separate reviewed plan. A saved connection, collected events, a stored native SLO and observed native metrics are separate evidence.

## Empty recent reads

- **Read latest measurements** queries a recent interval; it does not replay the historical trial batch.
- New Relic's exact “No events found” notice returns no collected samples. Nines does not count its zero-filled buckets as measurement coverage or healthy attainment.
- Use the trial's saved timestamps for historical collection, or send fresh telemetry deliberately. Do not re-ingest the trial simply to clear a status badge.
- Other provider warnings, partial responses and mismatched good/total evidence still fail collection without committing samples.

## Find your data

- The trial fixture sends **custom events**, with event type `NinesTrialRequest`. They are not dimensional `Metric` rows and do not appear as named metrics in the Metric explorer.
- Open New Relic's query builder, select the configured account and run:

  ```sql
  FROM NinesTrialRequest
  SELECT count(*) AS total,
    filter(count(*), WHERE success = true) AS good
  WHERE service = 'nines-trial'
  SINCE 1 day ago
  ```

- For a historical acceptance run, use its exact `run` attribute and timestamps from the local fixture state. A recent dashboard time range can exclude the entire six-minute trial. The fixture sends one bounded batch; it does not stream traffic continuously.
- In Nines, select the tenant containing `newrelic-trial`, then open `newrelic-trial-availability`. Its connection defaults to manual collection to protect trial usage. Collect a bounded range explicitly; do not enable continuous collection merely to inspect an old test.
- New Relic's generated queries normally include `UNTIL 2 minutes AGO`. They deliberately exclude the newest events while metrics settle. Allow several minutes after fresh traffic before expecting a native SLI result. See [consuming service levels](https://docs.newrelic.com/docs/service-level-management/consume-slm/).
- Empty native metrics do not mean that custom events failed ingestion. Native service levels have their own derived metric queries. Inspect these after publication and verify fresh data; previous event storage does not establish native metric history.

## Configure collection

- Use the official NerdGraph endpoint for your region and a user API key. The EU endpoint is `https://api.eu.newrelic.com/graphql`.
- Pin the upstream account in server-owned configuration. The key is an environment reference, never part of an SLO:

  ```yaml
  name: newrelic-production
  kind: newrelic
  url: https://api.eu.newrelic.com/graphql
  options:
    accountID: "123456"
  credentialsEnv:
    userKey: NEWRELIC_API_KEY
  collectionMode: manual
  ```

- The separate ingest/license key is only needed to send test data. Nines collection and publication use the user key. Do not give the running Nines server an ingest key unless a separate ingestion workflow needs it.
- Use `queryLanguage: nrql`, `SELECT count(*) FROM EventType` queries and optional AND-connected predicates. Time bounds and bucket aliases belong to the collector. Percentiles, gauges and SLO attainment percentages are not event counts.
- See the [SLO example](../../examples/newrelic/api-availability.slo.yaml) and [provider count semantics](provider-adapters.md#configure-new-relic). Native publication does not expand the collector's query subset.

## Configure native publication

- Associate the service level with an existing service entity or workload in the same New Relic account. Copy its entity GUID from New Relic metadata. Nines verifies account ownership and never chooses or creates an entity automatically.
- The user key's identity needs full-platform access and permission to manage events-to-metrics rules. Read access alone does not establish publication permission. New Relic documents this in [service-level requirements](https://docs.newrelic.com/docs/service-level-management/create-slm/).
- Add an explicit native destination to the trusted connection:

  ```yaml
  rules:
    mode: native
    namespace: production
    entityGUID: REPLACE_WITH_APPROVED_NEW_RELIC_ENTITY_GUID
  ```

- For managed portal connections, the administrator must approve both the account and native entity:

  ```yaml
  managedProviderRoutes:
    newrelic: ["123456"]
    newrelic-entity: ["REPLACE_WITH_APPROVED_NEW_RELIC_ENTITY_GUID"]
  ```

- A server-configured New Relic connection's account and `rules.entityGUID` also populate these approved choices. In **Telemetry connections**, opt into **Enable reviewed native SLO publication** and select an approved entity. Credentials stay encrypted. Publication verifies that the entity belongs to the selected account.
- Recording-rule directories, ruler URLs, automatic publication and Kubernetes operator publication are not supported for this native destination. Saving an SLO never publishes it automatically.

## Review and publish

1. Use an exact rolling window of **1, 7, 14, 28 or 30 days**. Unsupported windows are refused. The percentage target must fit New Relic's five-decimal precision; Nines does not silently round it.
2. Open the SLO deployment panel and inspect the native document, account, entity, ownership and warnings. The CLI uses the same reviewed operation:

   ```sh
   nines rules plan --context production --id api-availability --agent
   nines rules apply --context production --id api-availability --expected-digest REVIEWED_DIGEST --agent
   ```

3. Publication translates the reviewed total query into `validEvents` and the good query into `goodEvents`, retaining the event type and predicates. The default count selector implements `count(*)`; facets, bad-event definitions, other selectors and time-based criteria are outside this subset.
4. After creation, Nines reads the native definition back and checks its content and association. The response contains the indicator ID, native link and New Relic's generated good-event, valid-event and indicator queries.
5. Use **Inspect native SLI metrics** in the portal to open those exact queries. Run them in the configured account. Select **Last 24 hours** for the one-shot playground trial so its historical traffic is included. Verify the counts and time range; no provider-returned query is executed automatically by this panel.
6. A refreshed plan recovers the ID and link from the provider after a Nines restart. `nines integration SLO_ID --agent` reports native storage independently from collection and native evaluation.

## Revisions and limits

- Native revisions are immutable. Editing a Nines SLO creates another owned native indicator after explicit review; older revisions remain retained. Conflicting or duplicate ownership matches block publication. New Relic's account SLI quota therefore matters; inspect and clean up retained revisions deliberately.
- Nines does not issue unconditional native updates or deletion. **Verify provider removal** checks that an administrator removed all indicators in the ownership scope.
- A failed create can have an uncertain outcome. Nines never retries it automatically. Inspect New Relic before refreshing or retrying, and use one publisher per ownership scope. Independent publishers do not provide an exactly-once contract.
- Native alert conditions and notification destinations are not published. Nines burn policies and notification delivery remain independent. A stored native SLO is not proof of native evaluation, complete coverage or working notifications.
- New Relic source: [NerdGraph service-level configuration](https://docs.newrelic.com/docs/apis/nerdgraph/examples/nerdgraph-slm/).

## Playground opt-in

- The [bounded provider trial](provider-adapters.md#run-a-bounded-live-datadog-and-new-relic-test) sends synthetic events once and preserves its run state.
- When generating the optional playground tenant file, set `NINES_NR_NATIVE_PUBLISH=1` and `NINES_NR_ENTITY_GUID` to the reviewed entity GUID. This enables reviewed publication only; generation does not create service levels or ingest more data.
- The fixture does not create production entities. For a disposable account with no services, provision a dedicated synthetic workload explicitly, then configure its GUID.

## Read native attainment

The SLO publication panel now has an on-demand **Native provider results** read with explicit UTC dates, interval history and range attainment. Budget is calculated from the native generated good/valid metrics and labelled accordingly. These results stay separate from Nines event-count collection and do not prove alert delivery. Empty native metrics remain unavailable; no raw-event history is substituted.

Use `nines native results SLO_ID --from RFC3339 --to RFC3339` for the same evidence without the portal. `nines native list CONNECTION` and `nines native draft CONNECTION --native-id ID --name NAME --project PROJECT --service SERVICE` prepare supported pre-existing COUNT definitions for reviewed import. Discovery is restricted to the server-configured associated entity/account. See [limits and examples](provider-adapters.md#native-results-and-reviewed-import).
