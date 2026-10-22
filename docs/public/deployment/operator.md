# Kubernetes operator

The operator is experimental. It reconciles Nines CRDs into Prometheus Operator `PrometheusRule` resources or a configured Mimir ruler namespace. It shares the platform's deterministic compiler and `nines.dev/v1alpha1` definitions, including named severity policies. It does not require the Nines API server, PostgreSQL or a model provider.

The operator is separate from [deploying the Nines server on Kubernetes](kubernetes.md). CRDs and their status are not automatically synchronized into the Nines portal or its database. The supported MVP path remains the server/CLI and [integration playground](playground.md). The controller has unit and simulated API coverage, but full real-cluster acceptance is still outstanding.

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

## Run the controller in an evaluation cluster

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

## Mimir and intents

For Mimir, replace the operator ConfigMap's provider configuration with [the Mimir example](../../../deploy/operator/mimir-config.yaml). Pin its query/ruler URLs, organization ID and ruler namespace, then mount the configured bearer-token Secret file read-only if authentication is required. The upstream dataset and ingestion must already exist. Mimir mode does not require Prometheus Operator. Resources cannot choose their own upstream organization or destination.

[The intent example](../../../examples/operator/intent.yaml) uses an explicitly selected MeasurementBinding generation. It queries provider evidence before deterministic compilation. It does not invoke an LLM or infer a measurement from free text. Policy arrays are carried into the compiled SLO. Review its instrumentation, project, selection and journey links before applying it.

One installation is bound to one namespace/tenant boundary. The controller uses a namespace Lease and ownership checks; Mimir cleanup uses a finalizer and the original configured route. Keep that configuration and credentials available until cleanup completes. Do not remove finalizers or uninstall CRDs as a shortcut: deleting a CRD removes all resources of that kind across the cluster.

See [experimental limits](../experimental.md) for the acceptance work remaining before promoting the operator into the supported MVP path.
