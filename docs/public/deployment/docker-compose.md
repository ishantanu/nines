# Docker Compose

Use the repository root and a current Docker installation with Compose. This deployment runs one server and PostgreSQL 17, binds the UI to localhost and stores the database in its own named volume. The image is built from source; no published registry image is assumed.

## Login credentials

- Web username: `admin`.
- Web password: `NINES_BOOTSTRAP_PASSWORD` in your private `.env`. There is no fixed persistent password.
- Database password: `POSTGRES_PASSWORD` in `.env`, used only by PostgreSQL and the server connection.
- API key: `API_TOKEN` in `.env`, used separately for automation.

The setup command below generates these values. `admin` / `admin` belongs to in-memory `-demo` mode and the separate loopback playground. It is not the persistent Compose password.

## Start a persistent workspace

```sh
git clone https://github.com/ishantanu/nines.git
cd nines
python3 scripts/setup-local.py
```

This creates `.env` with private file permissions and independent random API, database and bootstrap credentials. It refuses to replace an existing file. Keep `.env` outside Git. For manual setup, copy `.env.example` and replace every credential placeholder; the bootstrap password must be 12–72 bytes and the database password must be URL-safe. No model provider is required.

```sh
docker compose -p nines up --build -d
docker compose -p nines ps
curl --fail http://127.0.0.1:8080/healthz
docker compose -p nines logs --tail=100 platform
```

Open [localhost:8080](http://127.0.0.1:8080) and sign in as `admin` with `NINES_BOOTSTRAP_PASSWORD` from `.env`. This is server-authenticated login. Use **Access control → Users** for profiles, passwords and tenant roles. `admin` / `admin` applies only to the separate synthetic `-demo` mode. Initial persistent data is empty. In a separate terminal, inject the API token without including it in shell history:

```sh
# Read interactively, or inject through a secret manager.
read -r -s NINES_API_KEY
export NINES_API_KEY
make cli
bin/nines config set-context local --server http://127.0.0.1:8080 --tenant default
bin/nines get slos --context local --agent
bin/nines plan --context local -f examples/api-availability.slo.yaml --agent
# Review the plan. For a new SLO, creation expects revision zero.
bin/nines apply --context local -f examples/api-availability.slo.yaml --expected-revision 0 --agent
```

The example queries require suitable instrumentation and a configured source to produce observations. Applying the YAML does not create request metrics.

## Connect Prometheus or Mimir

Legacy single-source evaluation reads `PROMETHEUS_URL` or `MIMIR_URL` from `.env`; configure one, then recreate the server. A provider on the host is not reachable as `localhost` inside the container. Use a reachable service name or an explicitly configured host gateway.

For owner-managed query connections, set `NINES_SOURCE_HOSTS` to exact approved hostnames in `.env`, recreate `platform`, then use **Telemetry connections → Add telemetry connection**. `scripts/setup-local.py` supplies the private encryption key used for saved bearer tokens. Keep this key with your protected recovery material. Mimir organizations must also be authorized in trusted tenant configuration. Saving a source does not deploy rules or create an upstream metrics tenant.

For a complete example with rule volumes, reloads, Mimir ingestion and two severity policies, use the separate [integration playground](playground.md).

## Named tenants and SSO

Named sources, ruler destinations and custom identity configuration require mounted files. Use an additional Compose file such as:

```yaml
services:
  platform:
    env_file:
      - ${NINES_RUNTIME_ENV_FILE:?Set an absolute runtime environment file path}
    environment:
      TENANTS_FILE: /etc/nines/tenants.yaml
      AUTH_CONFIG_FILE: /etc/nines/auth.yaml
    volumes:
      - ${NINES_CONFIG_DIR:?Set an absolute config directory}/tenants.yaml:/etc/nines/tenants.yaml:ro
      - ${NINES_CONFIG_DIR:?Set an absolute config directory}/auth.yaml:/etc/nines/auth.yaml:ro
```

The default server already mounts local-account configuration. This override selects your custom auth file instead. Keep this override and its secrets outside Git. Make the YAML readable by container UID 65532. The runtime environment file supplies every secret referenced by those YAML files. The base Compose file still requires its `API_TOKEN` and database password. Explicit `environment` entries take precedence over the runtime file. With `TENANTS_FILE`, grant access using the credentials declared in that file. See [identity and tenants](identity-and-tenants.md).

```sh
docker compose -p nines -f compose.yaml -f /etc/nines/identity.compose.yaml up -d --build
```

For a shared deployment, use an HTTPS reverse proxy and set `NINES_PUBLIC_URL` to its origin. Custom OIDC configuration must use the same public origin. Keep port 8080 bound to loopback or a private proxy network. The [Linux guide](linux-service.md#https) includes a proxy example.

## Stop, upgrade and back up

```sh
docker compose -p nines stop platform
docker compose -p nines start platform
```

PostgreSQL applies `POSTGRES_PASSWORD` during initialization of an empty volume. Changing the environment value alone does not rotate a retained database-role password; update the role through an authenticated database administration session and keep the server environment synchronized.

Stopping or running `docker compose down` without `--volumes` retains database storage. Do not use `down --volumes` for an instance whose data you need. Follow the [backup and upgrade guide](backup-and-restore.md) before rebuilding a retained instance.

## Confirm the install

1. Sign in, create a viewer under **Access control → Users**, and verify its restricted access in a separate browser session.
2. Add or configure a telemetry connection and test it. Apply an SLO whose queries match real metrics.
3. Open its Integration status and Performance tabs. Review the Rules plan before applying it, then confirm external evaluation.
4. Restart `platform` and confirm accounts, definitions and history are retained.

To use another host port, set `NINES_PORT` and the matching `NINES_PUBLIC_URL` together. Changing the bootstrap environment password does not reset an existing account; use the Users tab. A token-only or SSO-only deployment can replace the auth mount and `AUTH_CONFIG_FILE` through the override above.
