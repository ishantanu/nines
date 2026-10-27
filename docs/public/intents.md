# SLO intents

An intent expresses the reliability outcome you want to measure.

- Nines can help turn natural language into a draft, discover relevant telemetry and prepare a change set.
- The deterministic compiler and explicit human review determine what can be applied.
- Model-assisted drafting and tool-driven preparation remain experimental. The guided path and offline CLI do not require a model.
- Intent YAML can set `spec.lifecycle` and policy `minEvents`.
  - The compiler preserves them in the SLO and its intent provenance.
  - Review shows lifecycle changes and their paging impact.
  - Agent context includes lifecycle and suppression separately from measured reliability; a disabled alert is not evidence that an objective is healthy. See [lifecycle and traffic guards](measurements.md#lifecycle-and-alert-eligibility).

## Define a goal and measurement

- An intent includes the service, source, target, window and SLO-owned measurement. These are reviewable inputs, not values a model may silently change during deployment.

<!-- example: examples/api-success.intent.yaml -->

- [Download the complete intent](../../examples/api-success.intent.yaml).

## Follow the review flow

1. **Draft the goal.** Use the portal or author intent YAML. State the user outcome and the observation point, including excluded traffic and retries.
2. **Verify telemetry.** Model mode can first propose a measurement from the tenant’s [template catalog](slo-templates.md#templates-in-intent-conversations). The recipe and digest are recorded for review; its examples are not telemetry evidence. Discover the metric and matching populations through the tenant's configured source. Missing or ambiguous evidence requires clarification.
3. **Confirm the measurement.** An owner/editor confirms the exact goal and measurement digests. A model or agent role cannot perform this confirmation.
4. **Prepare the changes.** Inspect the proposed SLO, optional critical user journey link, parameter diff, rationale, assumptions and available backtest evidence.
5. **Approve the exact change set.** Record a reason and review its limits. Edits to the goal, measurement, source or affected revisions invalidate the previous approval.
6. **Apply and verify.** The SLO and proposed journey change commit atomically. Rule delivery then follows the telemetry connection's automatic or manual policy. Check measurement coverage, backend evaluation and output separately.

- A successful model response is not permission to write production configuration. The workflow model has no approval or direct resource-write tool.

## Choose a model provider

- Model configuration is optional and provider-specific.
  - Nines supports native Anthropic and other documented provider routes, compatible endpoints and local model configurations.
  - You do not need an OpenAI account to use the deterministic platform or a different supported adapter.
- Configure provider credentials in the server environment or secret manager.
  - Do not put keys in intent YAML, prompts, project files or exported evidence.
  - See [provider setup and measured acceptance](intent-evaluations.md) for exact configuration and current limits; drafting support does not imply full tool-calling workflow support.

## Optional semantic advice

- [System One advice with Jev](system-one.md) suggests a template category and assesses whether the description defines eligible events, success and the observation point.
  - It works in guided or model-directed preparation with separate consent.
  - Results are advisory probabilities, never telemetry evidence or permission to apply a change.

## Understand rollback

- Existing definitions can be restored as new revisions through a separately reviewed rollback.
  - Newly created resources have no previous definition and block this restore path.
  - Applying a restore cannot undo consumed error budget or recover missing historical telemetry.
- See [recommendation governance](intent-governance.md) for approval withdrawal, audit export and revision-safe rollback, and [MCP and skills](mcp.md) for agent access.
