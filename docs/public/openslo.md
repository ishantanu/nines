# OpenSLO interoperability

Nines uses OpenSLO v1 for a bounded export format and native Dash0 SLO publication. Nines resources continue to use `nines.dev/v1alpha1` YAML.

## Why keep both formats?

- OpenSLO describes portable reliability objectives and measurement sources.
- Nines adds critical user journeys, revisioned measurements, quality checks, agent proposals, approvals, audit, lifecycle and error-budget policy. The initial exporter cannot represent those concerns losslessly.
- Providers implement different OpenSLO subsets and evaluation semantics. A valid OpenSLO document is not proof that a provider can run the same measurement or alert policy.

## Export without a server

```sh
# Review the document and every omission as structured output.
bin/nines openslo export --offline \
  -f examples/dash0/api-availability.slo.yaml --agent

# After review, explicitly acknowledge omissions for a bare YAML document.
bin/nines openslo export --offline \
  -f examples/dash0/api-availability.slo.yaml \
  --acknowledge-omissions > api-availability.openslo.yaml
```

- Requires an inline measurement or `providerWindow` containing bare PromQL counter selectors. The exporter does not attempt to reverse-engineer arbitrary query expressions into selectors.
- Produces one occurrence-based ratio objective and a rolling window expressible in whole minutes. It uses OpenSLO duration units rather than Nines' numeric seconds.
- Structured output contains `document` and `warnings`. Bare export requires explicit acknowledgement and prints warnings to stderr. Burn policies are omitted rather than silently mapped to provider alerting.
- Source, project, scrape interval, collection step and governance remain in Nines. The receiving system needs its own trusted connection and evaluation configuration.
- The public Go package is `github.com/ishantanu/nines/openslo`; `Export` accepts the shared `api/v1alpha1.ServiceLevelObjective` type and returns a typed document plus warnings.
- This is export only. General OpenSLO import, arbitrary OpenSLO constructs and lossless round trips are not implemented.

## Dash0 profile

- [Dash0 publishing](dash0.md) applies stricter checks: a rolling 28-day window, target below 100% and inline good/total counter selectors.
- Stable ownership metadata and upstream version checks are added at the publishing boundary, not to ordinary Nines definitions.
- Dash0 returns `openslo.com/v1`; Nines accepts that response spelling while ordinary exports use the upstream `openslo/v1` spelling.
- Native SLO publishing does not publish Dash0 check rules. Existing Nines burn policies continue to use the Nines evaluator.

References: [OpenSLO v1 SLO schema](https://openslo.com/schema/v1/slo/), [Dash0 SLO subset](https://www.dash0.com/docs/dash0/monitoring/alerting/slos-api).
