# Migrate from Sloth or Pyrra

Nines converts [Sloth](https://sloth.dev) and [Pyrra](https://github.com/pyrra-dev/pyrra) SLO definitions into Nines SLO definitions. Conversion is offline and deterministic, saves nothing, and reports every approximation. What cannot be converted faithfully is skipped with the reason, never guessed.

## Plan the migration

1. **Convert** your definitions and read the notes and skipped items.
2. **Plan and apply** the converted SLOs. They start **experimental**: Nines measures them and generates recording rules but does not page.
3. **Run beside your current rules.** Compare the Nines SLI, budget and burn with Sloth or Pyrra for a full SLO window, or at least long enough to cover an incident.
4. **Promote to committed** once the numbers agree, then retire the old alerts. Promoting a committed SLO enables Nines burn alerts, so doing this while the old rules are still running pages you twice.

## Convert in the portal

On **Service level objectives**, choose **Import from Sloth or Pyrra**. Paste or upload YAML with one or more definitions, choose the tool, project, metrics connection, lifecycle and, for Sloth, the SLO window, then **Convert**.

- Each converted SLO shows how many notes are **not carried over** and how many are **to review**. Open it to read every note and the exact YAML.
- Skipped definitions are listed with the reason.
- **Plan** checks each selected SLO against the saved state without saving it. Then acknowledge the review and **Apply**. Each apply uses the revision from its own plan, so an SLO that changed since is refused rather than overwritten.
- Viewers and agents can convert and plan but not apply.

## Convert on the command line

```sh
nines import sloth -f slo.yaml --out-dir slos/ --project payments --source mimir --window 30d
nines import pyrra -f pyrra-slos.yaml --out-dir slos/
nines import sloth -f slo.yaml --agent        # JSON with definitions, notes and skipped items

nines plan -f slos/payments-requests-availability.slo.yaml --agent
nines apply -f slos/payments-requests-availability.slo.yaml --expected-revision 0
```

`import` runs without a server and never overwrites a file unless you pass `--force`. Without `--out-dir` it prints the YAML. The same conversion is available as `POST /api/migrations/convert` and in the Go client as `ConvertMigration`.

## What converts

|        | Sloth                                                                    | Pyrra                                                                            |
| ------ | ------------------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| Input  | `prometheus/v1` spec and the `PrometheusServiceLevel` resource           | `pyrra.dev` `ServiceLevelObjective`                                              |
| SLI    | `sli.events` with `error_query` and `total_query`                        | `indicator.ratio` (errors and total) and `indicator.latency` (success and total) |
| Target | `objective` percentage                                                   | `target` percentage                                                              |
| Window | The generator setting, 30 days by default. Set it with the window option | `window`, such as `4w` or `30d`, up to 31 days                                   |
| Labels | Spec and SLO labels                                                      | Resource labels                                                                  |
| Alerts | Page and ticket alerts, each switchable                                  | `alerting.disabled`                                                              |

### How queries convert

Nines evaluates good and total event counts for each minute. A Sloth rate over `{{.window}}` becomes an increase over one minute, and `rate()` becomes `increase()`. Nines needs good events, so `goodQuery` is total minus errors, computed from your two queries.

- A query is converted only when it is made of sums of counter rates or increases, with optional `by` or `without` and the common `or vector(0)`. Anything else is skipped and names the function it could not convert, for example `histogram_quantile()`.
- Pyrra metrics are counter selectors with optional label matchers. Regular-expression matchers are kept as written.
- Verify the converted counts against your dashboards before relying on them.

### Alert policies

Both tools generate multi-window burn-rate alerts. Nines creates four standard policies from the budget fraction consumed over each look-back: 2% in 1 hour, 5% in 6 hours, 10% in 1 day and 10% in 3 days. At a 30-day window these are the familiar burn rates 14.4, 6, 3 and 1. They scale with the window, and a policy whose look-back is longer than the SLO window is left out and reported.

Compare these thresholds with your current alerts. Pyrra scales its own factors differently, so they will not match exactly.

## Choose a connection that speaks PromQL

Converted definitions use PromQL. They work with a Prometheus, Mimir, Dash0 or VictoriaMetrics connection. Datadog, New Relic and Google Cloud connections run their own query languages and will refuse these definitions when you plan them, so rewrite the queries there instead. The import dialog warns when the connection you name cannot run PromQL.

## What does not convert

- **Sloth raw error-ratio queries** (`sli.raw`) and **plugins** have no event counts or are generated at runtime. Rewrite the SLI as error and total counters, or define it in Nines directly.
- **Pyrra `latencyNative` and `boolGauge`** indicators use native histograms and 0/1 gauges, which Nines cannot count per interval.
- **Pyrra grouping** creates one SLO per group value. Nines imports one aggregate SLO. Add `quality.cohortLabels` after import to inspect the groups.
- **Alert names, labels, annotations and routing** are not carried over. Nines generates its own alert rules from the policies, and routing stays in your Alertmanager configuration.
- **Descriptions** are not carried over, because a Nines SLO has no description field.
- **Labels** are limited to eight with Nines naming rules. A label that cannot be kept is reported, and an invalid name is renamed when it can be.
- **Windows** longer than 31 days are skipped.
- **Generated rules** differ from Sloth's and Pyrra's. Deploy them with the usual [rule delivery](deployment/automatic-rules.md) and keep them separate from the old ones.

## Safe defaults and limits

- Imported SLOs are experimental unless you choose otherwise. Choosing **committed** in the portal shows a warning because it pages straight away.
- Each name is `<service>-<slo>` for Sloth and the resource name for Pyrra, lowercased and made unique within the import.
- A request converts at most 50 YAML documents or 200 SLOs from at most 1 MiB of YAML.
- Converted YAML is validated with the same rules as any definition before it is offered, so a result that cannot be applied is skipped instead.
- Nothing is sent to a provider or a model.
