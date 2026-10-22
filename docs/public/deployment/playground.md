# Prometheus playground

This synthetic local stack demonstrates the complete measurement and rule flow with Prometheus. It needs Docker Compose v2 and a `POSTGRES_PASSWORD` supplied through your shell or a private `.env`. No host Go, Node, PostgreSQL or existing metrics backend is needed. The first run builds Nines from this checkout and downloads container images. Mimir is available through an [optional profile](#optional-mimir-profile).

## Credentials and first startup

| Credential | Value or source |
| --- | --- |
| Web username | `admin` |
| Web password | `admin`; override with `NINES_PLAYGROUND_PASSWORD` before first startup |
| Database password | Required `POSTGRES_PASSWORD` in `.env` or the shell; no default |
| API token | `nines-local-playground-owner-token` for this synthetic playground only |

The loopback playground explicitly enables the `admin` / `admin` bootstrap account. Ordinary accounts and persistent deployments still require 12–72-byte passwords. The web password, database password and API token are separate. `NINES_BOOTSTRAP_PASSWORD` in the shared `.env` belongs to the separate persistent `compose.yaml` deployment; the playground uses `NINES_PLAYGROUND_PASSWORD`.

For a fresh checkout, `python3 scripts/setup-local.py --playground` generates a private `.env` with a random database password. This optional setup command needs Python 3.9 or later. Alternatively, supply your own URL-safe `POSTGRES_PASSWORD` through the environment or `.env`. Keep the value for future restarts; do not regenerate it on every invocation. Existing `.env` files are never overwritten by setup.

From the repository root:

```sh
# Fresh checkout only; skip if .env or POSTGRES_PASSWORD is already configured.
python3 scripts/setup-local.py --playground
docker compose -p nines-playground -f compose.playground.yaml up --build -d
docker compose -p nines-playground -f compose.playground.yaml logs -f setup
```

The setup service waits for Nines, creates `checkout-success` in the **Playground Prometheus** workspace and exits. Re-running it preserves existing definitions. A setup failure is visible in its exit code and logs; container startup alone does not prove the flow works.

| Component | Address |
| --- | --- |
| Nines | [localhost:18080](http://127.0.0.1:18080) |
| Prometheus | [localhost:19090](http://127.0.0.1:19090) |
| Synthetic metrics and failure control | `http://127.0.0.1:19092` |

Sign in with username `admin` and password `admin`. Set `NINES_PLAYGROUND_PASSWORD` before the first startup to choose another bootstrap password. The password is hashed and persisted; changing the bootstrap environment variable later does not reset it. API-token access remains available with token `nines-local-playground-owner-token`. The default web password and API token are deliberately public and for this loopback-only synthetic stack; the database password is supplied privately. Do not use this configuration for shared or production deployments. All published ports bind to loopback. Existing services on 8080 and 9090 are not used. Override ports with `NINES_PLAYGROUND_PORT`, `NINES_PLAYGROUND_PROMETHEUS_PORT` and `NINES_PLAYGROUND_SAMPLE_PORT` if needed.

## Follow the four states

1. Open **SLOs**, then **Checkout success** in **Playground Prometheus**. **SLO saved** confirms the stored definition. **Measurements collected** confirms recent provider intervals for that revision; expect a brief initial wait for scrapes and the collector.
2. **Rules deployed** initially reads **Not deployed**. Creating the SLO and collecting measurements do not create external metrics.
3. Open the SLO's **Rules** tab. Review the generated group and destination, then select **Deploy reviewed rules**. Nines writes the owned file to a shared volume. The playground's separate rule reloader observes file changes and asks only its own Prometheus to reload. Production deployments must supply their own reload mechanism.
4. The Integration status panel refreshes every 15 seconds, or use **Check status**. **Rules deployed** reports whether the destination matches the current definition. **Rules evaluating** requires the expected loaded group and recent successful evaluations of every rule. Failed, stale, missing, changed and unavailable evidence stay distinct.

Prometheus scrapes a synthetic checkout service every five seconds and evaluates its own Nines recording and alert rules. The default stack has no remote-write destination, Mimir container or Mimir workspace. No telemetry or model account is required.

The example has a 10-minute SLO window, 10-second measurement intervals and 60/120-second critical and 120/300-second warning burn windows. They are intentionally short for testing. Recording-rule history starts when rules are activated; Nines collection does not backfill external recordings. Full SLO coverage takes ten minutes. External evaluator health does not establish traffic coverage or notification delivery. This stack tests alert state, and does not include Alertmanager or notification receivers.

## Query the results

In Prometheus, inspect [Rules](http://127.0.0.1:19090/rules) and query:

```promql
nines:slo_target:ratio{nines_tenant="prometheus",nines_slo="checkout-success"}
nines:sli:ratio{nines_tenant="prometheus",nines_slo="checkout-success",window="60s"}
nines:slo_burn_rate:ratio{nines_tenant="prometheus",nines_slo="checkout-success",window="60s"}
nines:slo_coverage:ratio{nines_tenant="prometheus",nines_slo="checkout-success",window="60s"}
```

For a host CLI built with `make cli`:

```sh
export NINES_URL=http://127.0.0.1:18080
export NINES_API_KEY=nines-local-playground-owner-token
bin/nines integration checkout-success --tenant prometheus --agent
```

The equivalent authenticated endpoint is `GET /api/slos/checkout-success/integration`, with `X-Nines-Tenant` selecting the authorized workspace. It performs read-only provider checks; it does not write rules or reload a provider.

## Exercise burn alerts and recovery

This command explicitly deploys the current sample rules, waits for collection/evaluation/recorded series, checks local user CRUD, session revocation, RBAC and encrypted source management, injects 10% synthetic failures, checks critical and warning alert policies in Nines and Prometheus, restores successful traffic and checks recovery. It exits nonzero on timeout or failed assertions. Allow several minutes:

```sh
docker compose -p nines-playground -f compose.playground.yaml run --rm verify
```

The verifier restores successful traffic in a `finally` block after alert testing. If interrupted or killed, reset it explicitly. To control failures manually:

```sh
curl -fsS -X POST 'http://127.0.0.1:19092/failure?percent=10'
# Allow the burn windows and pending duration to fill, then inspect Rules/Alerts.
curl -fsS -X POST 'http://127.0.0.1:19092/failure?percent=0'
```

The sample service increments synthetic counters; it is not an actual checkout application or load test. Nines' own HTTP/runtime telemetry remains a separate optional OTLP integration, not the SLO recording rules demonstrated here.

## Optional Mimir profile

Use this configuration to test remote-write ingestion, tenant headers and Mimir ruler destinations alongside Prometheus. Both the override file and `mimir` profile are required:

```sh
docker compose -p nines-playground \
  -f compose.playground.yaml -f compose.playground.mimir.yaml \
  --profile mimir up --build -d

docker compose -p nines-playground \
  -f compose.playground.yaml -f compose.playground.mimir.yaml \
  --profile mimir run --rm verify
```

The override adds a **Playground Mimir** workspace and owner membership, enables Prometheus remote write to Mimir, and tells setup/verification to exercise both backends. Prometheus sends only source metrics; its Nines recording rules are excluded so Mimir evaluates its own group. Each workspace has its own PostgreSQL schema. Select **Playground Mimir** to review and deploy its rules to ruler namespace `playground`, pinned to organization `playground`.

Mimir listens on `http://127.0.0.1:19009`; override that port with `NINES_PLAYGROUND_MIMIR_PORT` if needed. Query it with the organization header:

```sh
curl -fsSG http://127.0.0.1:19009/prometheus/api/v1/query \
  -H 'X-Scope-OrgID: playground' \
  --data-urlencode 'query=nines:sli:ratio{nines_tenant="mimir",window="60s"}'

bin/nines integration checkout-success --tenant mimir --agent
```

Use the same override/profile selection for logs and shutdown. To return to Prometheus-only mode, stop the optional configuration first, then start the base configuration:

```sh
# Preserve the database, rules and both backends' history:
docker compose -p nines-playground \
  -f compose.playground.yaml -f compose.playground.mimir.yaml \
  --profile mimir down

docker compose -p nines-playground -f compose.playground.yaml up --build -d
```

This also applies to playgrounds started before Prometheus became the default. Mimir data remains in its volume and reappears when the optional configuration is enabled again. Its workspace is absent from the base configuration. To remove all synthetic data from both modes, use the optional shutdown command with `down -v` instead.

The dual-backend verifier checks independent critical/warning alert firing and recovery in Nines and both evaluators. Mimir has a default one-minute ruler evaluation delay; this stack retains that behavior. Allow for provider delay when choosing windows and interpreting alert timestamps. See the [Mimir ruler delay configuration](https://grafana.com/docs/mimir/latest/configure/configuration-parameters/).

## Database password changes

PostgreSQL initializes its database-role password only on an empty data volume. Changing `POSTGRES_PASSWORD` in `.env` alone does not rotate a retained database password. Rotate the database role with an authenticated PostgreSQL administration session, then update `.env` and recreate the database/server containers. Keep the existing value until the rotation is complete. Do not delete a volume to change a password when you need its data. The same rule applies to a playground created before database passwords became mandatory environment configuration.

## Stop and reset

For the default Prometheus-only stack:

```sh
# Stop while keeping playground data and deployed rules:
docker compose -p nines-playground -f compose.playground.yaml down
# Delete only this configuration's synthetic database, Prometheus data and rules:
docker compose -p nines-playground -f compose.playground.yaml down -v
```

Use the same project name for every command. Do not use global container or volume cleanup commands. Rebuild after source changes with `up --build -d`; the database and backend history remain until explicitly removed. If the optional profile was enabled, use its shutdown command above so Mimir is stopped too.

## Users and data sources

Open **Access control → Users** to create local accounts, edit their profiles/passwords, change tenant roles and remove tenant access. Members, teams, external SSO mappings, service accounts and audit records remain separate tabs. Shared local identity changes require ownership of every tenant the user can access. Removing membership preserves other tenants; upstream SSO grants must be revoked at their configured source. Password resets revoke browser and CLI sessions.

Open **Data sources → Add data source** to create a managed connection. The default stack approves only host `prometheus`; use `http://prometheus:9090` rather than host loopback. The optional profile also approves `mimir`, with organization `playground` authorized only in its Mimir workspace. Existing YAML-configured connections remain read-only. Managed credentials are write-only API inputs and encrypted in PostgreSQL with a public, synthetic playground key. Production needs a private key backed up separately with its database. Referenced connection identities cannot be edited or deleted; create another connection and review the dependent definitions.

Use the Alert policy selector on the SLO Performance tab to chart each policy's burn windows. The SLO detail lists independent policy states; Rules lists severity labels for notification routing. The sample uses critical `fast-burn` at 6× and warning `sustained-burn` at 2×. The critical policy uses 60s/120s windows and the warning policy uses 120s/300s. These accelerated windows allow for ingestion delay but are for synthetic testing, not production defaults.

On narrow screens, the navigation dropdown includes Data sources and Access control. Existing playground volumes preserve prior SLO definitions. To try the sample policies against an existing stack, review and apply `deploy/playground/sample.slo.yaml` through the YAML editor; startup does not overwrite user definitions.
