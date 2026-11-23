# Error-budget response policies

An error-budget policy states who responds and when an operational change needs review.

- It belongs to the SLO and is versioned with its definition.
- Nines computes a decision from one SLO revision and measurement cutoff; it does not change the target, erase consumed budget or automatically freeze an external deployment system.

## Configure the policy

Add this optional section to an existing SLO or intent, or use **Add error-budget policy** in the portal:

```yaml
spec:
  errorBudgetPolicy:
    owner: payments-oncall
    runbookURL: https://runbooks.example.com/payments
    reviewBelow: 0.25
    onExhausted: blocked
```

- The owner and runbook are required.
  - The runbook accepts HTTP(S) without embedded credentials. `reviewBelow` is a fraction of the allowed error budget remaining, between zero and one.
  - It is not the SLI attainment percentage. `onExhausted` accepts `blocked` (also the omitted default) or `review_required`; exhaustion cannot be configured to permit proceeding.
- Policies survive intent compilation, canonical exports, revision archives and Kubernetes admission.
  - Editing one preserves the SLO's measurement samples.
  - The operator retains and validates the policy while reconciling rules; the Nines server computes these operational decisions. No separate policy resource is needed.

## Decisions

| Decision                | Meaning                                                                                                                                |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `not_configured`        | There is no response policy. No permission is implied.                                                                                 |
| `insufficient_evidence` | Coverage, freshness, counts, collection or measurement verification cannot support a decision.                                         |
| `review_required`       | Remaining budget is at or below the review threshold, the configured exhaustion response requires review, or the SLO is not committed. |
| `blocked`               | The committed SLO's budget is exhausted and its response is blocked.                                                                   |
| `proceed`               | Current quantitative evidence meets the configured policy. Normal change approval and deployment authorization still apply.            |

- Source changes and unverified collector lineage also prevent proceeding until the complete window has fresh source evidence.
- Evidence checks take precedence over a seemingly healthy retained percentage.
  - Partial windows, stale data, zero traffic, unmet quality count requirements and unverified structured measurements never yield `proceed`.
  - Historical context is descriptive and cannot authorize a current action.
  - Custom/imported counts retain their explicit metadata-verification limitations.
- The portal shows the owner, runbook, remaining budget, revision and cutoff.
  - API, Go client, CLI and the read-only `slo_budget_decision` MCP tool return the same decision with definition, source and policy digests.
  - The policy does not silence burn alerts or reclassify a measured violation.

## Agree responses for each budget level

`bands` state the response the owner has agreed as the remaining budget falls. Each band has a `name`, a `below` fraction and a `response` of up to 300 characters, and a policy holds at most five.

```yaml
errorBudgetPolicy:
  owner: payments-oncall
  runbookURL: https://example.com/runbooks/api
  reviewBelow: 0.25
  bands:
    - name: watch
      below: 0.5
      response: Prioritize reliability bugs over feature work.
    - name: protect
      below: 0.25
      response: Pause risky launches until the owner reviews.
```

- The decision carries the most severe band that applies, meaning the lowest `below` at or above the remaining budget. An exhausted or overspent budget matches the lowest band. The portal shows it as a response level.
- Bands describe the agreed response. They never change the decision state, which still comes from `reviewBelow` and `onExhausted`, and they grant or block nothing.
- No band is shown when evidence is insufficient, the objective is not committed, or a historical view is read, because no decision is made then.
- Rolling back a definition cannot restore budget that has been spent. A band reflects the budget actually consumed in the current window.
- Windows are rolling. A calendar-aligned refresh is not supported.

## Require a postmortem

Set `postmortemAbove` to the share of the error budget a single incident may consume before a postmortem is required. The SRE workbook suggests 0.2: one incident spending more than a fifth of the budget deserves one.

```yaml
spec:
  errorBudgetPolicy:
    owner: team-commerce
    runbookURL: https://runbooks.example/checkout
    reviewBelow: 0.25
    postmortemAbove: 0.2
```

- **The rule.** A postmortem is required when an [incident](incidents.md) consumed **more** than this share of the SLO's budget. At exactly the threshold, none is required. Zero, or leaving the field out, turns the rule off.
- **How consumption is measured.** It is the incident's impact: bad events from when impact started until the alert cleared or the incident was mitigated, over the budget the SLO's window allowed. An incident longer than the window can consume more than the whole budget.
- **Unknown stays unknown.** When the impact is unknown, because data is missing or the SLO's definition changed after the incident, the requirement is reported as unknown, never as not required.
- **Where it shows.** The incident says a postmortem is required and why. The SLO's budget decision lists incidents still awaiting a published postmortem. Publishing one on the incident's review clears it.
- **Advice, not enforcement.** Like response bands, the requirement never changes the decision state or blocks anything.
- **Availability.** It needs incidents and reliability reviews turned on (`NINES_FEATURES=incidents,reliability-reviews`).

## Enforce in CI

```sh
export NINES_URL=https://nines.example.com
export NINES_TENANT=payments
# Supply NINES_API_KEY through the CI secret store.
nines budget api --check
```

- Without `--check`, the command prints the decision for inspection.
  - With it, only a scoped, configured `proceed` decision succeeds.
  - Other states, unknown states and missing or mismatched decision identity fail with exit code 7.
  - HTTP, authentication and revision errors retain their existing CLI exit codes.
  - JSON output remains available on stdout; the failure is reported separately.
- Use `--expected-revision N` when the pipeline must evaluate a specific current definition.
  - Decisions are point-in-time evidence; read again near the action.
  - This command grants no permission and performs no deployment.
  - Your pipeline decides which actions it gates.
  - Preserve paths for incident response and corrective work in the runbook.

## Agent proposals and audit

- An update proposal freezes the existing SLO's budget decision in its recommendation report before approval.
  - The saved cutoff, policy digest and revision explain the evidence available during review.
  - Changes to quality requirements or the budget policy appear explicitly in the field diff and are included in the proposal digest and existing decision journal.
- The frozen decision does not remain a current release decision. Read fresh evidence before release.
  - Existing independent human approval, authorization and rollback reviews remain necessary; the MCP budget tool cannot change policies or bypass them.
  - Restoring configuration cannot recover an already consumed error budget.
- `GET /api/slos/{id}/budget` returns the current decision; optional `expectedRevision` rejects stale reads. See [measurement quality](measurement-quality.md) and [intent governance](intent-governance.md).

## Applying reviewed agent changes

- The reviewed intent apply path now enforces current quality and budget evidence inside its definition transaction.
  - An expired review-time report cannot authorize apply.
  - Corrective work needs a separately approved, short-lived owner exception; independently approved rollback retains an explicit recovery path.
  - The post-change observer records rule runtime and SLI outcomes without mutating resources. See [action-time safety and observation](intent-governance.md#action-time-safety-checks).
