# Nines v0.1.0-alpha.3

An early-access release of the self-hosted SLO and critical user journey platform.

- Nines is under active development; APIs may change before a stable release.
- Please report bugs and feature requests through GitHub Issues.

## Changes since alpha.2

- Reviewed SLO retirement resolves current references, freezes retained history and retries owned rule cleanup before separately confirmed permanent deletion. Audit receipts survive deletion and identifiers remain reserved.
- Observed dependency evidence qualifies actual counter sample age and series continuity, matches declared links and links journey steps. An optional instrumented HTTP example runs locally with Prometheus or Mimir.
- SLOs own their measurements and support same-name revisioned edits with retained historical definitions. The portal adds guided critical user journey forms, synchronized SLO editor tabs, canonical tenant URLs and live source-measurement reads.
- Saved SLO rules reconcile automatically when the owner enables a trusted Prometheus, Mimir or Kubernetes destination. Configuration, activation, runtime evaluation and measured attainment remain separate evidence.
- [Service dependency maps](service-dependencies.md) add visual editing, saved layouts and directed links. Kubernetes resource imports and optional namespace-scoped cluster reads produce reviewable candidates without inferring observed calls or criticality.
- SLI charts show measured attainment and target with a single series; incomplete history stays qualified. Keyboard time inspection, data tables, accessible theme contrast and responsive chart alignment improve analysis.
- Measurement-quality requirements, bounded cohort inspection, minimum alert traffic and error-budget response policies support reviewed operational decisions. Intent changes enforce current evidence at apply time and preserve independent approval, audit, post-change observation and reviewed rollback.
- Categorized SLI templates and pinned declarative extensions cover services, pipelines, storage, agents, LLM applications and inference. Optional synthetic AI workloads exercise evaluation quality/coverage, budget and latency SLOs through Prometheus or Mimir.
- Optional native Jev advice provides consented category and definition-clarity judgments with audit receipts and daily tenant limits. The Compose override works with Prometheus or Mimir; ordinary playgrounds remain usable without any model account.
- A separate platform admin console manages local accounts, tenant grants, session revocation and Jev policies. Identity acceptance includes real OpenLDAP TLS, controlled signed OIDC fixtures and the native CLI authorization flow.
- Session replay adds selective masking, visible playback failures and a portable HTML export. Optional immutable audit archival supports AWS S3 and explicitly configured S3-compatible storage, with disposable Ceph RGW acceptance.
- Website-hosted documentation adds chapter navigation, local search and downloadable examples. Docs load about 9.5 KB of JavaScript, reuse the page shell between chapters and retain usable navigation in narrow windows.

## Upgrade notes

- Back up the complete PostgreSQL database, tenant/authentication files and managed-source encryption key before replacing a persistent instance.
  - Follow [backup, restore and upgrades](deployment/backup-and-restore.md); an older binary is not guaranteed to read a database initialized by this candidate.
  - Provider-bound browser and CLI sessions created before the session-binding change may require one fresh sign-in.
- Keep the same Compose project, credentials, volumes and intended override/profile selection.
  - Do not use `down -v` for an upgrade.
  - The source checkout supplies the optional Jev override; select it explicitly and inject the provider key privately.
  - A key alone enables no outbound request.
- Review current CRDs and the operator deployment guide before replacing a Kubernetes controller. Recorded rule output is revision-scoped; retained older series do not establish current-definition evaluation.

## Experimental boundaries

- Model-assisted preparation, template selection and session replay remain experimental.
  - The Jev smoke test does not certify a generative model, task correctness or production calibration.
  - Real Anthropic and suitably capable local-model workflow acceptance remain tracked in [issue 34](https://github.com/ishantanu/nines/issues/34).
  - No automatic rollback is performed; restoring configuration cannot recover consumed error budget.
- The synthetic AI workload is an instrumentation/measurement demonstration, not a real application agent. Desktop-browser/offline replay acceptance, organization-specific Entra/directory behavior, real production scale and operational recovery remain environment-specific checks. See [experimental features](experimental.md) and the [verification record](verification.md).

## Downloads

- Choose the archive matching your operating system and CPU. Each archive contains `nines`, `nines-server`, `nines-mcp` and `nines-operator`, plus configuration examples, reference documentation and dependency licenses.

| Platform                | Archive suffix        |
| ----------------------- | --------------------- |
| Linux, Intel/AMD 64-bit | `linux_amd64.tar.gz`  |
| Linux, ARM 64-bit       | `linux_arm64.tar.gz`  |
| macOS, Intel            | `darwin_amd64.tar.gz` |
| macOS, Apple silicon    | `darwin_arm64.tar.gz` |

- Verify the downloaded archives against `SHA256SUMS` before extracting. `build.json` identifies the exact source commit, version, target architectures and archive hashes. The checksums provide integrity checking; these binaries are not code-signed or notarized.

## Quick start

After extracting the archive:

```sh
./nines version --agent
./nines-server -demo
```

- Open <http://127.0.0.1:8080> and sign in with **admin / admin**.
  - The synthetic in-memory demo resets on restart.
  - On macOS, downloaded executables may require explicit approval under Privacy & Security because they are not notarized.
- The CLI can validate YAML and generate recording rules without a server.
  - Connected operation requires the correct tenant, credentials and a running Nines server.
  - Persistent server operation requires PostgreSQL and explicit provider/authentication configuration.
  - The Compose playground requires a source checkout; the binary archive is not a container deployment bundle.

## Scope

- SLOs and critical user journeys as versioned YAML, Prometheus/Mimir collection and recording rules, configurable severity-based burn alerts, tenant-aware views, local accounts, configured SSO/LDAP, RBAC, the Go client and stdio MCP adapter.
- The standalone Kubernetes operator is included, with namespaced RBAC, leader failover and Prometheus Operator/Mimir reconciliation.
  - See the operator guide before installing CRDs or deploying its image.
  - Intent workflows and session replay remain experimental.
  - This alpha does not establish production-scale readiness or stable API compatibility.
  - Windows binaries and a hosted service are not included.
