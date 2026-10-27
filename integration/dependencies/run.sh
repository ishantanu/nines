#!/bin/sh
# Disposable Kubernetes API only: never loads the caller's kubeconfig or cluster.
set -eu
for tool in docker go kubectl curl base64 openssl; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool"; exit 1; }
done
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=$(mktemp -d)
name="nines-discovery-acceptance-$$"
network="$name-net"
server="$name-api"
runtime="$name-runtime"
cleanup() {
  result=$?
  if [ "$result" != 0 ]; then
    docker logs --tail 60 "$server" >&2 2>/dev/null || true
    docker logs --tail 60 "$runtime" >&2 2>/dev/null || true
  fi
  docker rm -f "$runtime" "$server" >/dev/null 2>&1 || true
  docker image rm "$name-image" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM
umask 077
docker network create "$network" >/dev/null
docker run -d --name "$server" --label nines.acceptance=dependencies \
  --network "$network" --network-alias api --memory 1536m --cpus 2 \
  --tmpfs /var/lib/rancher/k3s:rw,size=512m \
  rancher/k3s:v1.37.1-k3s1@sha256:ca7f37d993d82ef0dcdcfecb2e0e2618ea541dbaffc620c8cedebe01a82acd0d \
  server --disable-agent --disable traefik,servicelb,coredns,metrics-server,local-storage \
  --disable-network-policy --disable-cloud-controller --tls-san api \
  --write-kubeconfig-mode 0600 >/dev/null
ready=false
for _ in $(seq 1 120); do
  if docker exec "$server" kubectl get --raw=/readyz >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
[ "$ready" = true ] || { echo 'Kubernetes fixture did not become ready'; exit 1; }
for namespace in nines commerce; do
  docker exec "$server" kubectl create namespace "$namespace" >/dev/null
done
docker exec -i "$server" kubectl apply -f - < "$root/deploy/kubernetes/dependency-discovery-rbac.yaml" >/dev/null
docker exec -i "$server" kubectl apply -f - < "$root/internal/kubediscovery/testdata/resources.yaml" >/dev/null
mkdir -p "$work/image" "$work/serviceaccount" "$work/config"
docker cp "$server:/etc/rancher/k3s/k3s.yaml" "$work/kubeconfig" >/dev/null
kubectl --kubeconfig="$work/kubeconfig" config view --raw -o 'jsonpath={.clusters[0].cluster.certificate-authority-data}' | base64 --decode > "$work/serviceaccount/ca.crt"
rm "$work/kubeconfig"
docker exec "$server" kubectl -n nines create token nines-discovery --duration=15m > "$work/serviceaccount/token"
export NINES_DISCOVERY_TEST_OWNER=$(openssl rand -hex 24)
export NINES_DISCOVERY_TEST_VIEWER=$(openssl rand -hex 24)
export NINES_DISCOVERY_TEST_OTHER=$(openssl rand -hex 24)
cat > "$work/config/tenants.yaml" <<'CONFIG'
tenants:
  - id: commerce
    name: Commerce
    access:
      - {tokenEnv: NINES_DISCOVERY_TEST_OWNER, role: owner}
      - {tokenEnv: NINES_DISCOVERY_TEST_VIEWER, role: viewer}
    source: {kind: manual}
    kubernetesDiscovery:
      namespaces: [commerce]
  - id: other
    name: Other
    access:
      - {tokenEnv: NINES_DISCOVERY_TEST_OTHER, role: owner}
    source: {kind: manual}
CONFIG
arch=$(docker version --format '{{.Server.Arch}}')
case "$arch" in amd64|arm64) ;; *) echo 'Unsupported fixture architecture'; exit 1 ;; esac
cd "$root"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=false -o "$work/image/nines-server" ./cmd/server
chmod 555 "$work/image/nines-server"
cat > "$work/image/Dockerfile" <<'IMAGE'
FROM scratch
COPY nines-server /nines-server
USER 65532:65532
ENTRYPOINT ["/nines-server"]
IMAGE
docker build --quiet --tag "$name-image" "$work/image" >/dev/null
chmod 755 "$work/config" "$work/serviceaccount"
chmod 444 "$work/config/tenants.yaml" "$work/serviceaccount/ca.crt" "$work/serviceaccount/token"
docker run -d --name "$runtime" --label nines.acceptance=dependencies \
  --network "$network" --publish 127.0.0.1::8080 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --memory 256m \
  --env KUBERNETES_SERVICE_HOST=api --env KUBERNETES_SERVICE_PORT_HTTPS=6443 \
  --env NINES_DISCOVERY_TEST_OWNER --env NINES_DISCOVERY_TEST_VIEWER --env NINES_DISCOVERY_TEST_OTHER \
  --mount "type=bind,src=$work/config,dst=/etc/nines,readonly" \
  --mount "type=bind,src=$work/serviceaccount,dst=/var/run/secrets/kubernetes.io/serviceaccount,readonly" \
  "$name-image" -memory -tenants-file /etc/nines/tenants.yaml -addr 0.0.0.0:8080 >/dev/null
port=$(docker port "$runtime" 8080/tcp | sed 's/.*://')
export NINES_DISCOVERY_TEST_URL="http://127.0.0.1:$port"
export NINES_DISCOVERY_TEST_CONTAINER="$server"
ready=false
for _ in $(seq 1 60); do
  if curl --fail --silent "$NINES_DISCOVERY_TEST_URL/healthz" >/dev/null; then ready=true; break; fi
  sleep 1
done
[ "$ready" = true ] || { echo 'Nines fixture did not become ready'; exit 1; }
go test -race -count=1 -v ./integration/dependencies
docker stop --time 10 "$runtime" >/dev/null
[ "$(docker inspect --format '{{.State.ExitCode}}' "$runtime")" = 0 ] || { echo 'Nines did not terminate cleanly'; exit 1; }
echo 'Kubernetes discovery acceptance passed: verified TLS, shipped namespace RBAC, tenant isolation, reviewed revisions and permission revocation.'
