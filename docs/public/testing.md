# Testing

Install the pinned development linters as described in [Contributing](../../CONTRIBUTING.md#lint-tooling), then run `make lint`.

- CI has a dedicated Go/Python lint job that also rejects formatting drift.
- Run `make ui build test check release-check` from a clean checkout. Frontend compilation must finish before Go compilation because the server embeds its output. `go test -race` exercises the CLI, calculations, tenant/API boundaries, synthetic OIDC/LDAP, transactional behavior, rule generation and bounded workflow fixtures.
- Database-dependent cases require `TEST_DATABASE_URL`, pointed at a disposable PostgreSQL database whose user may create schemas.
  - Tests create unique schemas and remove only those.
  - CI provides PostgreSQL and rejects skipped database cases. Do not substitute a production connection.
- `make test-mimir` starts only a uniquely named disposable loopback Mimir container and removes it on exit.
  - It tests synthetic ingestion, tenant isolation, collection and ruler destinations. The runner refuses a non-loopback `MIMIR_TEST_URL`.
  - Tests do not contact a real identity provider, hosted model or Kubernetes cluster unless a separately reviewed harness explicitly does so.
- `make test-ceph` runs a disposable real Ceph RGW/OpenBao fixture with generated credentials and trusted loopback TLS. It checks encrypted audit readback, one-version conditional retries, enforced retention and protected-version deletion denial, then removes its containers and temporary files. See [archive acceptance](deployment/audit-archive.md#disposable-ceph-acceptance) for prerequisites and limits.
- The [Compose playground](deployment/playground.md) defaults to Prometheus.
  - CI runs its complete management/recording-rule/alert lifecycle in a two-mode matrix: Prometheus-only and the optional dual-backend Mimir profile.
  - The same setup and verifier select one or both workspaces from trusted Compose configuration.
- The first public release also requires clean-checkout CI, native CLI/browser identity verification, demo first-run verification, a reviewed dependency/license inventory, current vulnerability scans, container startup and artifact installation checks. Historical local test reports and screenshots are not substitutes for these checks.
- Review [GitHub Actions](https://github.com/ishantanu/nines/actions) for results on the exact commit under review. Locally passing tests do not establish Linux CI, real Entra/LDAP deployment, public-network ingress, restore, HA or production-scale conformance.
- Run `make intent-eval` and `make intent-workflow-eval` for the credential-free drafting and full workflow contract suites.
  - The [intent evaluation guide](intent-evaluations.md) explains its fixtures, reports and explicitly enabled hosted/local model evaluation.
  - The suite is also exercised by `go test ./...`; fixture results do not establish real model quality.
- See the [local verification record](verification.md) for completed candidate checks and outstanding limits.

## Kubernetes dependency discovery

- Run `make test-dependencies` with Docker, Go and kubectl available.
  - This starts a disposable K3s API and an ephemeral nonroot Nines container on a private test network, installs the shipped namespace-only discovery Role, and verifies real service-account TLS/authentication, candidate generation, tenant/namespace denial, read-only previews, revisioned layout saves and permission revocation.
  - It never loads the host kubeconfig or schedules workloads.
  - All fixture containers, images, tokens and networks are removed afterward. See [service dependencies](service-dependencies.md).

## Dependency evidence and SLO retirement

- Run `make test-topology` for real instrumented HTTP calls, Prometheus sample timestamps, journey steps, tenant boundaries and stale evidence after a paused exporter. The optional [topology playground](service-dependencies.md#run-an-instrumented-example) uses the same example resources.
- The required PostgreSQL suite covers reviewed [SLO retirement](slo-retirement.md), retained history, stale and cross-tenant rejection, concurrent references, failed cleanup across restart, credential rotation, notification cancellation/resolution, deletion receipts and reserved identifiers. `make test-prometheus` and `make test-mimir` verify actual owned configuration removal and evaluator absence. `make test-operator` additionally verifies original-route finalizers during provider outage/credential rotation and independent critical/warning transitions using actual operator-generated rules.
