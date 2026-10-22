#!/bin/sh
# Starts and removes only its own disposable loopback Mimir container.
set -eu

integration_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
platform_dir=$(CDPATH= cd -- "$integration_dir/../.." && pwd)
image='grafana/mimir:3.2.2@sha256:04068a0a838b6efe9449a04c1d6acd74c7438ac6073dfcbb99c29892647525e3'
container=$(docker run -d \
  --name "nines-mimir-integration-$$" \
  --publish 127.0.0.1::9009 \
  --mount "type=bind,src=$integration_dir/mimir.yaml,dst=/etc/mimir/nines-test.yaml,readonly" \
  --tmpfs /tmp:rw,size=268435456 \
  "$image" \
  --config.file=/etc/mimir/nines-test.yaml \
  --http.prometheus-http-prefix=/prometheus \
  --ingest-storage.enabled=false)
cleanup() { docker rm -f "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT HUP INT TERM
port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "9009/tcp") 0).HostPort}}' "$container")
cd "$platform_dir"
MIMIR_TEST_URL="http://127.0.0.1:$port" go test -race -count=1 -json ./integration/mimir ./internal/rules > mimir-test-results.json
python3 scripts/release/check-tests.py mimir-test-results.json --required mimir
