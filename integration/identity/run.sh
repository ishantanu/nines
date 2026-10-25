#!/bin/sh
# Owns only two disposable containers; never loads a caller's Nines/IdP profile.
set -eu
umask 077
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
for tool in docker go openssl python3; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 1; }
done
fixture=$(mktemp -d)
fixture=$(CDPATH= cd -- "$fixture" && pwd -P)
cd "$root"
: > identity-test-results.json
name="nines-identity-acceptance-$(openssl rand -hex 6)"
cleanup() {
  docker rm -fv "$name-ldap" "$name-database" >/dev/null 2>&1 || true
  docker image rm "$name-image" >/dev/null 2>&1 || true
  rm -rf "$fixture"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
for secret in admin-password reader-password user-password; do
  openssl rand -hex 24 | tr -d '\n' > "$fixture/$secret"
done
openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -subj /CN=nines-identity-fixture \
  -addext 'subjectAltName=IP:127.0.0.1,DNS:localhost' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -keyout "$fixture/server-key.pem" -out "$fixture/ca.pem" >/dev/null 2>&1
docker build --quiet --tag "$name-image" "$root/integration/identity" > "$fixture/image-id"
# Bound provider descriptors as well as memory for predictable fixture startup.
docker run -d --name "$name-ldap" --memory 384m --cpus 1 --pids-limit 128 --ulimit nofile=4096:4096 \
  --read-only --tmpfs /run:rw,size=16m --tmpfs /data:rw,size=96m \
  --publish 127.0.0.1::1389 --publish 127.0.0.1::1636 \
  --mount "type=bind,src=$fixture,dst=/fixture,readonly" "$name-image" >/dev/null
export POSTGRES_PASSWORD
POSTGRES_PASSWORD=$(openssl rand -hex 24)
docker run -d --name "$name-database" --memory 512m --cpus 1 \
  --publish 127.0.0.1::5432 --env POSTGRES_PASSWORD \
  --env POSTGRES_USER=nines_identity --env POSTGRES_DB=nines_identity postgres:17-alpine >/dev/null
ready=false
for attempt in $(seq 1 60); do
  for service in "$name-ldap" "$name-database"; do
    running=$(docker inspect --format '{{.State.Running}}' "$service")
    if [ "$running" != true ]; then
      docker inspect --format 'Fixture stopped: {{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}}' "$service" >&2
      exit 1
    fi
  done
  if docker exec "$name-database" pg_isready -U nines_identity >/dev/null 2>&1 \
    && docker exec --env LDAPTLS_CACERT=/fixture/ca.pem "$name-ldap" \
      ldapsearch -x -ZZ -H ldap://127.0.0.1:1389 -D cn=reader,dc=example,dc=test \
      -y /fixture/reader-password -b dc=example,dc=test -s base dn >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
$ready || { echo 'Disposable identity providers did not become ready' >&2; exit 1; }
ldap_port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "1636/tcp") 0).HostPort}}' "$name-ldap")
starttls_port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "1389/tcp") 0).HostPort}}' "$name-ldap")
database_port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' "$name-database")
export NINES_IDENTITY_ACCEPTANCE NINES_IDENTITY_LDAP_URL NINES_IDENTITY_STARTTLS_URL NINES_IDENTITY_FIXTURE_DIR NINES_IDENTITY_DATABASE_URL NINES_IDENTITY_CLI
NINES_IDENTITY_ACCEPTANCE=1
NINES_IDENTITY_LDAP_URL="ldaps://127.0.0.1:$ldap_port"
NINES_IDENTITY_STARTTLS_URL="ldap://127.0.0.1:$starttls_port"
NINES_IDENTITY_FIXTURE_DIR="$fixture"
NINES_IDENTITY_DATABASE_URL="postgres://nines_identity:$POSTGRES_PASSWORD@127.0.0.1:$database_port/nines_identity?sslmode=disable"
NINES_IDENTITY_CLI="$fixture/nines"
cd "$root"
go build -trimpath -o "$NINES_IDENTITY_CLI" ./cmd/nines
# The controlled-provider rejection/permission matrix complements the real
# upstream fixture. PostgreSQL tests must use this same disposable database.
TEST_DATABASE_URL="$NINES_IDENTITY_DATABASE_URL" go test -race -count=1 ./internal/identity ./internal/httpapi ./cmd/nines
result=0
go test -race -count=1 -timeout 5m -json ./integration/identity > "$fixture/results.json" || result=$?
cp "$fixture/results.json" identity-test-results.json
python3 scripts/release/check-tests.py identity-test-results.json --required identity
[ "$result" = 0 ]
docker exec "$name-ldap" slapd -VV 2>&1 | head -n 2
echo 'Identity acceptance passed with disposable PostgreSQL, signed OIDC and real OpenLDAP TLS.'
