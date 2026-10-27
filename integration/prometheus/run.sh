#!/bin/sh
# Uses only a disposable Prometheus and a private shared rule directory.
set -eu
integration_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
platform_dir=$(CDPATH= cd -- "$integration_dir/../.." && pwd)
fixture_dir=$(mktemp -d)
container=''
cleanup() {
  if [ -n "$container" ]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
  rm -rf "$fixture_dir"
}
trap cleanup EXIT HUP INT TERM
mkdir "$fixture_dir/rules"
chmod 755 "$fixture_dir" "$fixture_dir/rules"
cat > "$fixture_dir/prometheus.yaml" <<'CONFIG'
global:
  scrape_interval: 1s
  evaluation_interval: 1s
rule_files:
  - /rules/*.yaml
scrape_configs: []
CONFIG
chmod 644 "$fixture_dir/prometheus.yaml"
container=$(docker run -d --name "nines-prometheus-acceptance-$$" \
  --label nines.acceptance=prometheus --publish 127.0.0.1::9090 \
  --mount "type=bind,src=$fixture_dir/prometheus.yaml,dst=/etc/prometheus/prometheus.yml,readonly" \
  --mount "type=bind,src=$fixture_dir/rules,dst=/rules,readonly" \
  --tmpfs /prometheus:rw,size=67108864,uid=65534,gid=65534 \
  'prom/prometheus:v3.5.1@sha256:38c3b05c3bc744ff1b0b7b4eb82196026442845e62a1e2073795565da506d7a2' \
  --config.file=/etc/prometheus/prometheus.yml --web.enable-lifecycle)
port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "9090/tcp") 0).HostPort}}' "$container")
cd "$platform_dir"
PROMETHEUS_TEST_URL="http://127.0.0.1:$port" PROMETHEUS_TEST_RULE_DIRECTORY="$fixture_dir/rules" \
  go test -race -count=1 -json ./internal/rules -run '^TestRealPrometheus(ReviewedActivation|LifecycleAndMinEvents|Retirement)$' > "$fixture_dir/results.json"
python3 scripts/release/check-tests.py "$fixture_dir/results.json" --required prometheus
cp "$fixture_dir/results.json" prometheus-test-results.json
