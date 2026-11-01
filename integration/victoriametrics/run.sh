#!/bin/sh
# Disposable, loopback-only VictoriaMetrics; no host mounts or retained volumes.
set -eu
integration_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
platform_dir=$(CDPATH= cd -- "$integration_dir/../.." && pwd)
container=''
cleanup() {
  if [ -n "$container" ]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT HUP INT TERM
container=$(docker run -d --name "nines-victoriametrics-acceptance-$$" \
  --label nines.acceptance=victoriametrics --publish 127.0.0.1::8428 \
  --memory 256m --cpus 1 --tmpfs /storage:rw,size=134217728 \
  'victoriametrics/victoria-metrics:v1.126.0@sha256:26dfc4ab90cf6fedc8115269adc22d300bcd5eab82ea1ebed3c8e22b49886ce1' \
  -storageDataPath=/storage -search.cacheTimestampOffset=0)
port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "8428/tcp") 0).HostPort}}' "$container")
cd "$platform_dir"
VICTORIAMETRICS_TEST_URL="http://127.0.0.1:$port" \
  go test -race -count=1 -v ./internal/victoriametrics -run '^TestRealVictoriaMetrics$'
