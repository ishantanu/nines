# Nines alpha

An early-access release of the self-hosted SLO and critical user journey platform. Nines is under active development; APIs may change before a stable release. Please report bugs and feature requests through GitHub Issues.

## Downloads

Choose the archive matching your operating system and CPU. Each archive contains `nines`, `nines-server` and `nines-mcp`, plus configuration examples, reference documentation and dependency licenses.

| Platform | Archive suffix |
| --- | --- |
| Linux, Intel/AMD 64-bit | `linux_amd64.tar.gz` |
| Linux, ARM 64-bit | `linux_arm64.tar.gz` |
| macOS, Intel | `darwin_amd64.tar.gz` |
| macOS, Apple silicon | `darwin_arm64.tar.gz` |

Verify the downloaded archives against `SHA256SUMS` before extracting. `build.json` identifies the exact source commit, version, target architectures and archive hashes. The checksums provide integrity checking; these binaries are not code-signed or notarized.

## Quick start

After extracting the archive:

```sh
./nines version --agent
./nines-server -demo
```

Open http://127.0.0.1:8080 and sign in with **admin / admin**. The synthetic in-memory demo resets on restart. On macOS, downloaded executables may require explicit approval under Privacy & Security because they are not notarized.

The CLI can validate YAML and generate recording rules without a server. Connected operation requires the correct tenant, credentials and a running Nines server. Persistent server operation requires PostgreSQL and explicit provider/authentication configuration. The Compose playground requires a source checkout; the binary archive is not a container deployment bundle.

## Scope

SLOs and critical user journeys as versioned YAML, Prometheus/Mimir collection and recording rules, configurable severity-based burn alerts, tenant-aware views, local accounts, configured SSO/LDAP, RBAC, the Go client and stdio MCP adapter.

Intent workflows, session replay and the Kubernetes operator remain experimental. This alpha does not establish production-scale readiness or stable API compatibility. Windows binaries and a hosted service are not included.
