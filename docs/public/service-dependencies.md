# Service dependencies

A service dependency map links callers to the services they depend on and associates each service with its SLOs.

- Use it to review reliability risks across a critical user journey.
- A map describes relationships; it does not prove causality or calculate end-to-end journey availability.

## Create and edit a map

- Open **Service dependencies → Define dependency map**.
  - Set the map identifier, project and display name.
  - The editor has three views of the same draft:

* **Visual editor:** add services, drag their title handles to arrange them, and drag an arrow handle onto another service to draw a caller-to-dependency link. Select a service to edit its name, display name and SLO references. Mark a dependency critical only after reviewing its effect on the caller.
* **YAML:** edit the native `ServiceDependencyMap` resource. Valid changes appear in the visual editor.
* **Discover from Kubernetes:** import a local resource export or preview inventory from a configured cluster, then add the candidates to the draft.
* Keyboard users can focus a service's move handle and press arrow keys to reposition it.
  - The caller/dependency form creates the same links without dragging.
  - Removing a service removes its draft links. **Reset layout** returns to automatic positioning.
  - Saved positions belong to the map's revision and do not alter dependency semantics.
* Select **Validate and plan**, inspect the revision and previous definition, then **Apply reviewed definition**. Every edit invalidates the plan.
  - Owners and editors can apply; viewers and agent credentials can preview and plan. Concurrent changes require a fresh plan.
  - Nothing is saved or deployed by moving a node or previewing discovery.
* SLO references must identify existing SLOs in the current tenant, with the same service identity as the node.
  - Renaming a node can therefore require updating its SLO references.
  - Discovery merges retain existing SLO links, criticality, manual edges, positions and observed-source settings.

## Import Kubernetes resources

Import works without granting the Nines server any Kubernetes access. Export only the resources needed for the map:

```sh
kubectl get services,deployments,statefulsets,daemonsets,ingresses \
  -n commerce -o yaml > commerce-resources.yaml
```

- Choose **Discover from Kubernetes → Local resource YAML or JSON**, open that file, select **Preview discovery**, review the evidence and warnings, then **Add discovery to draft**.
  - The browser projects the export locally before sending a bounded inventory to Nines.
  - Only resource identities, selectors, labels, supported service annotations, host names and backend names enter that request.
  - Full URLs, user information, paths, unrelated environment values, image names, commands, status and Secret/ConfigMap references are discarded.
  - Treat the original export as sensitive and keep it out of version control.

The CLI performs the same projection locally and can run entirely offline:

```sh
nines dependencies import-kubernetes --offline \
  --file commerce-resources.yaml --namespace commerce \
  --name commerce-services --project production \
  --out commerce-services.yaml
nines validate --offline -f commerce-services.yaml
# After inspecting the candidate map, use the intended tenant context:
nines plan -f commerce-services.yaml --context production --agent
nines apply -f commerce-services.yaml --expected-revision 0 --context production --agent
```

- Omit `--out` to receive a JSON preview with the manifest, inventory digest, evidence and warnings.
  - Output files are created exclusively and never overwritten.
  - Agent credentials cannot bypass server-side write permissions.
  - A namespace flag supplies the default for resources with no namespace; explicitly namespaced resources retain their identities, including cross-namespace references in a multi-namespace export.
- Supported imports include Services, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, Pods and Ingress resources in a YAML stream, JSON document or Kubernetes List.
  - The default identities are `namespace/service-name` for Services and `namespace/kind/name` for unmatched workloads and Ingress.
  - Long workload identities receive a deterministic hash suffix to fit the 200-character map limit.

## What discovery infers

| Evidence                                                                                                           | Candidate relationship                                                                |
| ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------- |
| Service selector matches pod-template labels                                                                       | Workload uses that Service's identity; no edge is created merely by matching labels.  |
| Literal `*_HOST`, `*_URL`, `*_URI`, `*_ENDPOINT`, `*_ADDRESS`, `*_ADDR`, `*_DSN` or `*_HOSTNAME` environment value | Workload calls a Service whose name matches the sanitized host.                       |
| Ingress service backend                                                                                            | Ingress node routes to that Service.                                                  |
| Service `externalName`                                                                                             | Alias routes to a Service present in the same inventory.                              |
| `nines.dev/depends-on` annotation                                                                                  | Explicitly declared caller-to-dependency relationship, resolved within the inventory. |

For logical names that already appear in SLOs, annotate the Service or workload:

```yaml
metadata:
  annotations:
    nines.dev/service: api
    nines.dev/depends-on: database.commerce.svc.cluster.local, commerce/cache
```

- Pod-template annotations override workload annotations.
  - Dependency annotations accept comma-separated service identities or Kubernetes service DNS names.
  - A workload matching multiple different Service identities is skipped with a warning unless its identity is explicit. All candidate links default to noncritical.
  - NetworkPolicy permissions, co-location in a namespace and shared labels do not establish calls.
  - External hosts, variable substitution, Secrets, ConfigMaps, IP addresses and mesh configuration are not resolved.
  - Custom cluster DNS suffixes require explicit dependency annotations.
- The preview is limited to 2 MiB and 1,000 resources; the resulting map supports 40 services and 100 directed links.
  - Evidence is capped at 200 records with a truncation warning.
  - Narrow larger exports instead of accepting a partial map silently.
  - Discovery runs on demand and does not continuously reconcile a saved map.

## Enable connected-cluster discovery

- This optional reader runs inside the Nines server's Kubernetes cluster.
  - It uses the mounted service-account CA and rotating token, verifies API TLS and refuses redirects.
  - It does not use kubeconfig or accept a cluster URL or credentials from the browser.
  - Local Compose and `-demo` mode use resource imports instead.

Add an explicit namespace allowlist to each authorized tenant's server-owned configuration:

```yaml
tenants:
  - id: commerce
    name: Commerce
    access:
      - tokenEnv: COMMERCE_API_TOKEN
        role: owner
    source:
      kind: prometheus
      url: http://prometheus.monitoring.svc:9090
    kubernetesDiscovery:
      namespaces: [commerce]
```

- Create a dedicated ServiceAccount and a Role/RoleBinding for each allowed application namespace.
  - The reference [discovery RBAC manifest](https://github.com/ishantanu/nines/blob/main/deploy/kubernetes/dependency-discovery-rbac.yaml) grants **list only** on Services, Deployments, StatefulSets, DaemonSets and Ingress.
  - Adapt its `nines` installation namespace, `commerce` application namespace and ServiceAccount name before applying it.
  - It grants no Secret, ConfigMap, Pod, exec, write or cluster-wide permissions.
  - Kubernetes list responses still include workload configuration and literal environment values, so only enable namespaces whose configuration may be processed by this Nines installation.

In the Nines Deployment, explicitly set:

```yaml
spec:
  template:
    spec:
      serviceAccountName: nines-discovery
      automountServiceAccountToken: true
```

- The normal deployment disables token mounting.
  - Keep it disabled when discovery is not used.
  - Mount the trusted tenant file, set `TENANTS_FILE` to its path, restart the single Nines replica, then choose **Connected Kubernetes cluster** in the discovery tab.
  - Network policy must permit the Kubernetes API. See [Kubernetes deployment](deployment/kubernetes.md).

```sh
nines dependencies discover --namespace commerce \
  --name commerce-services --project production \
  --context production --agent
```

- The CLI asks Nines to read only that tenant's configured namespace and returns a preview; it does not save a map. Requests are time-bounded, paginated and size-limited.
  - An unavailable API, missing RoleBinding or forbidden namespace produces an error, not an empty successful discovery.
  - Tenants sharing a namespace can see the same sanitized inventory; use distinct namespaces where inventory isolation is required.
- These permissions follow Kubernetes' [namespace-scoped RBAC guidance](https://kubernetes.io/docs/concepts/security/rbac-good-practices/).
  - Run `make test-dependencies` to verify the reader against a disposable Kubernetes API with the shipped Role, verified TLS, tenant boundaries, revision checks and permission revocation.
  - The fixture does not load your kubeconfig or schedule workloads.

## Observed dependencies

- Observed service-graph metrics can complement the reviewed map through its existing `spec.observed` source, labels and window.
  - Declared edges use solid lines and observed edges use dashed lines.
  - Configuration discovery never relabels a candidate as observed traffic.
  - Missing trace coverage or an absent edge cannot establish that a service is independent or healthy.

### Read observed evidence

- Each observed link records the configured telemetry connection, a nonsecret connection fingerprint, the counter name, query window and read time. **Fresh sample** refers to the latest actual counter sample inspected inside that window, rather than the timestamp of an instant query result. **Stale sample**, **Sample age unverified**, **Source unavailable** and **Not seen in window** describe different gaps and remain visible in the evidence table. Expand **Evidence details** to inspect the timestamps and limits.
- Configure `scrapeIntervalSeconds` to match the exporter scrape cadence and `maxSampleAgeSeconds` to set the tolerated sample age.
  - Defaults are 15 seconds and three scrape intervals (at least 30 seconds).
  - Sample continuity compares observed samples with the expected samples of the series present in the window.
  - It does not measure instrumentation or trace-sampling coverage.
  - Those remain **unknown**: Nines cannot know about services that were never instrumented.
  - A counter can be fresh while reporting no new calls; freshness alone does not establish active traffic.
- Declared and observed directed links are matched by caller and dependency.
  - A declared link absent from the selected window remains declared; an observed-only link suggests a relationship to review. Kubernetes candidates retain their configuration provenance.
  - Renamed services appear as different identities until the reviewed map is updated.
  - Stale or age-unverified observed links are excluded from inferred upstream impact. Existing declared critical links still participate.
  - Impact identifies possible exposure; linked SLOs determine health.
- Queries return at most 100 observed edges and views at most 80 nodes.
  - Metadata is bounded independently, uses an inspection resolution of at least one scrape interval and at most 240 steps, and reports missing metadata as unverified. Truncation is explicit.
  - Narrow the labels or window when necessary.
  - Journey links show the affected service steps where present, with bounded details; open the journey for its full definition.

### Run an instrumented example

- The optional topology profile adds three small HTTP services and a traffic generator.
  - Each service exposes request counters; outbound HTTP wrappers expose `traces_service_graph_request_total`. This example measures real calls directly.
  - It does not run a trace processor or claim tracing coverage.
  - Production deployments can supply the same service-graph counter through a tracing pipeline such as Tempo's metrics generator.

```sh
# Prepare .env and its generated secrets as described in the playground guide.
./playground up --topology

nines apply --server http://localhost:18080 --tenant prometheus \
  -f examples/service-graph/gateway.slo.yaml --expected-revision 0
nines apply --server http://localhost:18080 --tenant prometheus \
  -f examples/service-graph/checkout.slo.yaml --expected-revision 0
nines apply --server http://localhost:18080 --tenant prometheus \
  -f examples/service-graph/payments.slo.yaml --expected-revision 0
nines apply --server http://localhost:18080 --tenant prometheus \
  -f examples/service-graph/journey.yaml --expected-revision 0
nines apply --server http://localhost:18080 --tenant prometheus \
  -f examples/service-graph/dependencies.yaml --expected-revision 0
```

- Use your configured tenant context or `NINES_API_KEY`; do not put API keys in shell arguments.
  - For Mimir, add `--mimir` when starting the playground.
  - The existing Prometheus remote-write route forwards the example counters to Mimir organization `playground`.
  - Use `--tenant mimir` when applying the examples to the Mimir workspace; tenant identifiers and provider organization IDs are separate.
  - Open **Service dependencies → Checkout services** after at least two scrapes.
  - The two observed links should match their declared links.
  - Linked SLOs need enough retained samples before reporting complete-window attainment.
- `make test-topology` runs a separate disposable fixture: real HTTP traffic, real Prometheus scraping, journey-step links, tenant isolation, then a paused exporter to verify stale evidence. `make test-operator` additionally checks cleanup across provider outage and credential rotation, plus independent critical/warning pending, firing and recovery transitions using the actual operator-generated rules on Prometheus and Mimir. The agentless fixture bridges the Prometheus Operator's selected ConfigMap to a disposable evaluator; it does not schedule a Prometheus Pod.
