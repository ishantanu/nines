#!/bin/sh
# Disposable provider acceptance only. No production mounts, devices or credentials.
set -eu
umask 077
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
for tool in docker go openssl curl; do
  command -v "$tool" >/dev/null || { echo "Required tool: $tool" >&2; exit 1; }
done
docker info >/dev/null
fixture=$(mktemp -d "${TMPDIR:-/tmp}/nines-ceph.XXXXXXXX")
fixture=$(CDPATH= cd -- "$fixture" && pwd -P)
name="nines-ceph-$(openssl rand -hex 6)"
ceph_image=quay.io/ceph/ceph@sha256:6bb1c8a42fbc0bf87938946990b65174466997bc11c31eb5a323225a779fd8f9
bao_image=quay.io/openbao/openbao@sha256:d24890f589bc57658a43c18faab2afc4fbedaadad3806d5a638e1adaed2dff8d
cleanup() {
  docker rm -f "$name-bao" "$name" >/dev/null 2>&1 || true
  rm -rf "$fixture"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
cp "$root/integration/ceph/bootstrap.sh" "$fixture/bootstrap.sh"
openssl rand -hex 12 > "$fixture/access-key"
openssl rand -hex 30 > "$fixture/secret-key"
openssl rand -hex 30 > "$fixture/bao-token"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -subj /CN=nines-ceph-fixture \
  -addext 'subjectAltName=IP:127.0.0.1,DNS:localhost' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -keyout "$fixture/server.key" -out "$fixture/ca.pem" >/dev/null 2>&1
cat "$fixture/server.key" "$fixture/ca.pem" > "$fixture/server.pem"
# The key-service listener shares the fixture's private network namespace.
# Only RGW's TLS port is published, on a random host loopback port.
docker run -d --name "$name" --hostname ceph-fixture \
  --memory 1536m --cpus 2 --pids-limit 512 \
  -p 127.0.0.1::9443 \
  --mount "type=bind,src=$fixture,dst=/fixture,readonly" \
  --entrypoint /bin/bash "$ceph_image" /fixture/bootstrap.sh >/dev/null
export BAO_DEV_ROOT_TOKEN_ID BAO_TOKEN BAO_ADDR SKIP_SETCAP
BAO_DEV_ROOT_TOKEN_ID=$(cat "$fixture/bao-token")
BAO_TOKEN=$BAO_DEV_ROOT_TOKEN_ID
BAO_ADDR=http://127.0.0.1:8200
SKIP_SETCAP=true
docker run -d --name "$name-bao" --network "container:$name" \
  --memory 256m --cpus 1 --pids-limit 128 \
  -e BAO_DEV_ROOT_TOKEN_ID -e BAO_TOKEN -e BAO_ADDR -e SKIP_SETCAP \
  "$bao_image" server -dev -dev-listen-address=127.0.0.1:8200 >/dev/null
ready=false
for attempt in $(seq 1 30); do
  if docker exec "$name-bao" bao status >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
$ready || { echo "OpenBao fixture did not start" >&2; exit 1; }
docker exec "$name-bao" bao secrets enable transit >/dev/null
address=$(docker port "$name" 9443/tcp)
export NINES_CEPH_ENDPOINT NINES_CEPH_CONTAINER SSL_CERT_FILE
NINES_CEPH_ENDPOINT="https://$address"
NINES_CEPH_CONTAINER=1
SSL_CERT_FILE="$fixture/ca.pem"
ready=false
for attempt in $(seq 1 150); do
  if curl --fail --silent --cacert "$SSL_CERT_FILE" --max-time 2 "$NINES_CEPH_ENDPOINT/" >/dev/null 2>&1; then ready=true; break; fi
  running=$(docker inspect --format '{{.State.Running}}' "$name")
  if [ "$running" != true ]; then echo "Ceph fixture exited during bootstrap" >&2; exit 1; fi
  sleep 1
done
$ready || { echo "Ceph fixture readiness timed out" >&2; exit 1; }
# Bind only to generated fixture credentials, never inherited AWS profiles/roles.
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN AWS_SECURITY_TOKEN AWS_ROLE_ARN AWS_WEB_IDENTITY_TOKEN_FILE AWS_CONTAINER_CREDENTIALS_RELATIVE_URI AWS_CONTAINER_CREDENTIALS_FULL_URI AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_EC2_METADATA_DISABLED AWS_CONFIG_FILE AWS_SHARED_CREDENTIALS_FILE
AWS_ACCESS_KEY_ID=$(cat "$fixture/access-key")
AWS_SECRET_ACCESS_KEY=$(cat "$fixture/secret-key")
AWS_EC2_METADATA_DISABLED=true
AWS_CONFIG_FILE=/dev/null
AWS_SHARED_CREDENTIALS_FILE=/dev/null
cd "$root"
echo 'Running synthetic Ceph RGW acceptance with TLS, OpenBao and disposable storage.'
go run ./integration/ceph
