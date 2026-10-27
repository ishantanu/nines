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
