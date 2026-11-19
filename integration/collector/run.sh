#!/bin/sh
# Owns one disposable database. Never reads a user's provider or Docker volume.
set -eu
umask 077
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
fixture=$(mktemp -d)
name="nines-collector-acceptance-$(openssl rand -hex 6)"
cleanup() {
 docker rm -fv "$name" >/dev/null 2>&1 || true
 rm -rf "$fixture"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
export POSTGRES_PASSWORD
POSTGRES_PASSWORD=$(openssl rand -hex 24)
docker run -d --name "$name" --memory 768m --cpus 2 \
 --publish 127.0.0.1::5432 --env POSTGRES_PASSWORD \
 --env POSTGRES_USER=nines_collector --env POSTGRES_DB=nines_collector \
 --tmpfs /var/lib/postgresql/data:rw,size=512m \
 'postgres:17-alpine@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193' >/dev/null
for attempt in $(seq 1 60); do
 if docker exec "$name" pg_isready -U nines_collector >/dev/null 2>&1; then break; fi
 sleep 1
done
docker exec "$name" pg_isready -U nines_collector >/dev/null
port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' "$name")
export TEST_DATABASE_URL
TEST_DATABASE_URL="postgres://nines_collector:$POSTGRES_PASSWORD@127.0.0.1:$port/nines_collector?sslmode=disable"
cd "$root"
# No user-paid provider paths are involved; all HTTP sources bind to loopback.
contracts_status=0
go test -race -count=1 -timeout 5m -json ./internal/store ./internal/service ./cmd/server > "$fixture/contracts.json" || contracts_status=$?
cp "$fixture/contracts.json" collector-test-results.json
python3 scripts/release/check-tests.py collector-test-results.json --required collector
[ "$contracts_status" -eq 0 ] || exit "$contracts_status"
# Performance results deliberately exclude race-detector overhead.
load_status=0
NINES_COLLECTOR_ACCEPTANCE=1 NINES_COLLECTOR_REPORT="$fixture/load.json" \
 go test -count=1 -timeout 10m -json ./integration/collector > "$fixture/load-tests.json" || load_status=$?
python3 scripts/release/check-tests.py "$fixture/load-tests.json" --required collector-load
[ "$load_status" -eq 0 ] || exit "$load_status"
cp "$fixture/load.json" collector-acceptance.json
printf 'Collector contracts and synthetic workload acceptance passed. Report: collector-acceptance.json\n'
