# Contributing to Nines

The current priority is a small, reproducible public MVP. Fix correctness, first-run failures, unclear contracts and missing verification before adding providers, model features or orchestration layers. See [release scope](docs/public/release.md).

## Setup

Use Go 1.27.3 or later, Node 24, npm, Python 3.9 or later and Docker when running integration tests. Install frontend dependencies with `npm ci --prefix web`, then `make ui build`. Go embeds `internal/httpapi/web`; finish the UI build before compiling Go. Run `go run ./cmd/server -demo` for isolated synthetic data.

## Lint tooling

Install the pinned development tools once. The virtual environment is local and ignored by Git:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
# Ensure $(go env GOPATH)/bin is on PATH.
python3 -m venv .venv-lint
. .venv-lint/bin/activate
python -m pip install -r requirements-lint.txt
make lint
```

`make lint-go` runs the standard golangci-lint checks plus Go/import formatting checks. `make lint-python` runs Ruff correctness/import checks and formatting checks. CI installs the same versions and runs both. Format Go with `golangci-lint fmt` and Python with `ruff format .`; neither lint target edits source. Error paths that affect CLI output, persisted data and test setup must be checked. Read-only stream teardown, post-commit rollback and writes to disconnected HTTP clients may deliberately discard errors when no recovery is possible.

The application server, CLI, client and MCP adapter are Go. Python is limited to source/archive/license tooling, local setup, verification and synthetic playground services. Those scripts use the standard library; Ruff is a development dependency. Native binaries and the final server container do not need a Python interpreter. The Docker build's notice-collection stage and the separate playground helper containers do use Python.

## Checks

```sh
make ui
make lint
make test
make check
make test-integration  # requires TEST_DATABASE_URL for a disposable database
make test-mimir       # starts/removes only its own loopback Mimir fixture
make test-operator    # private Docker API/controller; never uses your kubecontext
make release-check
```

The CI Go job supplies a disposable PostgreSQL service and fails if PostgreSQL checks are skipped. The Mimir job uses a digest-pinned disposable image and exercises collection, rule destinations, query-output identity and independent alert lifecycle timing. The operator job uses an agentless disposable API, real upstream controller, shipped RBAC, actual Nines reconciliation/failover/cleanup and a packaged container smoke check. It does not schedule workload pods. Its runner requires kubectl, curl, base64 and shasum in addition to Docker/Go. Unit tests use local HTTP/identity fixtures; they do not need real provider credentials. Do not point integration tests at production data.

Format Go with `golangci-lint fmt`. Run frontend tests and `npm run build --prefix web` when changing the UI. Add regression tests for calculation, tenant/role boundaries, revision/digest checks and recovery behavior. Keep `nines.dev/v1alpha1` definitions shared between clients and the server. Treat missing data as unknown. Model output must not directly redefine a measurement, lower a target or choose a credential-bearing destination.

Use a least-privilege API key and an explicit tenant for connected CLI/MCP work. Never commit `.env`, API keys, private recordings, database exports or provider responses. Development screenshots and historical lab reports are not public release evidence. The [public source export](docs/public/release.md) uses an allowlist and a file/path scan.

## Pull requests

Describe the user-visible problem, resulting behavior and verification. Keep changes scoped. Call out changes to calculation semantics, YAML fields, role checks or provider routing. A release candidate requires a green clean-checkout CI run; a local passing report alone is not a release.

## Reports

Use the bug template for reproducible failures with version information and redacted configuration. Follow [SECURITY.md](SECURITY.md) for vulnerabilities. GitHub repository protections and private reporting must be configured by the maintainer before public publication.
