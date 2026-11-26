#!/bin/sh
# Installs the Helm chart on a disposable kind cluster, with locally built
# images and a disposable PostgreSQL, then checks sign-in, a saved SLO that
# survives a server restart and an upgrade, operator reconciliation, and what
# uninstall keeps. Uses only the cluster it creates; KIND and HELM may name
# the binaries.
set -eu
kind=${KIND:-kind}
helm=${HELM:-helm}
for tool in docker kubectl python3 "$kind" "$helm"; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool"; exit 1; }
done
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=$(mktemp -d)
cluster="nines-helm-$$"
export KUBECONFIG="$work/kubeconfig"
tag="helm-acceptance-$$"
forward=''
cleanup() {
  result=$?
  [ -n "$forward" ] && kill "$forward" 2>/dev/null || true
  if [ "$result" != 0 ]; then
    kubectl -n nines get pods,events -o wide > helm-acceptance-diagnostics.log 2>&1 || true
    kubectl -n nines logs deployment/nines --tail=80 >> helm-acceptance-diagnostics.log 2>&1 || true
    kubectl -n nines logs deployment/nines-operator --tail=80 >> helm-acceptance-diagnostics.log 2>&1 || true
  fi
  "$kind" delete cluster --name "$cluster" >/dev/null 2>&1 || true
  docker image rm "nines:$tag" "nines-operator:$tag" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM
umask 077

echo 'Building images'
docker build --quiet --tag "nines:$tag" "$root" >/dev/null
docker build --quiet --tag "nines-operator:$tag" -f "$root/deploy/operator/Dockerfile" "$root" >/dev/null
"$kind" create cluster --name "$cluster" --kubeconfig "$KUBECONFIG" --wait 120s >/dev/null
"$kind" load docker-image --name "$cluster" "nines:$tag" "nines-operator:$tag" >/dev/null

# The operator's default source needs the PrometheusRule CRD. Immutable
# upstream release, checked by digest.
curl --fail --silent --show-error --location --max-time 120 \
  https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.94.1/stripped-down-crds.yaml > "$work/prometheus-crds.yaml"
printf '%s  %s\n' e35c128e5ae8607e04fe92e7d6aae28c7fc9cf24262549a84904fa0042ac21ae "$work/prometheus-crds.yaml" | shasum -a 256 -c - >/dev/null
kubectl apply --server-side -f "$work/prometheus-crds.yaml" >/dev/null

# A disposable database with a random password, never reused.
kubectl create namespace nines >/dev/null
password=$(python3 -c 'import secrets; print(secrets.token_hex(24))')
kubectl -n nines create secret generic postgres --from-literal=POSTGRES_PASSWORD="$password" >/dev/null
kubectl -n nines create secret generic nines-database \
  --from-literal=DATABASE_URL="postgres://postgres:$password@postgres:5432/nines?sslmode=disable" >/dev/null
kubectl -n nines apply -f "$root/integration/helm/postgres.yaml" >/dev/null
kubectl -n nines rollout status deployment/postgres --timeout=180s >/dev/null

port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
values="$work/values.yaml"
cat > "$values" <<EOF
publicURL: http://127.0.0.1:$port
database:
  existingSecret: nines-database
image:
  repository: nines
  tag: $tag
  pullPolicy: Never
operator:
  enabled: true
  image:
    repository: nines-operator
    tag: $tag
    pullPolicy: Never
EOF
install() {
  "$helm" upgrade --install nines "$root/charts/nines" --namespace nines --values "$values" --wait --timeout 5m >/dev/null
}
forward() {
  [ -n "$forward" ] && kill "$forward" 2>/dev/null || true
  kubectl -n nines port-forward service/nines "$port:8080" >/dev/null 2>&1 &
  forward=$!
  for _ in $(seq 1 60); do
    curl --fail --silent "http://127.0.0.1:$port/healthz" >/dev/null && return 0
    sleep 1
  done
  echo 'The server did not become reachable'
  exit 1
}
bootstrap() {
  kubectl -n nines get secret nines-credentials -o jsonpath='{.data.NINES_BOOTSTRAP_PASSWORD}' | python3 -c 'import base64, sys; print(base64.b64decode(sys.stdin.read()).decode())'
}

echo 'Installing the chart'
install
forward
first=$(bootstrap)
NINES_PASSWORD="$first" python3 "$root/integration/helm/check.py" --url "http://127.0.0.1:$port" --apply

echo 'Upgrading: credentials stay, data stays'
install
kubectl -n nines rollout restart deployment/nines >/dev/null
kubectl -n nines rollout status deployment/nines --timeout=180s >/dev/null
[ "$(bootstrap)" = "$first" ] || { echo 'The upgrade replaced the generated password'; exit 1; }
forward
NINES_PASSWORD="$first" python3 "$root/integration/helm/check.py" --url "http://127.0.0.1:$port"

echo 'Operator reconciles a ServiceLevelObjective'
kubectl -n nines apply -f - >/dev/null <<'EOF'
apiVersion: nines.dev/v1beta1
kind: ServiceLevelObjective
metadata:
  name: helm-acceptance
  annotations:
    nines.dev/project: default
spec:
  connection: primary
  displayName: Helm acceptance
  service: checkout
  target: 0.999
  window: 28d
  step: 1m
  queries:
    window:
      good: checkout_attempts_total{outcome="success"}
      total: checkout_attempts_total
  policies:
    - {name: fast-burn, severity: critical, shortWindow: 5m, longWindow: 1h, burnRate: 14.4, for: 2m}
EOF
reconciled=false
for _ in $(seq 1 60); do
  if kubectl -n nines get prometheusrules -o name 2>/dev/null | grep -q .; then reconciled=true; break; fi
  sleep 2
done
[ "$reconciled" = true ] || { echo 'The operator did not create a PrometheusRule'; exit 1; }

echo 'Uninstalling keeps CRDs, custom resources and credentials'
"$helm" uninstall nines --namespace nines --wait >/dev/null
kubectl get crd servicelevelobjectives.nines.dev >/dev/null
kubectl -n nines get slo helm-acceptance >/dev/null
kubectl -n nines get secret nines-credentials >/dev/null
echo '{"status": "passed", "chart": "charts/nines"}'
