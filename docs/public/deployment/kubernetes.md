# Kubernetes deployment reference

Nines runs on Kubernetes as one server replica with an external PostgreSQL database, plus an optional [operator](operator.md). Install it with the Helm chart, or apply the plain manifests yourself.

- CI installs the chart on a disposable kind cluster, signs in, saves an SLO, and checks that it survives an upgrade and a restart and that the operator deploys an SLO's rules.
- This does not establish production storage, ingress, HA or operator conformance across every Kubernetes/provider version.

## Install with Helm

You need a PostgreSQL database the cluster can reach; the chart does not install one. The database role must be able to create tables and schemas. Use private networking and verified TLS.

Put the database URL in a Secret, then install:

```sh
kubectl create namespace nines
kubectl -n nines create secret generic nines-database \
  --from-literal=DATABASE_URL='postgres://nines:URL_ENCODED_PASSWORD@database.internal:5432/nines?sslmode=verify-full'

helm install nines oci://ghcr.io/ishantanu/charts/nines --version 0.1.0-alpha.5 \
  --namespace nines --set database.existingSecret=nines-database
```

- Releases publish the chart from v0.1.0-alpha.5, pinned to that release's image digests and signed like the images. For an earlier version, or to test a change, install from a checkout: `helm install nines ./charts/nines ...`.
- The chart prints how to reach the portal and read the administrator password it generated. With the defaults, port-forward and sign in as `admin`:

```sh
kubectl -n nines port-forward service/nines 8080:8080
kubectl -n nines get secret nines-credentials -o jsonpath='{.data.NINES_BOOTSTRAP_PASSWORD}' | base64 -d; echo
```

### What the defaults give you

- Local accounts, with `admin` as owner of tenant `default` and platform administrator. Add people under **Access control**, and service accounts for automation.
- A generated administrator password and connection encryption key, in the Secret `nines-credentials`. The chart keeps it across upgrades and when you uninstall, so a reinstall against the same database still works. Back it up with the database.
- A restricted Pod: non-root, read-only root filesystem, no privilege escalation, all capabilities dropped and no Kubernetes API token.
- One replica, replaced on upgrade (`Recreate`). Nines collects and evaluates on a schedule; a second copy would collect and alert twice, so the chart does not offer replicas or autoscaling. Upgrades have a short downtime.
- A configuration change restarts the server, which reads its configuration at startup.

### Common settings

Set these in a values file and pass it with `--values`. The chart rejects unknown or misspelled settings. See [`values.yaml`](https://github.com/ishantanu/nines/blob/main/charts/nines/values.yaml) for all of them.

```yaml
database:
  existingSecret: nines-database

# HTTPS through your ingress controller; publicURL follows the host.
ingress:
  enabled: true
  className: nginx
  host: nines.example.com
  tlsSecretName: nines-tls

# Hosts that people may connect as telemetry sources in the portal.
connections:
  allowedHosts: [prometheus-operated.monitoring.svc:9090]

# Single sign-on and tenants: the same YAML as AUTH_CONFIG_FILE and
# TENANTS_FILE. Provider secrets come from environment variables.
auth:
  config: |
    publicURLEnv: NINES_PUBLIC_URL
    providers: [...]
tenants:
  config: |
    tenants: [...]
extraEnvFrom:
  - secretRef:
      name: nines-provider-credentials
```

- `auth.config` and `tenants.config` take the formats in [identity and tenants](identity-and-tenants.md). Keep credentials in Secrets, referenced through `extraEnvFrom`.
- To supply your own administrator password and encryption key instead of generated ones, set `credentials.existingSecret` to a Secret with `NINES_BOOTSTRAP_PASSWORD` and `NINES_SOURCE_ENCRYPTION_KEY`.
- Pin an image by digest with `image.digest`. Released charts already do.

### The operator

Set `operator.enabled: true` to run the [operator](operator.md) in the release namespace. It installs and upgrades the `nines.dev` CRDs (`crds.install`), which uninstall keeps so your resources are not deleted with them.

```yaml
operator:
  enabled: true
  tenant: default
  projects: [default]
  sources:
    primary:
      kind: prometheus-operator
      queryURL: http://prometheus-operated.monitoring.svc:9090
      prometheusLabels:
        release: monitoring
  # Let the portal publish reviewed SLOs here for the operator to deploy.
  publishFromPortal: true
```

- The default `prometheus-operator` source needs Prometheus Operator and its `PrometheusRule` CRD. For Mimir, set `kind: mimir` with `rulerURL`, `mimirOrgID` and `rulerNamespace`, and put the ruler token in `operator.credentialsSecret`.
- `publishFromPortal` gives the server's service account a token, limited to ServiceLevelObjectives in this namespace. Also set `rules.mode: operator` on the tenant's source; see [publish portal SLOs through the operator](operator.md#publish-portal-slos-through-the-operator).

### Upgrade and uninstall

```sh
helm upgrade nines oci://ghcr.io/ishantanu/charts/nines --version NEW_VERSION \
  --namespace nines --reuse-values
```

- Back up the database and the `nines-credentials` Secret first. Rolling back the chart does not undo database schema changes.
- `helm uninstall nines -n nines` keeps the CRDs, your custom resources and `nines-credentials`. Delete them yourself if you mean to remove Nines entirely.

## Install with manifests

The manifests in `deploy/kubernetes` run the same server without Helm. They can run independently or use the [reviewed operator publishing bridge](operator.md#publish-portal-slos-through-the-operator) for rule deployment.

### Choose an image

Use the published image for the release, pinned by the digest listed in the release notes:

```sh
export NINES_IMAGE=ghcr.io/ishantanu/nines:v0.1.0-alpha.4@sha256:DIGEST_FROM_THE_RELEASE_NOTES
```

- The image supports `linux/amd64` and `linux/arm64`, is signed and carries build provenance. See [container images](container-images.md) to verify it.
- `deploy/kubernetes/deployment.yaml` references the release tag. The `kubectl set image` step below replaces it with your pinned reference.
- To build your own instead, see [build your own](container-images.md#build-your-own) and push it to a registry your cluster can reach.

### Database and credentials

- Create the deployment namespace and provision a dedicated PostgreSQL database outside these manifests.
  - The database role needs to create Nines tables and tenant schemas.
  - Use private networking and verified PostgreSQL TLS.
  - Put the following values in a protected environment file outside the repository:

```text
DATABASE_URL=postgres://nines:URL_ENCODED_PASSWORD@database.internal:5432/nines?sslmode=verify-full
API_TOKEN=REPLACE_WITH_A_RANDOM_VALUE_OF_AT_LEAST_24_CHARACTERS
NINES_PUBLIC_URL=http://127.0.0.1:8080
NINES_BOOTSTRAP_PASSWORD=REPLACE_WITH_A_RANDOM_PASSWORD_OF_12_TO_72_BYTES
```

Use a secret manager in a shared cluster. For an isolated evaluation, create the Kubernetes Secret from that file:

```sh
kubectl create namespace nines --dry-run=client -o yaml | kubectl apply -f -
kubectl -n nines create secret generic nines-runtime --from-env-file=/secure/path/nines.env
kubectl -n nines apply -f deploy/kubernetes/auth-config.yaml
kubectl -n nines apply -f deploy/kubernetes/service.yaml
kubectl -n nines set image -f deploy/kubernetes/deployment.yaml nines="$NINES_IMAGE" --local -o yaml | kubectl -n nines apply -f -
kubectl -n nines rollout status deployment/nines
kubectl -n nines port-forward service/nines 8080:8080
```

- Keep the port-forward terminal open, then check `http://127.0.0.1:8080/healthz` in a separate terminal.
  - Sign in as `admin` using the bootstrap password from your protected runtime Secret.
  - The ConfigMap mounts local-account authentication; manage users under **Access control → Users**. API credentials remain available for automation.
  - Namespace Secrets are not encrypted merely because they are base64-encoded; follow your cluster's encryption and RBAC policy.

## Configuration, ingress and lifecycle

- The supplied `nines-auth` ConfigMap contains non-secret local-account configuration and is mounted read-only at `/etc/nines`.
  - Replace that ConfigMap with trusted tenant/identity YAML for SSO or multiple tenants; set `TENANTS_FILE` and `AUTH_CONFIG_FILE` to the mounted paths.
  - Credentials belong in Secret-backed environment variables referenced by the YAML.
  - Keep application tenant IDs separate from the Kubernetes deployment namespace.
  - The server does not need a Kubernetes service account token by default.
  - Optional [dependency discovery](../service-dependencies.md#enable-connected-cluster-discovery) requires explicitly enabling its dedicated ServiceAccount, namespace RoleBindings, token mount and tenant namespace allowlist.
- Add an ingress or Gateway with HTTPS according to your cluster standard. Keep the supplied Service as `ClusterIP`.
  - Set `NINES_PUBLIC_URL` (or a custom authentication `publicURL`) to the actual external origin and register its exact callback.
  - Source/ruler endpoints, DNS, private CAs and outbound identity/model/telemetry access require explicit network policy.
  - No ingress controller, certificate manager, storage class or cloud load balancer is assumed.
- Do not scale the Deployment above one replica. `Recreate` prevents overlapping collecting processes during replacement and entails downtime. Do not add a horizontal autoscaler.
  - The probes check the HTTP process; they do not validate database recovery or provider freshness.
  - Resource settings are starting values, not a scale guarantee.
- The root filesystem is read-only and the server has a bounded temporary directory.
  - If Prometheus rule files are configured, mount a writable persistent rule directory and arrange provider-side loading; the default manifests do not supply one.
  - Mimir ruler output uses the configured remote destination instead.
- Back up the external database, identity data, configuration and secrets before upgrading. Rollback is not guaranteed to undo schema changes. See [backup and restore](backup-and-restore.md) and [operational limits](../operations.md).
- Reference: Kubernetes [Recreate deployment strategy](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#recreate-deployment).

## Telemetry connections and deployment checks

- Add `NINES_SOURCE_HOSTS` and a private base64 32-byte `NINES_SOURCE_ENCRYPTION_KEY` to `nines-runtime` to enable managed source connections.
  - Generate the key with `openssl rand -base64 32`; retain it separately with recovery secrets.
  - Use Kubernetes service DNS names in endpoints.
  - Mimir organization boundaries still come from trusted tenant YAML, and Prometheus recording files need a writable shared volume plus provider-side loading and reloads.
- After a Secret or ConfigMap change, restart the single Deployment and check its rollout.
  - Sign in, create a restricted user, test the intended provider, apply a reviewed SLO and verify data coverage.
  - Review and deploy its rules separately; a healthy Pod does not establish working SLO collection or alert delivery.
  - Validate database persistence and recovery before exposing ingress.
