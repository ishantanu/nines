# Critical user journeys

A critical user journey describes an outcome a person needs from your product, such as completing checkout. Its steps link to the SLOs that measure the participating services. This makes the relationship between service reliability and user impact explicit.

## Define a journey

Journey definitions use the same `apiVersion` as SLOs. `spec.slos` declares the linked SLOs, their dimensions and whether each is required. Each step refers to those links through `sloRefs`; `dependsOn` describes the order between steps.

<!-- example: examples/checkout.journey.yaml -->

[Download the complete journey](../../examples/checkout.journey.yaml).

The example links cart submission to API availability, payment authorization to checkout correctness, and confirmation to an advisory background-job SLO. Replace the example IDs with SLOs in the intended tenant and project.

## Required and advisory links

Use `required: true` for SLOs that are necessary to explain the outcome. Use advisory links for supporting evidence. A journey link does not change an SLO's target, arithmetic, measurement population or alert policy.

A required link with missing evidence cannot establish journey health. Declared dependencies and observed service-graph edges remain separate evidence; topology alone does not establish causality or successful end-to-end execution.

## Review and apply

```sh
nines validate --offline -f examples/checkout.journey.yaml
nines plan -f examples/checkout.journey.yaml --context production --agent
nines apply -f examples/checkout.journey.yaml --expected-revision 0 --context production --agent
```

Zero creates a resource. For an edit, read the current revision and review a fresh plan before applying. Cross-tenant SLO IDs do not grant access to another tenant's resources.

## Propose links through an intent

An [SLO intent](intents.md) can propose a link to an existing journey and step. It must carry the journey's expected revision. The reviewed change set applies the SLO and the proposed journey change atomically. The model cannot approve that change, and a concurrent journey edit invalidates the stale proposal.

See [approval and audit](intent-governance.md), [SLO editing](slo-editing.md) and [architecture](architecture.md).
