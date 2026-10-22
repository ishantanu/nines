# PromQL validation

Nines uses the Prometheus parser and type checker pinned in `go.mod`. Input validation is shared by the CLI, API, operator and tools that call the service. It runs offline without contacting Prometheus or Mimir.

```sh
nines validate --offline -f examples/api-availability.slo.yaml --agent
nines rules generate --offline -f examples/api-availability.slo.yaml --tenant commerce
```

Both `goodQuery` and `totalQuery` must be valid PromQL expressions returning instant vectors. For example, `sum(increase(http_requests_total[1m]))` and `vector(100)` are valid types. An incomplete `sum(`, unknown function, scalar `100`, string literal or range vector `http_requests_total[1m]` is rejected with the affected field in the diagnostic. The vector requirement matches the collector and rule compiler's interval-count contract. A valid type alone does not make an expression an appropriate event count.

Provider-window selectors retain their restriction to metric names with exact label matchers and also pass through PromQL parsing. This catches invalid escapes that a selector-shape check alone would miss. Manual-ingestion SLOs may still omit both queries.

Every generated recording and burn-rate alert expression is parsed and checked before the rule group is returned for export or deployment, including provider-window and operator-owned groups. Interval queries are printed from their parsed syntax trees before nesting, so trailing comments cannot swallow generated syntax. Canonical formatting may change a rule plan's digest; fetch and review a fresh plan before applying it.

Offline validation does not establish that metrics exist, selectors match traffic, each input returns one aggregated series, values represent interval counts, good counts do not exceed total counts, or a query is affordable. It also cannot prove compatibility with every backend version or extensions such as MetricsQL and LogQL. Collection and integration diagnostics provide separate runtime evidence. There is no additional `promtool` installation requirement for these checks.

Previously accepted malformed or incorrectly typed queries now fail validation. Correct them before applying a definition or starting a server that loads them from storage. A successful validation receipt describes schema, PromQL types and generated-expression checks, rather than claiming telemetry or SLO correctness.
