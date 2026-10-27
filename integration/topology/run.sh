#!/bin/sh
# Instrumented HTTP requests and a real Prometheus, all in disposable containers.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=$(mktemp -d)
name="nines-topology-acceptance-$$"
network="$name-net"
cleanup(){
 result=$?
 if [ "$result" != 0 ]; then docker logs --tail 40 "$name-prometheus" >&2 2>/dev/null || true; fi
 docker rm -f "$name-traffic" "$name-gateway" "$name-checkout" "$name-payments" "$name-prometheus" >/dev/null 2>&1 || true
 docker network rm "$network" >/dev/null 2>&1 || true
 docker image rm "$name-image" >/dev/null 2>&1 || true
 rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM
cd "$root"
# Only the bounded, stdlib example is built, not a server/database image.
docker build --quiet -f examples/service-graph/Dockerfile -t "$name-image" . >/dev/null
docker network create "$network" >/dev/null
for service in gateway checkout payments; do
 case "$service" in
 gateway) docker run -d --name "$name-gateway" --label nines.acceptance=topology --network "$network" --network-alias graph-gateway --read-only --cap-drop ALL --security-opt no-new-privileges "$name-image" -name gateway -dependency checkout -next http://graph-checkout:8000/request >/dev/null ;;
 checkout) docker run -d --name "$name-checkout" --label nines.acceptance=topology --network "$network" --network-alias graph-checkout --read-only --cap-drop ALL --security-opt no-new-privileges "$name-image" -name checkout -dependency payments -next http://graph-payments:8000/request >/dev/null ;;
 payments) docker run -d --name "$name-payments" --label nines.acceptance=topology --network "$network" --network-alias graph-payments --read-only --cap-drop ALL --security-opt no-new-privileges "$name-image" -name payments >/dev/null ;;
 esac
done
docker run -d --name "$name-traffic" --label nines.acceptance=topology --network "$network" --read-only --cap-drop ALL "$name-image" -generate http://graph-gateway:8000/request >/dev/null
cat > "$work/prometheus.yaml" <<'CONFIG'
global:
  scrape_interval: 5s
scrape_configs:
  - job_name: playground-service-graph
    static_configs:
      - targets: [graph-gateway:8000, graph-checkout:8000, graph-payments:8000]
CONFIG
chmod 644 "$work/prometheus.yaml"
docker run -d --name "$name-prometheus" --label nines.acceptance=topology \
 --network "$network" --publish 127.0.0.1::9090 \
 --mount "type=bind,src=$work/prometheus.yaml,dst=/etc/prometheus/prometheus.yml,readonly" \
 --tmpfs /prometheus:rw,size=67108864,uid=65534,gid=65534 \
 prom/prometheus:v3.5.1@sha256:38c3b05c3bc744ff1b0b7b4eb82196026442845e62a1e2073795565da506d7a2 \
 --config.file=/etc/prometheus/prometheus.yml >/dev/null
port=$(docker port "$name-prometheus" 9090/tcp | sed 's/.*://')
export NINES_TOPOLOGY_TEST_URL="http://127.0.0.1:$port"
export NINES_TOPOLOGY_TEST_CHECKOUT="$name-checkout"
if go test -race -count=1 -json ./integration/topology > "$work/results.json"; then
 python3 scripts/release/check-tests.py "$work/results.json" --required topology
else
 cat "$work/results.json"
 exit 1
fi
