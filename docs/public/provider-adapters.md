# Observability provider adapters

Nines currently ships native Prometheus and Mimir connections, plus imported/manual counts. New Relic and Datadog are not selectable native providers in this release. The evaluation core is reusable, but adding either provider requires an adapter and supporting configuration; setting a new `kind` alone will not enable it.

The [declarative extension ecosystem](slo-extensions.md) distributes measurement recipes and presets on supported backends. It does not add native provider adapters or executable rule processors.

## Existing boundaries

The server's `Collector` interface returns interval good/total event counts. `WindowCollector` and `WindowPreviewer` support independently evaluated whole-window observations, including coverage. The deterministic engine consumes those normalized counts instead of a provider response format.

`TelemetrySource` supplies bounded metric discovery and dependency observations. The optional `CohortSource` returns grouped good/total counts at one cutoff. Quality reports and error-budget decisions operate on normalized stored evidence and do not execute PromQL.

These interfaces are separate capabilities. A provider can support collection without cohort inspection or external rule deployment. Unsupported capabilities must be reported explicitly. Whole-window estimates must never be summed as if they were disjoint intervals.

## What remains provider-specific

Connection configuration, the managed connection registry, source authentication, the structured measurement compiler and the rule manager currently implement Prometheus/Mimir semantics. The managed registry stores a concrete Prometheus client. Custom query validation and generated recording/burn rules use PromQL. These are extension points to update, not a complete runtime extension system.

Native provider support should include:

1. A tenant-scoped adapter that normalizes counts, timestamps, gaps, coverage and aggregation semantics. Do not substitute provider attainment percentages for event counts.
2. A measurement compiler for the provider's query model, preserving the reviewed population, success criteria, observation point and full-window semantics.
3. Connection factory and allowlist validation, encrypted or environment-referenced credentials, safe endpoint handling and a source fingerprint that changes with routing or credential identity.
4. Explicit discovery, cohort, historical-query and rule-management capabilities. Reading metrics and creating provider alert resources are separate operations.
5. Contract tests for missing numerator data, counter resets or provider aggregation equivalents, zero traffic, rate limits, partial responses, pagination/cardinality bounds, tenant/account isolation and concurrent configuration changes.

Adapters are compiled Go implementations today. A provider-specific credential scheme or endpoint must be added deliberately to server configuration and the portal. No New Relic or Datadog credentials or paid API calls were used to validate the current extension boundaries.

## Provider-neutral safeguards

A new adapter can reuse [measurement-quality checks](measurement-quality.md), [budget decisions](error-budget-policy.md), revision checks, CLI/API authentication, human-reviewed intent changes and audit records. Those features do not require a vendor-specific LLM component.

Rule generation remains Prometheus/Mimir-only until another rule adapter is implemented and verified. An adapter offering read-only metric collection must not report external rule deployment as supported or successful.
