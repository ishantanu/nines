#!/bin/sh
# No host kubeconfig or existing Kubernetes cluster is used. No nodes or pods.
set -eu
for tool in docker go kubectl curl base64 shasum python3; do
  command -v "$tool" >/dev/null || { echo "Missing fixture prerequisite: $tool"; exit 1; }
done
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=$(mktemp -d)
name="nines-operator-acceptance-$$"
network="$name-net"
server="$name-api"
controller="$name-controller"
mimir="$name-mimir"
runtime="$name-runtime"
prometheus="$name-prometheus"
cleanup() {
  result=$?
  if [ "$result" != 0 ]; then
    docker logs --tail 80 "$runtime" > operator-runtime-diagnostics.log 2>&1 || true
    docker exec "$server" kubectl -n team-commerce get slo,cuj,lease -o json > operator-runtime-debug-test-results.json 2>/dev/null || true
  fi
  docker rm -f "$runtime" "$controller" "$mimir" "$prometheus" "$server" >/dev/null 2>&1 || true
  docker image rm "$name-runtime-image" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM
umask 077
# Immutable upstream releases. Never apply their CRDs to the caller's cluster.
curl --fail --silent --show-error --location --max-time 120 \
  https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.94.1/stripped-down-crds.yaml > "$work/crds.yaml"
printf '%s  %s\n' e35c128e5ae8607e04fe92e7d6aae28c7fc9cf24262549a84904fa0042ac21ae "$work/crds.yaml" | shasum -a 256 -c -
docker network create "$network" >/dev/null
docker run -d --name "$server" --label nines.acceptance=operator \
  --publish 127.0.0.1::6443 \
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
[ "$ready" = true ] || { echo 'Disposable Kubernetes API did not become ready'; exit 1; }
docker exec -i "$server" kubectl apply --server-side -f - < "$work/crds.yaml" >/dev/null
docker exec "$server" kubectl wait --for=condition=Established --timeout=60s crd --all >/dev/null
docker cp "$server:/etc/rancher/k3s/k3s.yaml" "$work/kubeconfig" >/dev/null
# kubectl config view reads only this private file; it contacts no cluster.
for item in ca cert key; do
  case "$item" in
    ca) field='{.clusters[0].cluster.certificate-authority-data}' ;;
    cert) field='{.users[0].user.client-certificate-data}' ;;
    key) field='{.users[0].user.client-key-data}' ;;
  esac
  kubectl --kubeconfig="$work/kubeconfig" config view --raw -o "jsonpath=$field" | base64 --decode > "$work/$item.pem"
done
rm "$work/kubeconfig"
for namespace in nines-prometheus nines-rules nines-unselected; do
  docker exec "$server" kubectl create namespace "$namespace" >/dev/null
done
docker run -d --name "$controller" --network "$network" --user 0:0 \
  --read-only --tmpfs /tmp:rw,size=64m --memory 512m \
  --mount "type=bind,src=$work,dst=/etc/nines-test,readonly" \
  quay.io/prometheus-operator/prometheus-operator:v0.94.1@sha256:7c88d4e7bae63bd8d0f8da986054337c23b4472fa8d3c6817f5bc4b8407a2d6a \
  --apiserver=https://api:6443 --ca-file=/etc/nines-test/ca.pem \
  --cert-file=/etc/nines-test/cert.pem --key-file=/etc/nines-test/key.pem \
  --namespaces=nines-prometheus,nines-rules,nines-unselected,team-commerce \
  --disable-unmanaged-prometheus-configuration >/dev/null
# Install the shipped namespace Role/ServiceAccount and generated domain CRDs.
docker exec -i "$server" kubectl apply -f - < "$root/deploy/operator/crds.yaml" >/dev/null
docker exec "$server" kubectl wait --for=condition=Established --timeout=60s crd --all >/dev/null
docker exec -i "$server" kubectl apply -f - < "$root/deploy/operator/operator.yaml" >/dev/null
docker exec "$server" kubectl -n team-commerce scale deployment nines-operator --replicas=0 >/dev/null
docker exec "$server" kubectl -n team-commerce create token nines-operator --duration=15m > "$work/token"
api_port=$(docker port "$server" 6443/tcp | sed 's/.*://')
docker run -d --name "$mimir" --label nines.acceptance=operator --network "$network" --publish 127.0.0.1::9009 \
  --mount "type=bind,src=$root/integration/mimir/mimir.yaml,dst=/etc/mimir/test.yaml,readonly" \
  --tmpfs /tmp:rw,size=256m \
  grafana/mimir:3.2.2@sha256:04068a0a838b6efe9449a04c1d6acd74c7438ac6073dfcbb99c29892647525e3 \
  --config.file=/etc/mimir/test.yaml --http.prometheus-http-prefix=/prometheus \
  --ingest-storage.enabled=false >/dev/null
mimir_port=$(docker port "$mimir" 9009/tcp | sed 's/.*://')
mkdir -p "$work/prometheus-rules"
chmod 755 "$work/prometheus-rules"
cat > "$work/prometheus.yaml" <<'PROMETHEUS'
global:
  scrape_interval: 1s
  evaluation_interval: 1s
rule_files: [/rules/*.yaml]
scrape_configs: []
PROMETHEUS
chmod 644 "$work/prometheus.yaml"
docker run -d --name "$prometheus" --label nines.acceptance=operator \
  --publish 127.0.0.1::9090 --network "$network" \
  --mount "type=bind,src=$work/prometheus.yaml,dst=/etc/prometheus/prometheus.yml,readonly" \
  --mount "type=bind,src=$work/prometheus-rules,dst=/rules,readonly" \
  --tmpfs /prometheus:rw,size=67108864,uid=65534,gid=65534 \
  prom/prometheus:v3.5.1@sha256:38c3b05c3bc744ff1b0b7b4eb82196026442845e62a1e2073795565da506d7a2 \
  --config.file=/etc/prometheus/prometheus.yml --web.enable-lifecycle >/dev/null
prometheus_port=$(docker port "$prometheus" 9090/tcp | sed 's/.*://')
export NINES_OPERATOR_TEST_PROMETHEUS="http://127.0.0.1:$prometheus_port"
export NINES_OPERATOR_TEST_RULE_DIRECTORY="$work/prometheus-rules"
export NINES_OPERATOR_TEST_MIMIR_CONTAINER="$mimir"
export NINES_OPERATOR_TEST_API="https://127.0.0.1:$api_port"
export NINES_OPERATOR_TEST_CA="$work/ca.pem"
export NINES_OPERATOR_TEST_TOKEN="$work/token"
export NINES_OPERATOR_TEST_MIMIR="http://127.0.0.1:$mimir_port"
cd "$root"
test_status=0
NINES_OPERATOR_TEST_CONTAINER="$server" go test -race -count=1 -json -timeout 20m ./integration/operator > "$work/go-results.json" || test_status=$?
# Publish only a complete report; another run cannot corrupt this run's gate.
cp "$work/go-results.json" ".$name-test-results.json"
mv ".$name-test-results.json" operator-test-results.json
python3 scripts/release/check-tests.py "$work/go-results.json" --required operator
[ "$test_status" = 0 ]

# Exercise the released container entrypoint with the actual service-account
# mount layout, verified API TLS, nonroot UID, probes and clean termination.
arch=$(docker version --format '{{.Server.Arch}}')
case "$arch" in amd64|arm64) ;; *) echo 'Unsupported fixture architecture'; exit 1 ;; esac
mkdir -p "$work/image" "$work/serviceaccount" "$work/config"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=false \
  -ldflags='-s -w -X github.com/ishantanu/nines/internal/buildinfo.Version=operator-acceptance' \
  -o "$work/image/nines-operator" ./cmd/operator
GOOS=linux GOARCH="$arch" python3 scripts/release/licenses.py --go-only --package ./cmd/operator --out "$work/image/LICENSES" > "$work/notices.json"
cp LICENSE NOTICE THIRD_PARTY_NOTICES.md "$work/image/"
cp deploy/operator/Dockerfile.binary "$work/image/Dockerfile"
docker build --quiet --tag "$name-runtime-image" "$work/image" > "$work/image-id"
cp "$work/token" "$work/serviceaccount/token"
cp "$work/ca.pem" "$work/serviceaccount/ca.crt"
cat > "$work/config/operator.yaml" <<'CONFIG'
namespace: team-commerce
tenant: commerce
projects: [commerce]
sources:
  primary:
    kind: prometheus-operator
    queryURL: http://127.0.0.1:1
    prometheusLabels:
      release: monitoring
CONFIG
chmod 755 "$work/config" "$work/serviceaccount"
chmod 444 "$work/config/operator.yaml" "$work/serviceaccount/token" "$work/serviceaccount/ca.crt"
docker exec "$server" kubectl -n team-commerce patch lease nines-operator --type=merge \
  -p '{"spec":{"renewTime":"2000-01-01T00:00:00.000000Z","leaseDurationSeconds":1}}' >/dev/null
docker run --rm "$name-runtime-image" -version | grep 'operator-acceptance' >/dev/null
docker run -d --name "$runtime" --network "$network" --publish 127.0.0.1::8081 \
  --read-only --cap-drop ALL --security-opt no-new-privileges --memory 256m \
  --env KUBERNETES_SERVICE_HOST=api --env KUBERNETES_SERVICE_PORT_HTTPS=6443 \
  --mount "type=bind,src=$work/config,dst=/etc/nines,readonly" \
  --mount "type=bind,src=$work/serviceaccount,dst=/var/run/secrets/kubernetes.io/serviceaccount,readonly" \
  "$name-runtime-image" -config /etc/nines/operator.yaml -health-addr :8081 >/dev/null
health_port=$(docker port "$runtime" 8081/tcp | sed 's/.*://')
ready=false
for _ in $(seq 1 60); do
  if curl --fail --silent "http://127.0.0.1:$health_port/readyz" >/dev/null; then ready=true; break; fi
  sleep 1
done
[ "$ready" = true ] || { echo 'Operator container readiness failed'; exit 1; }
curl --fail --silent "http://127.0.0.1:$health_port/healthz" >/dev/null
docker exec -i "$server" kubectl apply -f - < examples/operator/compiled.yaml >/dev/null
docker exec "$server" kubectl -n team-commerce wait --for=condition=Ready \
  slo/checkout-availability cuj/purchase --timeout=120s >/dev/null
docker stop --time 10 "$runtime" >/dev/null
[ "$(docker inspect --format '{{.State.ExitCode}}' "$runtime")" = 0 ] || { echo 'Operator did not terminate cleanly'; exit 1; }
printf '%s\n' '{"status":"passed","container":"nonroot-read-only","apiTLS":"verified","rbac":"shipped-role","probes":"passed","sloAndCuj":"reconciled","shutdown":"clean"}' > operator-runtime-test-results.json
cat operator-runtime-test-results.json
