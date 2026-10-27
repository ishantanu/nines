# Kubernetes deployment reference

The manifests run the normal Nines server with one replica and an external PostgreSQL database.

- They can run independently or use the [reviewed operator publishing bridge](operator.md#publish-portal-slos-through-the-operator) for rule deployment.
- The supplied server manifests have passed a disposable kind cluster rollout, account/RBAC checks and server-restart persistence.
- This does not establish production storage, ingress, HA or operator conformance across every Kubernetes/provider version.

## Build and provide an image

Build the source with the repository Dockerfile and push it to a registry you control:

```sh
# Replace the registry, repository and immutable tag or digest for your environment.
export NINES_IMAGE=registry.example.com/reliability/nines:reviewed-build
docker build -t "$NINES_IMAGE" .
docker push "$NINES_IMAGE"
```

- No public Nines image or Helm chart is assumed. Use a cluster-compatible image architecture. For kind or another local cluster, load the image using that cluster's documented image-loading workflow. Keep dependency notices in the image.

## Database and credentials

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
