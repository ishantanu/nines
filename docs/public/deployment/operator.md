# Kubernetes operator

The Kubernetes operator is a first-class Nines deployment mode for GitOps-managed SLOs and critical user journeys. It reconciles shared `nines.dev/v1alpha1` definitions into Prometheus Operator `PrometheusRule` resources or a pinned Mimir ruler namespace. It uses the same deterministic compiler, validation, recording windows, configuration markers and severity policies as the CLI and server. No Nines server, PostgreSQL or LLM is required.

Choose the operator when Kubernetes and Git are your configuration source of truth. Choose the [server deployment](kubernetes.md) when you need the portal, database history, tenant users and interactive review workflows. Standalone GitOps resources are not imported into the portal. For portal-owned SLOs, the reviewed publishing bridge below connects the server to this operator. Kubernetes ServiceAccounts/RBAC and the configured namespace/tenant boundary authorize reconciliation; manual portal publication additionally requires the server owner role and an exact reviewed deployment digest. Owners can instead enable `rules.autoDeploy: true` so approved saved definitions publish automatically under the data source policy; agent approval restrictions remain unchanged. Both modes are part of the alpha release; neither declares stable API compatibility or production-scale certification.

## Render rules without running a controller

Requires Go 1.27.3 to build the CLI. These commands need no running Nines server or Kubernetes cluster:

```sh
make cli
bin/nines operator render --offline \
  -f examples/api-availability.slo.yaml \
  --namespace team-commerce --tenant commerce --rules-only
```

This prints a `monitoring.coreos.com/v1` `PrometheusRule` with recording rules and critical/warning alerts. Applying it requires the Prometheus Operator CRD and a Prometheus instance whose rule/namespace selectors include it. Review the generated metadata and add the selectors required by your installation before applying. GitOps owns this output; do not also give the Nines controller or server ownership of the same rules.

For plain Prometheus rule files instead of Kubernetes resources:

```sh
bin/nines rules generate --offline \
  -f examples/api-availability.slo.yaml --tenant commerce
```

To wrap native versioned YAML as a Nines CRD, omit `--rules-only`. Native `metadata.project` becomes the `nines.dev/project` annotation. The project must be allowed by the live controller's configuration before it can reconcile that resource. Kubernetes namespace and Nines tenant are independent identities; the controller's trusted configuration fixes their relationship.

## Install the controller

Prerequisites:

- A Kubernetes cluster selected explicitly in your kubeconfig, with permission to install cluster-scoped CRDs and the supplied namespaced RBAC.
- Prometheus Operator already installed for the default `prometheus-operator` source, including the `PrometheusRule` CRD.
- A reachable Prometheus query endpoint and rule selectors that include `team-commerce` and the configured `release: monitoring` label.
- An operator image available to the cluster.

Check your target, then build the image:

```sh
kubectl config current-context
docker build -f deploy/operator/Dockerfile -t nines-operator:dev .
```

For kind, load the image into your explicitly selected evaluation cluster:

```sh
# Replace YOUR_KIND_CLUSTER with its name.
kind load docker-image nines-operator:dev --name YOUR_KIND_CLUSTER
```

For another cluster, push to a registry it can access and replace the Deployment image in your reviewed copy of `deploy/operator/operator.yaml`.

Before applying, edit that file's ConfigMap for your actual provider. The example binds Kubernetes namespace `team-commerce` to Nines tenant `commerce`, allows project `commerce`, uses `http://prometheus.monitoring.svc:9090`, and labels generated rules `release: monitoring`. These are examples, not automatic discovery or a bundled Prometheus installation.

```sh
kubectl apply -f deploy/operator/crds.yaml
kubectl wait --for=condition=Established --timeout=60s \
  crd/servicelevelobjectives.nines.dev \
  crd/criticaluserjourneys.nines.dev \
  crd/measurementbindings.nines.dev \
  crd/servicelevelobjectiveintents.nines.dev

kubectl apply -f deploy/operator/operator.yaml
kubectl -n team-commerce rollout status deployment/nines-operator --timeout=120s
kubectl apply -f examples/operator/compiled.yaml

kubectl -n team-commerce get slo,cuj,prometheusrules
kubectl -n team-commerce get slo checkout-availability -o yaml
kubectl -n team-commerce logs deployment/nines-operator --tail=100
```

The compiled sample includes a critical and a warning policy plus a critical user journey linked to the SLO. Its queries require real `checkout_attempts_total` instrumentation matching the declared selectors. `Ready=True` / `RulesConfigured` establishes configuration, not metric coverage, successful external evaluation or alert notification delivery. Inspect the selected rules and queries in Prometheus separately.

## Publish portal SLOs through the operator

The server can publish a reviewed portal SLO as a Kubernetes `ServiceLevelObjective`; the operator then owns its `PrometheusRule` or Mimir group. Portal and GitOps resources have distinct ownership. Existing GitOps CRs are never adopted or overwritten. This bridge runs with the server and operator in the same Kubernetes cluster and does not load a host kubeconfig or accept a caller-supplied Kubernetes endpoint.

Install the operator first. Configure its namespace/tenant/project boundary and source name to match the portal connection. For example, a server tenant using the operator's `primary` source can set:

```yaml
tenants:
  - id: commerce
    name: Commerce
    access:
      - tokenEnv: COMMERCE_OWNER_TOKEN
        role: owner
    source:
      kind: prometheus
      url: http://prometheus.monitoring.svc:9090
      rules:
        mode: operator
        namespace: team-commerce
        publisher: nines-portal
```

Put this configuration in the server's `TENANTS_FILE`. Named `connections` use the same `rules` fields; their names must also exist in the operator's source map. Mimir uses `kind: mimir`, its query URL and pinned `mimirOrgID` here, with ruler credentials and namespace configured only in the operator. The operator rejects mismatched portal tenants, provider kinds or Mimir organizations. A namespace cannot be shared by different portal tenants. `publisher` is a stable, administrator-chosen ownership identity; changing it is a destination change.

Grant the server a dedicated service account. The optional examples assume the server is named `nines` in namespace `nines`, and the operator manages `team-commerce`. Review and adjust those names first:

```sh
kubectl apply -f deploy/operator/portal-rbac.yaml
kubectl -n nines patch deployment nines --type=strategic \
  --patch-file deploy/operator/portal-server-patch.yaml
# Mount/update TENANTS_FILE in your server deployment, then restart it.
kubectl -n nines rollout restart deployment/nines
kubectl -n nines rollout status deployment/nines
```

The server service account can get/create/update/delete SLO CRs only in the configured target namespace. It cannot write `PrometheusRule`, read provider Secrets or update operator status. The normal server deployment keeps service-account token mounting disabled; the optional patch enables its mounted token and verified Kubernetes CA for this bridge. The operator retains its separate service account, provider credentials and lease. These examples grant no cluster-wide runtime access.

With `rules.autoDeploy: true`, saving an SLO or applying an approved intent change set queues publication automatically. In manual mode, create or edit an SLO using that source, open **Rules**, review the namespace, publisher, Kubernetes YAML and generated rule preview, then choose **Apply reviewed rules**. The CLI uses the same manual endpoints:

```sh
nines rules plan --context commerce --id checkout-availability --agent
nines rules apply --context commerce --id checkout-availability \
  --expected-digest REVIEWED_DIGEST --agent
nines integration checkout-availability --context commerce --agent
```

Planning reads Kubernetes ownership and generation but performs no writes. Apply pins the destination in PostgreSQL before the mutation, uses resource-version concurrency checks, and refuses foreign ownership or a changed reviewed plan. An ambiguous write acknowledgement gets bounded exact-content readback, never a blind repeated write. The response first reports **Published to Kubernetes** / **Awaiting operator**. Successful reconciliation, provider evaluation, query-output visibility and SLO health remain separate facts. The portal refreshes integration status after apply and periodically thereafter.

Portal revisions are recorded independently of Kubernetes generations. Reconciliation and output verification bind the current portal revision, definition digest and actual Kubernetes UID. A new CR starting at generation 1 can represent portal revision 3; a metadata-only revision update cannot reuse previous positive status. A newly created resource receives its UID at creation, so the pre-creation rule group is explicitly a preview. Standalone GitOps resources continue to use Kubernetes generations.

To stop deployment while retaining the portal definition and collected history, review the current plan and select **Withdraw reviewed publication**, or use:

```sh
nines rules withdraw --context commerce --id checkout-availability \
  --expected-digest REVIEWED_DIGEST --agent
```

Withdrawal deletes only the owned SLO parent using UID/resource-version preconditions. The original route stays pinned while Mimir finalizers and foreground dependent cleanup run. If the result is **Withdrawal pending**, wait for cleanup, refresh the plan, and repeat withdrawal to confirm absence and release the pin. Keep the operator and original provider credentials running throughout. Only then change the destination or deployment mode and review a new plan. Kubernetes absence is configuration evidence; provider runtime removal may still await Prometheus Operator reload, and historical recorded samples remain until backend retention expires. Restore the original server routing first if it was edited prematurely. This also prevents switching to a direct provider writer while an operator still owns the publication.

The MCP adapter also supports reviewed operator publication. Withdrawal is a separate opt-in: `-allow-rule-withdrawal` requires both `-allow-writes` and `-allow-rule-deployment`, plus an owner API credential. Enabling deployment alone does not expose the withdrawal tool.

This feature publishes portal-owned SLOs and reads their deployment evidence back. Standalone GitOps CRs and Kubernetes CUJ/binding/intent resources are not imported into the portal; portal CUJ relationships remain in the server. Intent-derived SLOs can use this same reviewed publication step. Install matching server/operator builds so the controller reports portal revision evidence.

## Mimir and intents

For Mimir, replace the operator ConfigMap's provider configuration with [the Mimir example](../../../deploy/operator/mimir-config.yaml). Pin its query/ruler URLs, organization ID and ruler namespace, then mount the configured bearer-token Secret file read-only if authentication is required. The upstream dataset and ingestion must already exist. Mimir mode does not require Prometheus Operator. Resources cannot choose their own upstream organization or destination.

[The intent example](../../../examples/operator/intent.yaml) owns `spec.measurement` and `stepSeconds` inside the intent. No binding CR or selection is needed. The same measurement fields work directly inside an SLO CR. Legacy binding-generation selections remain accepted as an alternative. It queries provider evidence before deterministic compilation. It does not invoke an LLM or infer a measurement from free text. Policy arrays are carried into the compiled SLO. Review its instrumentation, project, measurement and journey links before applying it.

One installation is bound to one namespace/tenant boundary. The controller supports standby replicas and uses a namespace Lease and ownership checks; Mimir cleanup uses a finalizer and the original configured route. Keep that configuration and credentials available until cleanup completes. Do not remove finalizers or uninstall CRDs as a shortcut: deleting a CRD removes all resources of that kind across the cluster.

## Status and failure diagnosis

```sh
kubectl -n team-commerce get slo,cuj,measurementbindings,servicelevelobjectiveintents
kubectl -n team-commerce get slo checkout-availability -o jsonpath='{.status}'
kubectl -n team-commerce get lease nines-operator
```

`status.observedGeneration` and each condition identify the reconciled Kubernetes generation. `Ready=True` means the declared configuration was processed, not that an error budget is healthy. SLO status separately reports `deployment`, `evaluation` and `ruleOutput`, with diagnostic codes and corrective actions shared with the server/CLI. A matching `PrometheusRule` can coexist with `evaluation: not_loaded` when labels or namespaces do not match the Prometheus selectors. Mimir can evaluate correctly while query output is missing in another dataset. Evidence from a previous generation is cleared on reconciliation failure or project denial.

`ScopeDenied` means the project is not allowed. `ReconcileFailed` includes invalid definitions, source/destination route changes, stale binding selection and foreign ownership. `CleanupBlocked` preserves the Mimir finalizer when the original route, credentials or ownership cannot be verified. Fix the cause and let reconciliation retry. Do not remove the finalizer or bypass ownership checks.

The controller polls its namespace every ten seconds and renews its thirty-second lease every five seconds. Loss of renewal cancels reconciliation; mutations also check the last successful renewal. Resource requests have bounded deadlines. A standby remains ready without holding the lease. Scale the Deployment to two replicas for standby capacity; only one writer is active per namespace. This is independent from the single-process server collector's limits. The controller bounds namespace listings to 1,000 objects per kind; this is a guard, not a scale benchmark.

`/healthz` checks the process. `/readyz` verifies access to all four CRDs and, for Prometheus Operator sources, the `PrometheusRule` API. These probes do not certify provider availability, leader ownership, useful traffic or notifications. Logs use the shared OpenTelemetry setup; configure OTLP exporters through the normal telemetry environment variables.

## Credentials, upgrades and rollback

Use `queryBearerTokenFile` and `rulerBearerTokenFile` for separate scoped credentials. `bearerTokenFile` remains a shared-credential fallback. Token files are read for each request, supporting mounted Secret rotation; credentials are never stored in CRD specs or status. The shipped Role grants namespace resource/status access, owned rule writes and the namespace Lease, without reading Secrets, deleting rule objects or granting cluster administration. Mount only the provider Secret files the operator needs. Kubernetes authorization governs who may edit SLO specs, intent selections and the trusted ConfigMap.

Build a versioned image from a source checkout using `deploy/operator/Dockerfile`, or use a Linux release archive matching the cluster architecture:

```sh
# Run in the extracted Linux release archive.
./nines-operator -version
docker build -f deploy/operator/Dockerfile.binary -t YOUR_REGISTRY/nines-operator:YOUR_VERSION .
```

Push that image to your registry, set it in a reviewed copy of the Deployment, and apply it. The manifests also have a [Kustomize entry point](../../../deploy/operator/kustomization.yaml); `kubectl kustomize deploy/operator` renders the source-checkout bundle for review. Review the fixed example namespace, provider URLs, tenant, projects, labels and image before `kubectl apply -k`. The installation includes cluster-scoped CRDs; runtime permissions remain namespaced.

For upgrades, review CRD changes and save desired manifests/configuration in Git, apply the CRDs, update the image and use `kubectl rollout status`. Configuration is loaded at startup: restart the Deployment after ConfigMap changes. File-based credential contents can rotate without changing the route identity. Keep the original Mimir route available until all groups on it have been cleaned up; route changes require deletion with the original configuration before recreating the resource.

Rollback means reverting the reviewed desired YAML and/or Deployment image in Git, applying it, then checking the new observed generation, loaded rules and query marker. A new Kubernetes generation is expected. Measurement selectors, sampling intervals and saved binding revisions can change under the same SLO name. New generation/revision labels isolate recording series and alert timers; generated selectors never consume another revision's interval recordings. Kubernetes/GitOps retains desired-definition history; the standalone operator does not archive external provider samples. Source/destination route transfers still require cleanup on the original route before recreation. Reverting configuration cannot restore a consumed error budget or recover deleted provider data. Remove SLOs and wait for Mimir finalizers before uninstalling the controller. Leave shared CRDs installed unless an administrator has verified all namespaces; deleting a CRD deletes all its custom resources.


## Run operator acceptance locally

```sh
make test-operator
```

Requires Docker, Go, kubectl, curl, base64, shasum and Python for the existing release gate/license tooling. The runner creates a private Docker network, a loopback-only TLS K3s API, a real Prometheus Operator and disposable Mimir. It installs the checked-in CRDs and exact shipped namespace Role, then tests the production Nines controller using a short-lived ServiceAccount token. No host kubeconfig/context or existing cluster is used. Temporary credentials, containers, image and network are removed afterwards.

Coverage includes real structural admission, namespace RBAC, project denial, all four resource kinds, exact binding-generation selection, owned create/update/delete recovery, foreign ownership refusal, current-generation status, cascading garbage collection, lease revocation/standby takeover, Mimir acceptance, blocked cleanup and successful finalizer cleanup. Intent telemetry metadata is synthetic, so this does not establish real model quality. The real upstream controller additionally verifies both selector dimensions and removal from generated rule ConfigMaps.

The packaged operator is run as a nonroot, read-only container with normal service-account paths and verified API TLS. Acceptance checks `/healthz`, `/readyz`, reconciliation of the shipped SLO/CUJ sample, and clean termination. K3s is agentless to keep the fixture small: it does not schedule workload pods, benchmark scale or establish every provider/Kubernetes version combination. CI requires the real cases to run and the container checks to succeed. Run these checks on the exact candidate commit before release.
