# Linux service

This guide runs one native server as an unprivileged systemd service. Build with Go 1.27.3 and use an external PostgreSQL database. The embedded UI is committed; Node is needed only when rebuilding it.

## Build and install

The commands below start from a source checkout. When using a reviewed native release archive instead, use its `nines-server` binary in the install command in place of `nines-server-linux-amd64`; no Go compiler or source build is needed. Match the archive to the host operating system and architecture and verify its checksums first.

```sh
make build
```

When building on macOS for Linux, select the deployment architecture explicitly:

```sh
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/nines-server-linux-amd64 ./cmd/server
```

Use `arm64` for an ARM host. Transfer the matching binary over your normal verified delivery channel. On the Linux host, create the service account and directories, then install the binary:

```sh
sudo useradd --system --home-dir /var/lib/nines --shell /usr/sbin/nologin nines
sudo install -d -o root -g root -m 0755 /opt/nines
sudo install -d -o root -g nines -m 0750 /etc/nines
sudo install -o root -g root -m 0755 ./nines-server-linux-amd64 /opt/nines/nines-server
sudo install -m 0644 deploy/systemd/nines.service /etc/systemd/system/nines.service
```

`useradd` is a one-time step; adapt it if the account already exists or your distribution uses a different `nologin` path. The service manager creates `/var/lib/nines` and `/run/nines` for the service. Keep TLS root CAs installed for PostgreSQL, providers and identity clients.

## Runtime configuration

Create `/etc/nines/nines.env`, owned by root with mode 0600:

```text
DATABASE_URL=postgres://nines:URL_ENCODED_PASSWORD@db.internal:5432/nines?sslmode=verify-full
API_TOKEN=REPLACE_WITH_A_RANDOM_VALUE_OF_AT_LEAST_24_CHARACTERS
# Optional server-owned configuration:
# TENANTS_FILE=/etc/nines/tenants.yaml
# AUTH_CONFIG_FILE=/etc/nines/auth.yaml
# NINES_PUBLIC_URL=https://nines.example.com
# NINES_BOOTSTRAP_PASSWORD=GENERATE_A_RANDOM_PASSWORD_FOR_LOCAL_ACCOUNTS
# NINES_ENTRA_CLIENT_SECRET=...
```

Use a secret manager or a protected editor to set real values. Environment files contain secrets. The native process does not load a checkout's `.env` automatically. Add a `sslrootcert` connection parameter if PostgreSQL uses a private CA, and make the CA readable to the service.

For tenant or identity files, use root ownership, group `nines` and mode 0640. Resolve secret fields through the environment variables named by each YAML file. See [team login](identity-and-tenants.md).

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now nines
sudo systemctl status nines
sudo journalctl -u nines --since '10 minutes ago'
curl --fail http://127.0.0.1:8080/healthz
```

The service writes structured logs to stdout/journald. Standard `OTEL_*` environment variables configure optional telemetry exports. No Prometheus `/metrics` endpoint is advertised by this guide.

## HTTPS

Terminate TLS at a trusted reverse proxy while leaving Nines on loopback. For example, an operator-managed Caddy configuration is:

```caddyfile
slo.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Replace the hostname, configure DNS and certificate issuance, and follow your proxy's deployment policy. For a network-separated proxy, protect the private hop and restrict direct access to Nines. An identity `publicURL` must be the external HTTPS origin. Nines constructs OIDC callbacks from that configured origin, not an arbitrary request host.

The reverse proxy must allow normal API methods, request bodies and streamed responses. Choose timeouts appropriate for the bounded workflow/replay features you enable. Do not add public caching to authenticated API responses.

## Upgrade

Back up the database and configuration first. Stop the service, install the reviewed binary, start it and verify both API access and provider collection. Do not run old and new collecting servers simultaneously. See [backup and restore](backup-and-restore.md).

Reference: Caddy's [reverse proxy quickstart](https://caddyserver.com/docs/quick-starts/reverse-proxy).
