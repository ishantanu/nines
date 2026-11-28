# Collector scheduling

- The default `-process-role=all` runs one combined server. Existing CLI, API, MCP and operator resource contracts are unchanged.
- PostgreSQL deployments can separate scheduled collection and evaluation from the HTTP server with `-process-role=collector`.
- Keep **one control-plane process**. Rule publication, notifications, identity, workflows and other control-plane workers are not made highly available by this change.
- No Kafka, NATS or Redis service is required. PostgreSQL owns a coalescing queue with one job row per SLO.

## Run separate collectors

Use the same reviewed binary, PostgreSQL database, tenant configuration and source configuration for every process. Load secrets through your deployment's secret manager; the binary does not automatically source a checkout's `.env`.

```sh
# Terminal/service 1: API, UI and control-plane workers; no scheduled collection.
nines-server -process-role=control-plane

# Terminal/service 2: collection and SLO evaluation; no HTTP listener.
nines-server -process-role=collector -collection-workers=4

# Terminal/service 3: optional second collector using the same configuration.
nines-server -process-role=collector -collection-workers=4
```

- Supply `DATABASE_URL` and, when applicable, `TENANTS_FILE`, `AUTH_CONFIG_FILE` and the credential environment variables referenced by those files to each process.
- Saved telemetry connections also require the same encryption key, provider allowlists and server-owned routing configuration. A collector cannot choose another tenant's database or Mimir organization.
- Collector-only startup skips model discovery, replay, workflow repositories, rule deployment and HTTP identity discovery. It does not accept `-demo` or `-memory`.
- For Kubernetes, run one control-plane Deployment with the first argument and a separate collector Deployment with the second. Use the same image, configuration and secret references. Start with two collector replicas; configure restart policies and resource limits. Collectors have no HTTP readiness endpoint, so do not copy the portal's HTTP probes onto them.
- For systemd, use separate units with these arguments and the same protected environment file. See [Linux service](linux-service.md) and [Kubernetes deployment](kubernetes.md) for the surrounding deployment setup.

## Scheduling and failure behavior

- `-collection-workers` bounds scheduled jobs per process, from 1 to 32; the default is 4. Tenants are visited round-robin, with at most one active job per tenant per process. Two replicas can therefore run two jobs for a tenant, subject to their global limits.
- Replicas claim individual SLOs with PostgreSQL row locks and a fresh UUID ownership token. Jobs have a 60-second lease and a 45-second execution deadline. A stale worker cannot commit samples, collection health or alert transitions after expiry or replacement.
- Successful jobs become due 15 seconds after completion. Failed attempts retry after 5 seconds, doubling to a maximum of 5 minutes. The queue coalesces missed runs rather than accumulating an unbounded backlog.
- Abrupt worker failure leaves its job claim until the lease expires. Another replica can then claim it; polling, provider latency and queue load add to recovery time. Graceful shutdown cancels active queries.
- Queries can run more than once after a failure. Fencing protects local commits; it does not cancel requests already accepted by a remote provider. Notification delivery retains its existing at-least-once semantics.
- Automatic collection reads at most 300 intervals per batch and retains 24 hours or two rolling windows, whichever is longer. Collection resumes within that recent batch; it does not queue a cursor through older gaps. Missing intervals stay unknown. Gaps beyond the latest 300 intervals require an explicit supported historical collection request and available upstream history.
- Interactive collection shares a per-tenant/process admission limit. If collection is already running, the API returns unavailable (HTTP 503) instead of waiting behind a slow query. Retry later; it is not a permission failure.
- Scale based on measured lag and database/provider limits. There is no automatic scaling controller. Source rate limits, expensive PromQL and database connection budgets can become bottlenecks before worker count does.

## Database and telemetry budgets

- Collector-only stores default to two PostgreSQL connections per tenant, per process, unless `pool_max_conns` is explicitly configured. Budget additional pools for managed-source identity storage and the control plane. Multiplying tenants and replicas multiplies connections.
- Export metrics through the existing OpenTelemetry configuration: `nines.collector.jobs`, `nines.collector.duration` and `nines.collector.schedule.lag`. Labels include tenant and outcome, without SLO IDs, queries or provider credentials. Nines does not add a Prometheus `/metrics` endpoint here. See [monitor Nines itself](monitoring.md) for every signal and suggested alerts.
- Track scheduling lag, source failures, provider throttling, database connections and storage growth. Backups and PostgreSQL availability remain deployment responsibilities.

## Measured acceptance envelope

A local reference run used PostgreSQL 17, two collector replicas with four workers each, and a loopback synthetic Prometheus-compatible source. Each profile ran for 30 seconds after seeding at most 600 intervals per SLO. API reads ran concurrently. The workload runner uses real Nines collectors, transactions and HTTP handlers; it does not run the Prometheus query engine or paid vendor APIs.

| Profile     | Tenants / SLOs | Provider behavior                                         | Job p95 | Schedule lag p95 | API p95 / errors | Max concurrent queries |
| ----------- | -------------- | --------------------------------------------------------- | ------- | ---------------- | ---------------- | ---------------------- |
| Baseline    | 2 / 20         | Immediate                                                 | 31 ms   | 1.00 s           | 7.8 ms / 0       | 2                      |
| Long window | 8 / 200        | 10 ms latency; every tenth request 429; missing intervals | 76 ms   | 3.93 s           | 8.8 ms / 0       | 8                      |
| Slow source | 4 / 20         | 150 ms latency per request                                | 463 ms  | 1.44 s           | 9.0 ms / 0       | 8                      |

- All tenants progressed. The long-window run attempted 469 jobs across all 200 SLOs, observed 88 throttled requests and never queried more than 300 intervals. Incomplete coverage remained incomplete.
- Database relation sizes grew from 15.3 MB to 20.9 MB in the long-window profile, with 167,760 sample rows. This short run is not a retention-sized capacity or sustained ingestion benchmark.
- Peak sampled Go heap was 11.7 MB; process maximum RSS reached 46.1 MB. These figures include the in-process HTTP fixtures, exclude PostgreSQL and reflect this macOS ARM64 run. CPU usage was 2.1–5.4 CPU-seconds per 30-second profile, excluding seed setup. Maximum RSS is cumulative across profiles.
- Race-enabled PostgreSQL tests separately verify concurrent claims, cross-tenant token rejection, expiry during commit, replica takeover, duplicate notification prevention and restart recovery. Required collector cases must pass without skips.
- Reproduce with `make test-collector` (Docker, Go and Python required). It owns and removes one disposable PostgreSQL container, uses generated credentials and makes no paid provider calls. Sanitized results go to ignored `collector-acceptance.json`. CI runs the same gate.
- The checked-in reference report is `integration/collector/reports/2026-11-02.json`. It records the base commit, modified-tree status and source hashes. Treat it as evidence for this workload, not a production throughput guarantee or full-platform HA certification.

## Architecture decision: PostgreSQL job ownership

**Status:** accepted for the bounded collector deployment described above.

- Store job ownership alongside the SLO and its samples so lease checks and measurement commits share a transaction. This avoids a database/broker dual-write boundary and an additional operational dependency.
- Prefer one coalesced job per SLO over enqueueing every tick. Delayed work remains bounded and tenants can make progress independently.
- Use lease tokens in addition to expiry: an old worker must not overwrite a replacement worker even if its provider request eventually succeeds. Check ownership before mutation and again before committing.
- Retain one control plane rather than claiming that collector fencing also coordinates every background worker. Full-platform failover, large asynchronous backfills and sustained high-cardinality capacity require separate work.
- Reconsider a durable broker when measured throughput, fan-out or independent event consumers justify it. A broker alone would not replace tenant isolation, transactional fencing or idempotent writes.
