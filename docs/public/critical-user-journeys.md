# Critical user journeys

A critical user journey describes an outcome a person needs from your product, such as completing checkout.

- Its steps link to the SLOs that measure the participating services.
- This makes the relationship between service reliability and user impact explicit.

## Define a journey in the portal

- Open **Critical user journeys → Define journey**. The **Definition** tab uses the same guided edit and review flow as SLO definitions:

1. Enter an identifier, project, display name, owner, criticality and user outcome.
2. Link existing SLOs from the current tenant. Choose each link's dimension and whether it is required for journey health. The SLO's own measurement, target and alert policies remain in its definition.
3. Add the journey steps, choose the actor for each step and select its SLOs. Add dependencies to describe ordering, including parallel branches. Every linked SLO must appear in a step; dependencies cannot form a cycle.
4. Optionally enable an agent contract with a success definition, duration/cost/step limits and approval-evidence requirement. These evaluate reported evidence; they do not execute an agent or enforce approval.
5. Select **Validate and plan**, inspect the resulting YAML and revision, then **Save reviewed journey**. Agent credentials submit a proposal for owner review instead of applying directly.

- Use **Edit definition** on an existing journey to create a reviewed revision under the same identifier.
  - The **YAML** tab edits the same resource; valid edits carry across tabs.
  - Any edit invalidates the previous plan, and a concurrent server revision requires a fresh plan.
  - If the tenant has no SLOs, define one in **Service level objectives** before linking it.

## Journey YAML

- Journey definitions use the same `apiVersion` as SLOs. `spec.slos` declares the linked SLOs, their dimensions and whether each is required. Each step refers to those links through `sloRefs`; `dependsOn` describes the order between steps.

<!-- example: examples/checkout.journey.yaml -->

- [Download the complete journey](../../examples/checkout.journey.yaml).
- The example links cart submission to API availability, payment authorization to checkout correctness, and confirmation to an advisory background-job SLO. Replace the example IDs with SLOs in the intended tenant and project.

## Required and advisory links

- Use `required: true` for SLOs that are necessary to explain the outcome. Use advisory links for supporting evidence. A journey link does not change an SLO's target, arithmetic, measurement population or alert policy.
- A required link with missing evidence cannot establish journey health. Declared dependencies and observed service-graph edges remain separate evidence; topology alone does not establish causality or successful end-to-end execution.

## Measure an agentic journey from its runs

An agentic journey records runs, each with a verdict against the journey's contract. In the portal, tick **Measure judged runs with an objective** in the journey form's agent contract, or add `spec.runObjective` to turn those runs into an objective.

```yaml
spec:
  agentic:
    successDefinition: The agent resolves the request within its limits.
    approvalRequired: true
  runObjective:
    target: 0.9
    windowSeconds: 604800
    minRuns: 20
```

- `target` is the share of judged runs that must pass. `windowSeconds` is how far back runs count, from one hour to 31 days. `minRuns` is the fewest judged runs before the objective is read.
- A **passed** run is good, and **failed** and **violated** runs are bad. **Incomplete** and **inconclusive** runs are unjudged and are never counted as success, so they cannot rescue a result.
- Only runs recorded against the journey's current contract count. After a revision, older runs are listed as another contract and excluded, so evidence is always bound to the definition it was judged against.
- With too few judged runs the objective is not scored and the journey's health is **unknown**, not healthy. When the objective is met health is unaffected, and when it is missed health becomes **degraded** with the reason stated. A journey without `runObjective` behaves exactly as before.
- It requires an agentic contract, because only agentic journeys record runs, and it reads at most the latest 200 runs; the result says when a window may hold more.
- Nines does not run journeys or confirm that runs were representative. The runs are evidence posted by whatever ran the journey, so the objective is only as good as that evidence.

## Review and apply

```sh
nines validate --offline -f examples/checkout.journey.yaml
nines plan -f examples/checkout.journey.yaml --context production --agent
nines apply -f examples/checkout.journey.yaml --expected-revision 0 --context production --agent
```

- Zero creates a resource.
  - For an edit, read the current revision and review a fresh plan before applying.
  - Cross-tenant SLO IDs do not grant access to another tenant's resources.

## Propose links through an intent

- An [SLO intent](intents.md) can propose a link to an existing journey and step.
  - It must carry the journey's expected revision.
  - The reviewed change set applies the SLO and the proposed journey change atomically.
  - The model cannot approve that change, and a concurrent journey edit invalidates the stale proposal.
- See [approval and audit](intent-governance.md), [SLO editing](slo-editing.md) and [architecture](architecture.md).

## Explore a journey in the portal

- Choose a critical user journey from the selector above its definition. The selector does not reserve a permanent column beside the map.
- Collapse the desktop navigation using its header button. The icon rail retains accessible labels and hover/focus tooltips, and remembers the preference in this browser. Mobile navigation is unchanged.
- The map fits its available viewport. Use **Fit journey**, zoom controls, the percentage button to reset to 100%, or **Expand journey** for a larger dialog. Scroll inside the canvas or drag empty space to pan; touch scrolling remains native.
- Map edges represent explicit step dependencies. Stages are dependency layers, not additional saved resources. Steps in the same stage may run in parallel.
- Select a step to open its actor, dependencies and linked SLO measurements in a closable panel. Step health summarizes its referenced SLOs; it does not measure end-to-end journey success. Unavailable or unlinked measurements stay unknown.
- **List** provides an ordered, keyboard-accessible view with collapsible dependency stages. **Linked objectives** below the canvas expands into the full SLO comparison table.

These controls change the view only. They do not edit definitions, collect telemetry or publish provider resources.
