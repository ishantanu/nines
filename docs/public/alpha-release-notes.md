# Nines v0.1.0-alpha.4

An early-access release of the self-hosted SLO and critical user journey platform.

- Nines is under active development; APIs may change before a stable release.
- Please report bugs and feature requests through GitHub Issues.

## Changes since alpha.3

- The portal, public site and documentation pages use Astryx with the shared `nines` theme and self-hosted Figtree. Documentation renders Astryx headings, links, code, quotes and tables as static HTML without adding client script. The login page, Home overview and SLI charts are redesigned.
- New SLO practices, each opt-in and deterministic, with missing data kept unknown: [low-traffic confidence for burn alerts](measurements.md), [labels and roll-ups](measurements.md#classify-objectives-with-labels), [graduated budget response levels](error-budget-policy.md#agree-responses-for-each-budget-level), [journey run objectives](critical-user-journeys.md#measure-an-agentic-journey-from-its-runs), [session SLIs](measurements.md#measure-sessions-instead-of-requests), [good-minute SLOs](measurements.md#judge-intervals-instead-of-events), [per-customer reports](measurement-quality.md#report-an-slo-for-key-customers), failure concentration, [peer comparison](measurement-quality.md#compare-cohorts-with-their-peers), [composed availability ceilings](service-dependencies.md), a [target-change check](slo-editing.md#review-a-target-change-against-delivered-reliability) and an [adoption review](slo-guide.md#keep-objectives-few-user-focused-and-linked). See the [SLO practices plan](slo-practices-roadmap.md) for each item's status and limits.
- Every SLI kind from the SRE book is an [SLO type](slo-types.md) with a guided form: availability, latency, correctness, durability, freshness, coverage and throughput, plus sessions, good-minute and journey run objectives. The portal shows the type on each objective and filters by type. The Define dialog asks one **SLO type** question (defaulting to availability), then a name, service, metric and filters, and keeps identifiers, intervals, templates and optional settings collapsed. Per-customer reporting can be turned on from the same select.
- [Create from a goal](intents.md) (SLO intents) is the reviewed path: a goal, telemetry evidence and a backtest, then a separate approval before apply. It uses the same SLO types and wording as Define objective and works without a model; only model drafting and model-directed runs are experimental.
- Optional [Git delivery](git-delivery.md) on GitHub and GitLab opens a pull or merge request with the exact reviewed SLO or journey definition instead of applying it. It reports applied state and drift, and it never merges or deploys.
- [Migrate from Sloth or Pyrra](migration.md): offline conversion with per-SLO notes, explicit skips and an experimental-by-default lifecycle, through a portal dialog, `nines import` and the API.
- Native CLI device login with optional operating-system credential storage, plus read-only intent measurement candidates, service context, evidence comparison, preparation-gap evidence and review packets across API, Go client, CLI and MCP.

## Upgrade notes

- Back up the complete PostgreSQL database, tenant/authentication files and managed-source encryption key before replacing a persistent instance.
  - Follow [backup, restore and upgrades](deployment/backup-and-restore.md); an older binary is not guaranteed to read a database initialized by this candidate.
  - Provider-bound browser and CLI sessions created before the session-binding change may require one fresh sign-in.
- Provider credential fingerprints now use a separate Argon2id-derived identity. Gather fresh source evidence and re-review pending route-bound plans after updating; this does not change local account password storage.
- Keep the same Compose project, credentials, volumes and intended override/profile selection.
  - Do not use `down -v` for an upgrade.
  - The source checkout supplies the optional Jev override; select it explicitly and inject the provider key privately.
  - A key alone enables no outbound request.
- The SLO CRD schema gained optional fields (labels, burn-policy confidence, good-minute and throughput intervals, per-customer lists, session measurements and budget response levels), and `measurement.type` accepts correctness, durability, coverage and freshness. Apply the shipped `deploy/operator/crds.yaml` before upgrading the controller; existing resources remain valid.
- Git delivery is off unless a tenant configures `gitOps` with a credential environment variable. Nothing changes for existing tenants.
- Review current CRDs and the operator deployment guide before replacing a Kubernetes controller. Recorded rule output is revision-scoped; retained older series do not establish current-definition evaluation.

## Experimental boundaries

- Git delivery and Sloth/Pyrra migration are new in this alpha. The GitHub and GitLab adapters are tested against in-memory fakes of their APIs, not live services, and the importers are checked against the projects' published examples, not every real-world file. Review each proposal and each converted definition before relying on it. Converted alert thresholds are standard burn-rate policies, not a copy of your current alerts.
- The newer SLO types depend on the provider: sessions, good-minute and throughput SLOs and per-customer reports work with Prometheus and Mimir; sessions, good-minute and throughput SLOs are accepted for Dash0 but not run against a live account; VictoriaMetrics accepts good-minute and throughput only as untested PromQL; and Datadog, New Relic and Google Cloud accept none of them. See the [provider table](slo-types.md#which-providers-support-which-types).
- Session, good-minute and per-customer SLOs refuse unsupported combinations with a reason (for example per-customer reports on sessions, or good-minute on provider-evaluated windows) instead of partly working.

- Model-assisted preparation, template selection, session replay, remote MCP HTTP/OAuth and the additional provider adapters remain experimental.
  - The Jev smoke test does not certify a generative model, task correctness or production calibration.
  - Real Anthropic and suitably capable local-model workflow acceptance remain tracked in [issue 34](https://github.com/ishantanu/nines/issues/34).
  - No automatic rollback is performed; restoring configuration cannot recover consumed error budget.
- Paid-provider calls are not required by ordinary CI. Synthetic adapter contracts do not certify every vendor account, region or query profile; live paid-provider checks require explicit opt-in and are separate from ordinary CI. Remote MCP has SDK/local OAuth fixture coverage; hosted agent-client interoperability requires deployment-specific acceptance.
- The synthetic AI workload is an instrumentation/measurement demonstration, not a real application agent. Desktop-browser/offline replay acceptance, organization-specific Entra/directory behavior, real production scale and operational recovery remain environment-specific checks. See [experimental features](experimental.md) and the [verification record](verification.md).

## Downloads

- Choose the archive matching your operating system and CPU. Linux and macOS archives contain `nines`, `nines-server`, `nines-mcp` and `nines-operator`; Windows archives contain `nines.exe`, `nines-server.exe` and `nines-mcp.exe`, because the operator runs only in Linux containers. Every archive includes configuration examples, reference documentation and dependency licenses.

| Platform                  | Archive suffix         |
| ------------------------- | ---------------------- |
| Linux, Intel/AMD 64-bit   | `linux_amd64.tar.gz`   |
| Linux, ARM 64-bit         | `linux_arm64.tar.gz`   |
| macOS, Intel              | `darwin_amd64.tar.gz`  |
| macOS, Apple silicon      | `darwin_arm64.tar.gz`  |
| Windows, Intel/AMD 64-bit | `windows_amd64.zip`    |
| Windows, ARM 64-bit       | `windows_arm64.zip`    |

- Verify the downloaded archives against `SHA256SUMS` before extracting. `build.json` identifies the exact source commit, version, target architectures and archive hashes. The checksums provide integrity checking; these binaries are not code-signed or notarized.
- Container images for `linux/amd64` and `linux/arm64` are published as `ghcr.io/ishantanu/nines` and `ghcr.io/ishantanu/nines-operator`, tagged with this version, signed with keyless Cosign and with build provenance. Their digests are listed at the end of these notes. See [container images](https://github.com/ishantanu/nines/blob/v0.1.0-alpha.4/docs/public/deployment/container-images.md).

## Quick start

After extracting the archive (on Windows, run `.\nines.exe` and `.\nines-server.exe`):

```sh
./nines version --agent
./nines-server -demo
```

Or run the demo in a container. A container listens on all interfaces, so it needs a random `API_TOKEN`. In the browser, choose **Set API token** and paste it before the demo sign-in:

```sh
export API_TOKEN="$(openssl rand -hex 24)"; echo "$API_TOKEN"
docker run --rm -p 127.0.0.1:8080:8080 -e API_TOKEN ghcr.io/ishantanu/nines:v0.1.0-alpha.4 -demo
```

- Open <http://127.0.0.1:8080> and sign in with **admin / admin**.
  - The synthetic in-memory demo resets on restart.
  - On macOS, downloaded executables may require explicit approval under Privacy & Security because they are not notarized.
- The CLI can validate YAML and generate recording rules without a server.
  - Connected operation requires the correct tenant, credentials and a running Nines server.
  - Persistent server operation requires PostgreSQL and explicit provider/authentication configuration.
  - The Compose playground requires a source checkout; for containers, use the published images.

## Scope

- SLOs and critical user journeys as versioned YAML, Prometheus/Mimir collection and recording rules, configurable severity-based burn alerts, tenant-aware views, local accounts, configured SSO/LDAP, RBAC, the Go client and stdio MCP adapter. Additional providers and OAuth-protected remote MCP are opt-in experimental capabilities with separate deployment guides.
- The standalone Kubernetes operator is included, with namespaced RBAC, leader failover and Prometheus Operator/Mimir reconciliation.
  - See the operator guide before installing CRDs or deploying its image.
  - Intent workflows and session replay remain experimental.
  - This alpha does not establish production-scale readiness or stable API compatibility.
  - A hosted service is not included. Windows archives contain the CLI, server and MCP server; the operator runs only in Linux containers.
