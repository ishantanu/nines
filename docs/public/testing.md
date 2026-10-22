# Testing

Install the pinned development linters as described in [Contributing](../../CONTRIBUTING.md#lint-tooling), then run `make lint`. CI has a dedicated Go/Python lint job that also rejects formatting drift.

Run `make ui build test check release-check` from a clean checkout. Frontend compilation must finish before Go compilation because the server embeds its output. `go test -race` exercises the CLI, calculations, tenant/API boundaries, synthetic OIDC/LDAP, transactional behavior, rule generation and bounded workflow fixtures.

Database-dependent cases require `TEST_DATABASE_URL`, pointed at a disposable PostgreSQL database whose user may create schemas. Tests create unique schemas and remove only those. CI provides PostgreSQL and rejects skipped database cases. Do not substitute a production connection.

`make test-mimir` starts only a uniquely named disposable loopback Mimir container and removes it on exit. It tests synthetic ingestion, tenant isolation, collection and ruler destinations. The runner refuses a non-loopback `MIMIR_TEST_URL`. Tests do not contact a real identity provider, hosted model or Kubernetes cluster unless a separately reviewed harness explicitly does so.

The [Compose playground](deployment/playground.md) defaults to Prometheus. CI runs its complete management/recording-rule/alert lifecycle in a two-mode matrix: Prometheus-only and the optional dual-backend Mimir profile. The same setup and verifier select one or both workspaces from trusted Compose configuration.

The first public release also requires clean-checkout CI, native CLI/browser identity verification, demo first-run verification, a reviewed dependency/license inventory, current vulnerability scans, container startup and artifact installation checks. Historical local test reports and screenshots are not substitutes for these checks.

CI configuration is committed locally before repository creation. It has not run on GitHub until an actual run URL and commit are recorded. Locally passing tests do not establish Linux CI, real Entra/LDAP deployment, public-network ingress, restore, HA or production-scale conformance.

See the [local verification record](verification.md) for completed candidate checks and outstanding limits.
