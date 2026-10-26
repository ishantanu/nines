# SLI menus and SLO templates

Choose **Create SLO → Measurement → Start from a template** in the portal. Select a category and SLI, inspect its eligible and good events, then choose **Use template**. This replaces the unsaved measurement settings. Adapt the metric, labels, observation point, target and window, then use **Verify telemetry and plan SLO** before reviewing and applying.

Built-in templates ship with Nines and work without installation or a lockfile. Administrators can extend the same catalog with [versioned SLO extensions](slo-extensions.md), including typed recipe inputs, evaluation contracts and policy presets.

The shipped catalog has six categories:

| Category | SLIs |
| --- | --- |
| Request serving | Request availability and latency |
| Pipelines | Record correctness and delivery freshness |
| Storage | Operation success and sampled integrity checks |
| Agents | Logical task completion, tool-attempt success, task budget compliance and outcome evaluation |
| LLM applications | Usable response success and output evaluation |
| Inference serving | Request availability, duration and conditional time to first token |

Templates are starting points, not discovered telemetry. Metric names, label values, 99% targets, thresholds and rolling windows are illustrative. Built-in template drafts use `lifecycle: experimental` and no alert policies. Installed extensions can include reviewed policy presets; experimental lifecycle suppresses their paging until explicitly promoted. They can produce recording rules, but do not introduce a paging commitment. Set justified targets, add reviewed burn policies and promote the lifecycle when your measurement is ready.

## Use the CLI without a server

```sh
nines --offline templates list
nines --offline templates list --category agent
nines --offline templates show agent-task-completion
nines --offline templates generate agent-task-completion \
  --name support-completion --service support-agent --source production \
  --out support-completion.slo.yaml
nines --offline validate -f support-completion.slo.yaml
nines --offline rules generate -f support-completion.slo.yaml --tenant support
```

`generate` prints native `nines.dev/v1alpha1` ServiceLevelObjective YAML by default. `--out` writes a new file and refuses to overwrite. `--agent` emits a structured draft receipt containing the manifest, YAML, template ID/revision/digest, manifest digest, warnings and `telemetryVerified: false`. `--output yaml` emits that receipt as YAML; omit it for bare SLO YAML.

Use a values file to set targets and replace the example measurement. These are command inputs, not another persisted resource or DSL:

```yaml
name: support-completion
project: customer-support
service: support-agent
source: production
target: 0.98
windowSeconds: 604800
stepSeconds: 60
measurement:
  type: availability
  metric: support_agent_tasks_total
  labels:
    service: support-agent
    environment: production
  successLabels:
    outcome: success
  countFrom: counter
  scrapeIntervalSeconds: 15
  observationPoint: One terminal outcome per logical task after internal retries, including failures, cancellations and timeouts.
```

```sh
nines --offline --agent templates generate agent-task-completion \
  --values task-values.yaml
```

A supplied `measurement` replaces the complete example measurement. Name/project/service/source flags override corresponding values-file fields. When only `--service` is supplied, the example service population label follows it; a complete supplied measurement retains its explicit label filters.

## Connected clients and agents

The server exposes tenant-authenticated reads and deterministic rendering:

- `GET /api/slo-templates?category=agent`
- `GET /api/slo-templates/agent-task-completion`
- `POST /api/slo-templates/agent-task-completion/render` with template values as JSON.

Viewer and agent roles can render drafts without write permissions. Go clients provide `SLOTemplates`, `SLOTemplate` and `RenderSLOTemplate`. The read-only MCP tools are `list_slo_templates` and `render_slo_template`; tool calls cannot override tenant, endpoints, credentials or deployment destinations. Neither rendering nor choosing a template calls a model, verifies telemetry, saves an SLO or deploys rules.

The built-in offline catalog belongs to the installed CLI version, extended by its explicit `--extension-lock`. The connected catalog belongs to the server version and administrator-installed tenant extensions. Compare template digests when reproducing a draft across versions. Templates have their own revision and content digest; subsequent SLO edits use the existing SLO revision history. Save the draft receipt with review artifacts if you need template provenance; rendering alone does not persist it in SLO history.

## Templates in intent conversations

Model-assisted intent preparation can discover built-ins and installed tenant extensions with `list_slo_templates`, then propose an inline measurement with `render_slo_template`. Start with a typed outcome, target, rolling window and pinned scope, or let the drafting adapter extract these from explicit user text. The model uses the selected template digest and typed values; it cannot install extensions, switch tenant/source/population, overwrite an existing measurement or confirm its own choice.

Only the proposed measurement is adopted. Template targets, rolling windows, quality presets and burn policies do not replace the typed goal. When step or lifecycle is unspecified, the draft uses the recipe's validated step and experimental lifecycle. Explicit goal commitments remain unchanged. The ordinary template renderer and operator renderer continue to render complete recipe presets for direct authoring.

The worker verifies telemetry through `find_measurements` and pauses for an owner/editor to confirm the exact measurement digest. After confirmation, deterministic compilation, optional historical validation, independent approval and reviewed apply follow the existing flow. Guided preparation remains deterministic and does not automatically select a recipe.

The conversation shows the template ID/revision/digest and extension version/artifact digest when applicable. Selection is recorded in the audit journal and frozen into recommendation evidence. Manual goal edits clear the active template attribution and confirmation; earlier run receipts and reviewed proposals remain available. A catalog digest is pinned per model run. Catalog changes stop unfinished runs with `template_catalog_changed`; an already-rendered and reviewed definition has no registry dependency.

Discovery is paginated and bounded. Oversized recipe schemas require the manual measurement editor. Recipe descriptions, rubrics and illustrative metrics are untrusted data, not evidence of instrumentation or instructions to change tools. The worker retains its existing six-model-call and six-tool-call budgets. Synthetic workflow evaluations cover this route across provider protocols. Run `make selection-eval` for template-selection contracts and use the [live acceptance guide](intent-evaluations.md#template-selection-acceptance) for your exact model. The tested local Qwen 0.6B configuration failed the nine-case live corpus; passing server guardrails alone does not establish model suitability.

Read [agent, LLM and inference SLOs](agent-slos.md), [measurement quality](measurement-quality.md) and [model evaluations](intent-evaluations.md).
