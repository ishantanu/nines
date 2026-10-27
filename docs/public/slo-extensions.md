# SLO extensions

Nines extensions let teams distribute measurement recipes, typed inputs, policy presets, evaluation contracts and deterministic conformance cases without rebuilding Nines.

- Built-in templates ship with Nines and need no installation or lockfile.
- Extensions add recipes to the same catalog and use the same deterministic measurement compiler and SLO validation. Bundles are ordinary versioned YAML.
- The CLI, server, portal, MCP and operator renderer share the same loader and compiler.
- This first extension contract supports local files, Git repositories and explicit checksum-pinned HTTPS artifact downloads. Renderers use installed files only. There is no runtime script execution, Go interpreter, model call, registry discovery or automatic update. Native telemetry-provider adapters remain a [separate extension boundary](provider-adapters.md).

## Try the included extension

The repository includes a [complete example bundle](../../examples/extensions/example-agent.yaml), [pinned lockfile](../../examples/extensions/example.lock.yaml) and [typed values](../../examples/extensions/support-values.yaml). No installation or network access is needed:

```sh
nines --offline extensions test -f examples/extensions/example-agent.yaml
nines --offline extensions list --lock examples/extensions/example.lock.yaml
nines --offline templates list --extension-lock examples/extensions/example.lock.yaml
nines --offline templates generate example-agent--evaluation-pass \
  --extension-lock examples/extensions/example.lock.yaml \
  --values examples/extensions/support-values.yaml \
  --out support-evaluation.slo.yaml
nines --offline validate -f support-evaluation.slo.yaml
```

- The generated file is a normal `nines.dev/v1alpha1` ServiceLevelObjective.
  - It owns its complete measurement and has no extension dependency.
  - Keep its draft receipt and lockfile alongside review artifacts if you need reproducible template provenance.
  - Existing SLO revision history records the rendered definition, not an executable extension reference.

## Bundle contract

- A bundle uses `apiVersion: nines.dev/v1alpha1` and `kind: SLOExtensionBundle`.
  - Metadata declares an extension name, publisher description and exact semantic version. `spec.engine: nines.dev/measurement-v1` selects the supported compiler capability.
  - Unknown engines, fields and resource versions are rejected.
- A recipe contains:

* A local name, SLI category, title, eligible events, good events and requirements.
* A complete experimental SLO manifest with an inline counter-ratio or classic-histogram measurement.
* Named typed inputs mapped to supported measurement or SLO fields.
* Optional references to policy presets and evaluation contracts declared in the same bundle.
* Catalog identifiers combine the extension and recipe names, for example `example-agent--evaluation-pass`.
  - Extension names are lowercase identifiers of at most 30 characters; recipe/category/input names have at most 24. Names starting with `nines` are reserved.
  - Custom categories are namespaced with the same prefix. Built-in categories remain available.
  - A lockfile can contain one version of each extension; duplicate identities and collisions are rejected.
* Versions use three numeric components, optionally followed by a lowercase SemVer prerelease.
  - Build metadata is not supported by this contract.
  - Changing published bytes requires a new version and digest, even when the change is only formatting.
  - Recipe `revision: 1` in catalog output describes the current template contract; the extension version and artifact digest identify the bundle release.

### Typed inputs

- Inputs have a description, type, required flag and optional default.
  - Numbers accept minimum/maximum constraints; strings can declare allowed values.
  - An input can target only these fields:

| Field                                                   | Type                                             |
| ------------------------------------------------------- | ------------------------------------------------ |
| `service`, `metric`, `observationPoint`                 | `string`                                         |
| `labels`, `successLabels`                               | `labels`, a map of exact string equality filters |
| `thresholdSeconds`, `target`                            | `number`                                         |
| `scrapeIntervalSeconds`, `stepSeconds`, `windowSeconds` | `integer`                                        |

- These bindings are fixed field names, not expressions or a query language.
  - Values cannot supply raw PromQL, endpoints, credentials, tenant overrides, executable commands or arbitrary SLO mutations.
  - Measurement-specific compatibility, numeric bounds, label validity, histogram requirements and SLO rules still pass through core validation.
- Extension renders use `values.inputs`; direct `measurement`, `target`, `windowSeconds` and `stepSeconds` overrides are rejected so they cannot bypass the declared input schema.
  - Common name/project/source fields remain available. `--service` can supply a declared service input; conflicting input values are rejected.
  - Required inputs must have a value or declared default.
  - Unknown inputs and duplicate YAML/JSON keys are rejected.
- In the portal, choose **Create SLO → Measurement → Start from a template**. Installed recipes appear alongside built-ins.
  - The form displays typed extension inputs, requirements, package version and the evaluation contract. **Use template** renders through the server with the selected template digest, then fills the ordinary editable measurement form.
  - It does not save or deploy the SLO.

### Policies and evaluation contracts

- Policy presets can set target, rolling window, burn policies and measurement-quality defaults.
  - A recipe references at most one named preset.
  - Only explicitly declared preset fields replace the recipe defaults.
  - Every preset must be used by a validated recipe.
  - These are reviewed starting points, not mandatory tenant policy: the rendered SLO remains editable, and existing governance controls apply when it is planned or applied.
- Evaluation contracts declare a versioned rubric, sampling policy and pinned population labels identifying the rubric/evaluator. They require `unknownVerdict: not_good` and `missingVerdict: unknown`.
  - Evaluation recipes measure `verdict=pass` and cannot expose a parameter that redefines success labels.
  - Rendering must preserve the contract's pinned cohort labels.
  - For the included example, changing `rubric: support-v1` or `evaluator: rules-v1` requires a different contract release.
- These checks constrain the definition; they cannot establish that an external evaluator implemented its rubric correctly or emitted every selected verdict.
  - Missing evaluations need a separate coverage signal.
  - Bundles do not invoke judges, certify model quality or treat missing observations as passes. See [AI workload measurements](agent-slos.md).

### Conformance cases

- Each recipe must include at least one positive case with exact expected good/total selectors and one rejection case. Loading and installing a bundle always runs these deterministic cases; a failed case rejects the bundle. `nines extensions validate` and `nines extensions test` both validate the full contract and run its cases, making them suitable for an extension author's CI.
- The author supplies those expected selectors.
  - Passing self-tests establishes internal consistency, not independent trust in the publisher, instrumentation or business semantics.
  - Core measurement-quality checks and human review remain necessary.

## Install, distribute and upgrade

- Teams can publish YAML artifacts and checksums in their own repositories, release storage or HTTPS artifact registries. Obtain the expected digest from a source you trust, then install explicitly:

```sh
# Local artifact; the lockfile parent directory must already exist.
nines --offline extensions install --file your-extension.yaml \
  --sha256 YOUR_TRUSTED_64_CHARACTER_SHA256 --lock extensions.lock.yaml

# Explicit download during installation only.
nines extensions install --url https://extensions.example.com/releases/your-extension-1.0.0.yaml \
  --sha256 YOUR_TRUSTED_64_CHARACTER_SHA256 --lock extensions.lock.yaml

# Public Git repository; use a full commit ID, never a branch or tag.
nines extensions install --git https://github.com/your-team/slo-extensions.git \
  --commit FULL_LOWERCASE_COMMIT_ID --git-path bundles/your-extension.yaml \
  --sha256 YOUR_TRUSTED_64_CHARACTER_SHA256 --lock extensions.lock.yaml

# Existing local clone, including private repositories; works offline.
nines --offline extensions install --git ./slo-extensions \
  --commit FULL_LOWERCASE_COMMIT_ID --git-path bundles/your-extension.yaml \
  --sha256 YOUR_TRUSTED_64_CHARACTER_SHA256 --lock extensions.lock.yaml
```

- Installation stores immutable content-addressed YAML in `nines-extensions/` beside the lockfile, then atomically updates the lockfile.
  - Commit both to your repository for reproducible offline rendering.
  - The installer serializes cooperating CLI writers, detects external lock edits, refuses artifact corruption and cleans its temporary guard on normal completion.
  - After a process crash, inspect and remove only its stale `.installing`/`.pending` files before retrying.
- Replacing an installed package requires a different version, its new trusted digest and `--replace`. The same version cannot change content.
  - There are no floating tags or automatic updates.
  - HTTPS downloads enforce size/time limits, reject redirects and URL credentials/query strings, use system TLS trust and send no Nines or provider credentials. `--offline` forbids remote downloads and permits local Git repositories.

### Git repository installation

- Git installs require `git` on PATH, a full lowercase commit ID (40 characters for SHA-1 repositories or 64 for SHA-256), a repository-relative bundle path and the independently trusted bundle SHA-256.
  - The installer fetches that commit into a disposable bare repository, reads the committed blob and applies the same schema, conformance and immutable installation checks.
  - Branches, tags, abbreviated IDs, symlinks, submodules and parent traversal are rejected.
  - Uncommitted files and a newer repository HEAD do not change the selected bundle.
- The installation receipt includes `git.repository`, `git.commit` and `git.path`; retain it with review artifacts.
  - The lockfile pins the installed bytes by package version and SHA-256.
  - Git is never consulted when the CLI, server, MCP or operator renders from that lockfile, and already-rendered SLOs need neither the lock nor the publishing repository.
- Direct remote installation supports public HTTPS repositories.
  - It disables redirects, inherited Git configuration, credential helpers and prompts; it runs no checkout, hooks, filters or submodule initialization.
  - For private repositories or SSH access, authenticate and clone using your existing tooling, then pass that local clone to `--git`.
  - A local clone must already contain the commit's objects; implicit lazy fetching is disabled.
  - Git operations have a 45-second deadline and the selected bundle has a 1 MiB limit.
  - A shallow fetch can still transfer other files from that commit, so prefer dedicated, small template repositories; the limit is not a cap on repository transfer size.
  - Temporary repositories are removed after success or failure.
- SHA-256 pinning establishes byte identity, not publisher identity. `metadata.publisher` is informational.
  - Signature verification, OCI resolution, authenticated registry configuration and registry search are not implemented.
  - For a private registry, fetch using your existing trusted tooling, then install the verified local file.
  - Never put registry or model credentials in a bundle or values file.

## Server and tenant configuration

An administrator can load a shared local snapshot at startup:

```sh
nines-server -extension-lock /etc/nines/extensions.lock.yaml
```

Alternatively, assign private catalogs per tenant in the administrator-owned tenant configuration:

```yaml
tenants:
  - id: support
    name: Support
    sloExtensionLock: extensions/support.lock.yaml
    source:
      kind: prometheus
      url: http://prometheus:9090
    access:
      - tokenEnv: SUPPORT_NINES_API_KEY
        role: owner
```

- Relative tenant lock paths resolve against the tenant configuration file's directory.
  - Artifact paths resolve against the lockfile directory and must stay inside it.
  - Lock entries cannot traverse parents, reference absolute paths or follow symlinks.
  - Missing/corrupt artifacts, wrong versions/digests and unsupported contracts fail startup.
  - There is no fallback to a different package.
  - Shared and tenant-local locks cannot be combined in the same server configuration.
- Mount the lockfile and its artifact directory read-only into Compose/Kubernetes containers, preserving their relative paths.
  - The container user must be able to read the mounted files; installation creates private files/directories, so provision a read-only copy with suitable deployment ownership.
  - Restart the server after an explicit lock upgrade.
  - Existing processes retain their immutable snapshot until restarted.
  - The API has no extension installation or reload endpoint, and tenant users or agents cannot choose filesystem paths or registry URLs.

## Agentic intent preparation

- The durable intent worker can discover and render recipes from the same tenant catalog.
  - It pins the catalog and template digests, records extension provenance and verifies the proposed measurement before a human confirms it.
  - This authoring path adopts the measurement while preserving the typed goal's numeric commitments and policies; it does not silently import a package's policy presets.
  - No extension installation, approval, apply or deployment tool is added to the model. See [templates in intent conversations](slo-templates.md#templates-in-intent-conversations).

## MCP, Go clients and Kubernetes

- The existing template API/Go client exposes installed recipe schemas and provenance.
  - MCP tools `list_slo_templates` and `render_slo_template` use the configured tenant catalog.
  - An extension render requires `expectedTemplateDigest` from that selected catalog entry; stale digests return a conflict.
  - Rendering adds no write, approval, deployment or network authority.
  - Extension descriptions and rubric text are untrusted data, not agent instructions.

Render directly for GitOps using the same compiler:

```sh
nines --offline operator render --template example-agent--evaluation-pass \
  --extension-lock examples/extensions/example.lock.yaml \
  --values examples/extensions/support-values.yaml --namespace team-support

# Or render reviewed recording rules as a PrometheusRule:
nines --offline operator render --template example-agent--evaluation-pass \
  --extension-lock examples/extensions/example.lock.yaml \
  --values examples/extensions/support-values.yaml --namespace team-support \
  --tenant support --rules-only
```

- The controller reconciles the resulting self-contained SLO or PrometheusRule through its existing configured destinations.
  - It does not download, load or execute extensions during reconciliation.
  - Removing a bundle or losing access to its publishing registry does not stop already-rendered SLO evaluation.
- The contract currently supports the built-in event-ratio and classic-histogram primitives for Prometheus/Mimir. Arbitrary query generators, rule-rewriting processors and new native telemetry providers require separate capability work; they cannot be introduced through undeclared hooks in a bundle.
