#!/bin/sh
# Upgrade a database created by every published release with the current
# build, and check that definitions, revisions, samples, journeys and status
# survive. Each release seeds its own database with its own CLI, so the test
# uses only interfaces that release shipped.
#
# Requires Docker, Go, curl and python3. Downloads the published release
# archives from GitHub and verifies them against SHA256SUMS.
set -eu

root=$(cd "$(dirname "$0")/../.." && pwd)
# Every published release. v0.1.0-alpha.1 and v0.1.0-alpha.2 were never
# published (their releases are still drafts), so no database created by them
# exists to upgrade. Add each new release here when it is published.
versions=${NINES_UPGRADE_VERSIONS:-"v0.1.0-alpha.3 v0.1.0-alpha.4"}
repository=${NINES_UPGRADE_REPOSITORY:-ishantanu/nines}
work=$(mktemp -d)
container="nines-upgrade-$$"
server=""
token="upgrade-test-token-0123456789abcdef"

cleanup() {
  [ -n "$server" ] && kill "$server" 2>/dev/null || true
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "unsupported OS"; exit 1 ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; arm64 | aarch64) arch=arm64 ;; *) echo "unsupported CPU"; exit 1 ;; esac
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }

docker run -d --rm --name "$container" -e POSTGRES_PASSWORD=upgrade-test -p 127.0.0.1::5432 postgres:17-alpine >/dev/null
port=$(docker port "$container" 5432/tcp | head -1 | sed 's/.*://')
for _ in $(seq 1 60); do
  docker exec "$container" pg_isready -U postgres >/dev/null 2>&1 && break
  sleep 1
done
psql() { docker exec "$container" psql -U postgres -v ON_ERROR_STOP=1 -qtA "$@"; }

(cd "$root" && go build -o "$work/current-server" ./cmd/server)

cat > "$work/slo.yaml" <<'EOF'
apiVersion: nines.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: api-availability
  project: production
spec:
  displayName: API availability
  service: gateway
  target: 0.999
  windowSeconds: 86400
  stepSeconds: 60
  goodQuery: sum(increase(http_requests_total{code!~"5.."}[1m]))
  totalQuery: sum(increase(http_requests_total[1m]))
EOF
cat > "$work/journey.yaml" <<'EOF'
apiVersion: nines.dev/v1alpha1
kind: CriticalUserJourney
metadata:
  name: upgrade-check
  project: production
spec:
  displayName: Upgrade check
  description: Journey seeded by the upgrade test.
  owner: team-sre
  criticality: high
  slos:
    - id: api-availability
      dimension: availability
      required: true
  steps:
    - id: request
      name: Send a request
      actor: user
      sloRefs: [api-availability]
EOF
python3 - "$work/samples.json" <<'EOF'
import json, sys, time
end = int(time.time()) // 60 * 60 - 120
points = [{"time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(end - 60 * i)), "good": 9990 + i % 3, "total": 10000} for i in range(120)]
json.dump({"samples": points}, open(sys.argv[1], "w"))
EOF

start() { # binary database port
  DATABASE_URL="postgres://postgres:upgrade-test@127.0.0.1:$port/$2?sslmode=disable" API_TOKEN="$token" "$1" -addr "127.0.0.1:$3" >"$work/server.log" 2>&1 &
  server=$!
  for _ in $(seq 1 60); do
    curl -fsS "http://127.0.0.1:$3/healthz" >/dev/null 2>&1 && return 0
    kill -0 "$server" 2>/dev/null || break
    sleep 1
  done
  cat "$work/server.log"
  echo "server did not become ready"
  exit 1
}
stop() { kill "$server"; wait "$server" 2>/dev/null || true; server=""; }
get() { curl -fsS -H "Authorization: Bearer $token" "http://127.0.0.1:$1$2"; }
snapshot() { # port -> one line describing what must survive
  {
    get "$1" /api/slos/api-availability/status
    echo
    get "$1" /api/slos/api-availability/samples
    echo
    get "$1" /api/journeys
    echo
  } | python3 -c '
import json, sys
status, samples, journeys = (json.loads(line) for line in sys.stdin if line.strip())
names = sorted(j["resource"]["manifest"]["metadata"]["name"] for j in journeys)
print(json.dumps({"reliability": status.get("reliability"), "coverage": status.get("coverage"), "samples": len(samples), "journeys": names}, sort_keys=True))'
}

results=""
n=0
for version in $versions; do
  n=$((n + 1))
  archive="nines_${version}_${os}_${arch}.tar.gz"
  base="https://github.com/$repository/releases/download/$version"
  mkdir -p "$work/$version"
  curl -fsSL "$base/$archive" -o "$work/$version/$archive"
  curl -fsSL "$base/SHA256SUMS" -o "$work/$version/SHA256SUMS"
  want=$(grep " $archive\$" "$work/$version/SHA256SUMS" | cut -d' ' -f1)
  [ -n "$want" ] && [ "$want" = "$(sha256 "$work/$version/$archive")" ] || { echo "$archive does not match SHA256SUMS"; exit 1; }
  tar -xzf "$work/$version/$archive" -C "$work/$version"

  database="upgrade_$n"
  psql -c "CREATE DATABASE $database"
  old_port=$((18400 + n))
  start "$work/$version/nines-server" "$database" "$old_port"
  cli() { NINES_URL="http://127.0.0.1:$old_port" NINES_API_KEY="$token" "$work/$version/nines" "$@"; }
  cli apply -f "$work/slo.yaml" --expected-revision 0 --agent >/dev/null
  curl -fsS -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    --data @"$work/samples.json" "http://127.0.0.1:$old_port/api/slos/api-availability/samples" >/dev/null
  cli apply -f "$work/journey.yaml" --expected-revision 0 --agent >/dev/null
  before=$(snapshot "$old_port")
  stop

  new_port=$((18500 + n))
  # Start twice: the first start upgrades, the second must find nothing to do.
  for round in first second; do
    start "$work/current-server" "$database" "$new_port"
    after=$(snapshot "$new_port")
    stop
    if [ "$before" != "$after" ]; then
      echo "$version upgrade ($round start) changed data"
      echo "before: $before"
      echo "after:  $after"
      exit 1
    fi
  done
  recorded=$(psql -d "$database" -c "SELECT count(*) FROM nines_schema_migrations WHERE component='store'")
  [ "$recorded" = 1 ] || { echo "$version: store migrations recorded $recorded times"; exit 1; }
  results="$results\"$version\": $after, "
done
printf '{"status": "passed", "os": "%s", "arch": "%s", "upgraded": {%s}}\n' "$os" "$arch" "${results%, }"
