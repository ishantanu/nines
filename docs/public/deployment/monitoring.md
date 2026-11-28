# Monitor Nines itself

The server and the operator emit OpenTelemetry traces, metrics and logs about their own work: requests, collection, tenants and platform changes. This is separate from the SLOs Nines computes for your services.

## Turn on export

Point Nines at an OpenTelemetry collector that accepts OTLP over HTTP (protobuf):

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.observability.svc:4318
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production
```

- Without an endpoint, logs are written to stdout as JSON and nothing is exported. `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER` and `OTEL_LOGS_EXPORTER` accept `otlp` or `none` per signal.
- With the [Helm chart](kubernetes.md#install-with-helm), set `telemetry.otlpEndpoint` and optional `telemetry.resourceAttributes`. Both the server and the operator then export, and every signal carries `k8s.namespace.name` and `k8s.pod.name`.
- Service names are `nines-server` and `nines-operator`; override with `OTEL_SERVICE_NAME`.
- Exported data never contains credentials, query tokens, request bodies, SLO queries or provider secrets. Errors are reported as a class, such as `network` or `timeout`, not their text.

## What it reports

| Signal | Name | Labels | Shows |
| --- | --- | --- | --- |
| Trace + metric + log | `http.server.request.duration`, `http.server.request.count` | `http.route`, `http.request.method`, `http.response.status_code`, `nines.tenant` | Every request, by exact route: tenant APIs, `/api/platform/tenants/create`, `/auth/cli/device/token` and so on. The tenant is set only after authorization. |
| Metric | `nines.collector.jobs`, `nines.collector.duration`, `nines.collector.schedule.lag` | `nines.tenant`, outcome | Scheduled collection, including tenants created while running. See [collectors](collectors.md). |
| Metric | `nines.tenants` | `state` (active, retired), `managed_by` (configuration, console) | Tenants this process serves. |
| Metric | `nines.tenant.reconcile`, `nines.tenant.reconcile.duration` | `outcome` | Each pass over the tenant registry, every 10 seconds. |
| Metric | `nines.tenant.registry.last_success` | | Unix time of the last successful registry pass. |
| Metric | `nines.tenant.open.failures` | `stage` (open, schedule, mount), `error.class` | Registered tenants a process could not serve. |
| Trace + metric | `nines.operation.duration`, `nines.operation.count` | `nines.operation` (such as `tenant.create`, `tenant.delete`, `workflow.approve`), `nines.tenant`, `outcome` | Platform and workflow operations, with database spans beneath them. |
| Trace + metric | `db.client.operation.duration` | | Every PostgreSQL query, including identity and tenant schemas. |
| Metric | `go.goroutine.count`, `go.memory.used` | | Process health. |

Platform changes are also kept in the [platform audit history](../platform-administration.md#audit-history), with actor, reason and channel (browser or CLI). Audit is the record of who did what; telemetry is for operating the service.

## Suggested alerts

- **A process stopped following the tenant registry:** `time() - nines.tenant.registry.last_success > 120` (seconds) while the process is up. Tenants created or retired elsewhere will not take effect on it.
- **Registry reconcile failing:** a rising `nines.tenant.reconcile{outcome="failed"}`. Usually the identity database is unreachable; already-served tenants keep working.
- **A tenant cannot be served:** any increase in `nines.tenant.open.failures`. `stage` says where it failed and the server log names the tenant.
- **Collection falling behind:** `nines.collector.schedule.lag` above your SLO step, per tenant. See [collectors](collectors.md).
- **Sign-in trouble:** a rising rate of 4xx or 5xx on `/auth/*` routes, such as `/auth/cli/exchange` or `/auth/local/{provider}/login`.
- **Failed tenant deletions:** `nines.operation.count{nines.operation="tenant.delete",outcome="failed"}`. The tenant shows **Deletion incomplete** in the console until deleted again.
