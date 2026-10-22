.PHONY: demo run build server cli mcp ui test test-integration test-mimir intent-eval intent-workflow-eval check release-check public-source release site site-check lint lint-go lint-python
VERSION ?= dev
RELEASE_TARGETS ?= linux/amd64,linux/arm64,darwin/amd64,darwin/arm64
ui:
	cd web && npm ci --no-audit --no-fund && npm run build
build: server cli mcp
server:
	go build -trimpath -o bin/nines-server ./cmd/server
cli:
	go build -trimpath -o bin/nines ./cmd/nines
mcp:
	go build -trimpath -o bin/nines-mcp ./cmd/mcp
demo:
	go run ./cmd/server -demo
run:
	go run ./cmd/server
test:
	go test -race ./...
	npm test --prefix web
test-integration:
	@test -n "$$TEST_DATABASE_URL" || (echo 'Set TEST_DATABASE_URL to a dedicated test database'; exit 1)
	go test -race -count=1 -json ./... > database-test-results.json
	python3 scripts/release/check-tests.py database-test-results.json --required database
intent-eval:
	@go run ./integration/intent-eval
intent-workflow-eval:
	@go run ./integration/intent-eval --suite workflow
test-mimir:
	sh integration/mimir/run.sh
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
	golangci-lint fmt --diff
lint-python:
	ruff check .
	ruff format --check .
