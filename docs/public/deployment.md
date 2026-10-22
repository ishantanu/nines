# Deploy Nines

Nines runs as one collecting Go server with an embedded web UI and PostgreSQL for durable data. Start with a local demo, then choose a single-instance deployment. The project currently requires Go 1.27.3 to build from source. Node 24 is needed to rebuild the UI or product site.

| Deployment | Guide | Scope |
| --- | --- | --- |
| Local integration playground | [Prometheus playground](deployment/playground.md) | Synthetic traffic, recording and burn-rate rules; optional Mimir profile |
| Local demonstration | [README quickstart](../../README.md#try-the-demo) | Synthetic, ephemeral and loopback-only |
| Docker Compose | [Persistent container setup](deployment/docker-compose.md) | Recommended first evaluation with a dedicated database volume |
| Linux service | [Native binary and systemd](deployment/linux-service.md) | One server behind a TLS reverse proxy |
| Kubernetes | [Single-replica manifests](deployment/kubernetes.md) | Deployment reference; separate from the experimental operator |
| Kubernetes operator | [Controller and offline rendering](deployment/operator.md) | Experimental CRDs, PrometheusRule output and Mimir ruler reconciliation |
| Team access | [Identity and tenant configuration](deployment/identity-and-tenants.md) | Local accounts, API keys, OIDC/Microsoft Entra, LDAP and roles |
| Metrics and rules | [Prometheus/Mimir routing](deployment/metrics-and-rules.md) | Explicit query connections and reviewed rule destinations |
| Data protection | [Backup, restore and upgrades](deployment/backup-and-restore.md) | Database, configuration and credential recovery |
| Product website | [GitHub Pages](site.md) | Static documentation, separate from the server |

## Before exposing a server

Use a dedicated PostgreSQL database and role. Nines creates tables and tenant schemas at startup; the role needs the corresponding DDL privileges. Keep the database on a private network and use certificate-verified PostgreSQL TLS across untrusted networks. The example Compose database is on its internal Docker network.

Configure random API credentials or server-owned identity settings. Authentication must be paired with an explicit tenant role. Put HTTPS in front of the server and configure an OIDC `publicURL` that exactly matches the external origin. Do not expose the synthetic demo as a team deployment.

Keep one collecting process for the MVP. There is no distributed collector leader election. Multiple replicas can duplicate collection, workflow execution and alert delivery. A rollout must stop the old process before starting its replacement. The Kubernetes reference uses `Recreate` for that reason.

## Verify a deployment

1. Confirm `/healthz` returns HTTP 200. This is a process check, not proof of healthy provider queries, database recovery, SLO coverage or correct rule evaluation.
2. Use a least-privilege client credential with an explicit tenant and verify that the expected workspace is visible.
3. Apply a reviewed SLO, then verify source collection, sample freshness and coverage. Unknown data should remain unknown.
4. Review recording/burn-rate rules and their destination before deployment. Verify provider-side evaluation and Alertmanager delivery separately.
5. Rehearse a database/configuration restore into an isolated deployment before relying on retained history.

The [operational limits](operations.md), [configuration reference](configuration.md) and [release checklist](release.md) define the current MVP boundary. Manifests and guides do not establish HA, production scale, a real identity-provider rollout or a completed restore exercise.
