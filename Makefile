.PHONY: demo run build server cli mcp operator ui test test-providers test-integration test-mimir test-victoriametrics test-prometheus test-ceph test-operator test-identity test-dependencies test-topology intent-eval intent-workflow-eval template-eval selection-eval check release-check public-source release site site-check lint lint-go lint-python check-api
VERSION ?= dev
RELEASE_TARGETS ?= linux/amd64,linux/arm64,darwin/amd64,darwin/arm64
ui:
	cd web && npm ci --no-audit --no-fund && npm run build
build: server cli mcp operator
server:
	go build -trimpath -o bin/nines-server ./cmd/server
cli:
	go build -trimpath -o bin/nines ./cmd/nines
operator:
	go build -trimpath -o bin/nines-operator ./cmd/operator
mcp:
	go build -trimpath -o bin/nines-mcp ./cmd/mcp
demo:
	go run ./cmd/server -demo
run:
	go run ./cmd/server
test:
	go test -race ./...
	npm test --prefix web
# Synthetic provider/API/MCP contracts only. Paid-provider credentials are never required.
test-providers:
	env -u DD_API_KEY -u DD_APP_KEY -u NEWRELIC_API_KEY -u NEWRELIC_INGEST_KEY -u NEWRELIC_ACCOUNT_ID -u GOOGLE_APPLICATION_CREDENTIALS -u GOOGLE_OAUTH_ACCESS_TOKEN -u MIMIR_TEST_URL -u PROMETHEUS_TEST_URL -u VICTORIAMETRICS_TEST_URL -u TEST_DATABASE_URL NINES_LIVE_NATIVE_READ=0 go test -race -count=1 ./internal/datadog ./internal/newrelic ./internal/dash0 ./internal/googlecloud ./internal/victoriametrics ./internal/providers ./internal/queryspec ./internal/rules ./internal/datasources ./internal/mcpserver ./client ./cmd/nines ./cmd/mcp ./api/openapi ./api/mcp

test-integration:
	@test -n "$$TEST_DATABASE_URL" || (echo 'Set TEST_DATABASE_URL to a dedicated test database'; exit 1)
	go test -race -count=1 -json ./... > database-test-results.json
	python3 scripts/release/check-tests.py database-test-results.json --required database
intent-eval:
	@go run ./integration/intent-eval
intent-workflow-eval:
	@go run ./integration/intent-eval --suite workflow
selection-eval:
	@go run ./integration/intent-eval --suite template-selection
template-eval:
	@go run ./integration/intent-eval --suite templates
test-prometheus:
	sh integration/prometheus/run.sh
test-victoriametrics:
	sh integration/victoriametrics/run.sh
test-mimir:
	sh integration/mimir/run.sh
test-ceph:
	sh integration/ceph/run.sh
test-identity:
	sh integration/identity/run.sh
test-topology:
	sh integration/topology/run.sh
test-dependencies:
	sh integration/dependencies/run.sh
test-operator:
	sh integration/operator/run.sh
check:
	go vet ./...
release-check:
	python3 -m unittest discover -s scripts/release -p 'test_*.py'
	python3 scripts/release/source.py
public-source:
	python3 scripts/release/source.py --out dist/public-source
release:
	python3 scripts/release/build.py --version "$(VERSION)" --targets "$(RELEASE_TARGETS)"

site:
	npm run build:site --prefix web
	python3 scripts/site/check.py
site-check:
	python3 scripts/site/check.py

# Development tools: golangci-lint v2.15.0 and requirements-lint.txt.
lint: lint-go lint-python
lint-go:
	golangci-lint run
	golangci-lint fmt --diff api client cmd examples integration internal provider reviewskills skills
lint-python:
	ruff check .
	ruff format --check .

# Validate the versioned HTTP contract with the pinned development tool.
check-api:
	python3 -m openapi_spec_validator api/openapi/v1alpha1.yaml
	python3 -c 'import json,jsonschema; jsonschema.Draft202012Validator.check_schema(json.load(open("api/mcp/v1alpha1.schema.json")))'
	go test ./api/openapi ./api/mcp
