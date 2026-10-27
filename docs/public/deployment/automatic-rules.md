# Automatic rule deployment

An owner can enable automatic rule deployment for a telemetry connection.

- Creating or editing an SLO through the portal, CLI or an approved intent workflow then queues its recording and burn-rate alert rules automatically.
- The same policy applies to a reviewed configuration rollback.
- The SLO save is a database operation; provider calls happen afterward in the reconciler.
- A failed deployment does not undo or conceal the saved definition.
- The Compose playground enables this policy for Prometheus and its optional Mimir workspace.
  - Other sources remain manual until explicitly enabled.
  - Owners should review this delegation: it covers existing and future saved SLOs using the source.
  - The agent role still cannot save arbitrary definitions, confirm intent measurements or approve/apply intent change sets.
  - Automatic delivery does not approve an agent recommendation.

## Prometheus

```yaml
# Inside a tenant in TENANTS_FILE
source:
  kind: prometheus
  url: http://prometheus:9090
  rules:
    namespace: nines
    directory: /rules
    autoDeploy: true
    reload: true
```

- Nines writes one owned rule group atomically and requests `POST /-/reload` on this trusted Prometheus endpoint. The same directory must be writable by Nines, mounted read-only into Prometheus, and included in Prometheus configuration:

```yaml
rule_files:
  - /etc/prometheus/nines/*.yaml
```

- For example, mount the same Compose volume as `/rules` in Nines and `/etc/prometheus/nines` in Prometheus.
  - When multiple Prometheus sources share a directory, give each a distinct rule namespace and configure each instance's `rule_files` glob to match only its own namespace prefix. Enable `--web.enable-lifecycle` on Prometheus.
  - An HTTP prefix in the configured URL is preserved.
  - The server rejects `autoDeploy: true` for direct Prometheus deployment unless `reload: true` is set.
  - A local file alone cannot make a remote Prometheus load rules, and remote write sends samples rather than rule definitions.
  - When a shared filesystem is unsuitable, use the [Kubernetes operator](operator.md).

## Mimir

```yaml
source:
  kind: mimir
  url: https://mimir.example.com/prometheus
  mimirOrgID: commerce
  bearerTokenEnv: MIMIR_QUERY_TOKEN
  rules:
    namespace: nines
    rulerURL: https://mimir-ruler.example.com/prometheus
    bearerTokenEnv: MIMIR_RULES_TOKEN
    autoDeploy: true
```

- The reconciler uploads the owned group to the configured ruler namespace with the pinned organization and credentials.
  - Mimir activates it through ruler polling; no reload call or shared directory is needed.
  - Runtime evaluation and recorded output in the query dataset remain separate checks.
  - Output freshness uses the stored sample timestamp and allows Mimir's standard one-minute evaluation delay in addition to the usual evaluation/ingestion allowance.
  - Deployments with a longer custom ruler delay can still report stale output even when rules evaluate successfully. See the [Mimir delay configuration](https://grafana.com/docs/mimir/latest/configure/configuration-parameters/).
- For portal publishing through the operator, set `rules.mode: operator`, the trusted namespace/publisher and `autoDeploy: true`.
  - Saved definitions are published as owned Kubernetes SLOs; the controller reconciles provider rules.
  - Automation policy changes do not transfer a pinned publication route.
  - Standalone GitOps CRs already reconcile automatically and need no server setting.

## Configure in the portal

- Under **Telemetry connections**, an owner can enable rule deployment and **Deploy rules automatically after SLO saves** for a managed connection.
  - Prometheus reload is required and enabled while automatic deployment is selected.
  - The managed rule directory and allowed provider hosts/organizations remain server configuration.
  - Static YAML sources are edited through their server configuration.
- Open an SLO's **Integration status** to see automatic deployment as pending, applying, applied, failed, overdue or paused.
  - Rule configuration, backend activation, runtime evaluation, query-output visibility and SLO attainment remain independent evidence.
  - A written file or accepted ruler POST does not mean the rules are evaluating.
  - Reload failures remain visible and retain the written file for recovery.
- The worker scans persisted definitions at startup and every five seconds.
  - Failed attempts use exponential retry delays from five seconds up to five minutes.
  - Successful revisions are rechecked about once a minute to repair drift in owned content.
  - It refuses to replace foreign-owned files/groups, follows the existing operator ownership/route fences and checks current revision/destination before writes.
  - Reconciliation is serialized with definition changes and uses bounded provider calls.
  - Run one collecting server process, as required by the MVP; this is not a distributed scheduling or exactly-once guarantee.

## Pause and resume

- A tenant owner can select **Pause automatic deployment** on an SLO's Integration status card.
  - This stops future reconciliation; it does not remove loaded rules.
  - The pause persists in PostgreSQL across restarts and SLO edits.
  - Select **Resume automatic deployment** to reconcile the latest saved revision.
  - In memory mode it lasts only for the process lifetime.

```sh
nines rules pause --id checkout-success --expected-revision 2 --context production --agent
nines rules resume --id checkout-success --expected-revision 2 --context production --agent
nines integration checkout-success --context production --agent
```

- The owner-only API is `POST /api/slos/{id}/rules/automation`, with `expectedRevision` and an explicit boolean `paused`. The Go client exposes `SetRuleAutomation`. The MCP adapter exposes automation evidence through its existing read-only integration tool; no automation mutation tool is added.
- Manual reviewed rule apply remains available as an owner retry/override and resumes a paused SLO after successful persistence.
  - An explicit operator publication withdrawal pauses its automation before cleanup, preventing recreation.
  - Direct-provider withdrawal is not implemented by that operator-specific command: pause first and use your provider's owned cleanup procedure.
  - Disabling source automation stops future attempts for that source; it does not delete running rules.
  - Other rule writers must not manage the same owned group.
- See [playground testing](playground.md), [rule destinations](metrics-and-rules.md) and [intent governance](../intent-governance.md).
