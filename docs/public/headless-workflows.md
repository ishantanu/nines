# Workflows without a dashboard

- Use Nines from CI, an incident tool, a terminal or an agent through the CLI, HTTP API and MCP.
- The dashboard is optional for intent preparation, human review, application, runtime inspection and configuration rollback.
- An agent can prepare a recommendation and collect saved evidence. It cannot approve its own proposal or grant human consent.
- Capability parity preserves role boundaries: human approval/application remain CLI/API operations; MCP intentionally exposes preparation and read-only review evidence.

## Resume a saved workflow

```sh
nines auth status --context production --agent
nines intent conversations status CONVERSATION --context production --agent
```

- HTTP: `GET /api/intent-conversations/{id}/status`.
- MCP: `get_intent_workflow_status` with `{ "id": "CONVERSATION" }`. Available in read-only mode.
- Go: `Client.IntentWorkflowStatus(ctx, id)`.
- The response includes tenant, conversation, version, phase, next action, human-review requirement, exact change-set/rollback digests and the saved observation outcome when available.
- Status reads do not invoke a model, query telemetry, start work or mutate a definition. Reading a status does not authorize its suggested next action.
- Treat `retryAt` as a scheduling hint, not a guarantee. Poll no faster than every two seconds and back off when nothing changes. For `wait`, read status again; there is no `wait` mutation command.
- Status summarizes the current change set. Use `show` and `audit` to inspect retained change-set history; supported rollback requests can also target a historical applied digest.
- Saved observations describe their recorded period. Read `verify` for current resource/source drift before relying on a previously verified outcome.

| Phase | Next step | Review boundary |
| --- | --- | --- |
| `draft` | `run` | Explicit model consent when using a model |
| `preparing` | Wait, then read status | Do not start duplicate runs |
| `clarification` | `show`, then `message` | Ask the responsible person for missing semantics |
| `measurement_review` | `show`, then `confirm` | Human confirms the exact measurement and intent |
| `proposal_ready` | `changes` | Freeze evidence and changes into a reviewable digest |
| `approval_required` | `show`, then `approve` | Human reviews changes, rationale, evidence and limitations |
| `approved` | `apply` | Authorized applier must differ from approver |
| `observing` | Wait, then read status | Background observer checks runtime; owner/editor can request `observe` |
| `verified` | `verify` | Saved observation met its policy; inspect current drift |
| `outcome_review` | `verify` and `show` | Human decides corrective action; no automatic rollback |
| `rollback_approval_required` | `rollback-approve` | Human reviews exact restore diff |
| `rollback_approved` | `rollback-apply` | Authorized applier must differ from rollback approver |
| `rollback_blocked` | `show` | Resolve blockers; do not bypass them |
| `rollback_rejected` | `show` | Review the rejection and current evidence |
| `outcome_unavailable` | `verify` | No observation was recorded; inspect runtime evidence separately |
| `proposal_stopped` / `preparation_stopped` / `review_required` | `show` | Inspect evidence before starting replacement work |

## Prepare and review

- See [intent authoring](intents.md) for typed goals, inline measurement confirmation and explicit provider consent.
- Preparation is durable. Save the conversation ID and return later with `status` or `show`; a browser or chat session need not stay open.
- Use stable request IDs for create/run retries. A changed request needs a deliberate new operation, not a loop generating IDs.
- After preparation reaches `proposal_ready`, freeze the reviewed proposal:

```sh
# freeze.yaml: expectedVersion: CURRENT_VERSION
nines intent conversations changes CONVERSATION --context production -f freeze.yaml --agent
nines intent conversations show CONVERSATION --context production --agent
```

- Export a complete audit-verified snapshot for human review:

```sh
nines intent conversations report CONVERSATION --context production --agent > review.json
nines intent conversations report CONVERSATION --context production --markdown > review.md
```

- These artifacts contain exact changes, recommendation basis, approval and rollback state, a bounded complete audit and any recorded observation. Treat definitions and review notes as sensitive; share them only with authorized recipients.
- A concurrent change or invalid audit causes export to fail; read the current status and review a fresh artifact. The snapshot itself never approves a change. Use `get_intent_review_artifact` through read-only MCP, `GET /api/intent-conversations/{id}/review-artifact`, or the Go client's `IntentReviewArtifact`. The Markdown format encloses structured JSON in a safe code fence, including user-provided text.
- Record human approval only after reviewing that artifact. The operational request is YAML:

```yaml
expectedVersion: 12 # Read the current conversation version.
digest: EXACT_REVIEWED_CHANGE_SET_DIGEST
confirm: true
reason: Reviewed eligible events, success criteria, target impact and rollback limitations.
```

```sh
# Human approver's existing context:
nines intent conversations approve CONVERSATION --context production-reviewer -f approval.yaml --agent
# A separately authorized release engineer reads the new version and applies:
nines intent conversations apply CONVERSATION --context production-applier -f apply.yaml --agent
```

- `apply.yaml` supplies the newly read `expectedVersion`, the same reviewed `digest` and `confirm: true`.
- Never acquire or substitute another identity just to defeat independent review. The second context represents a real, independently authorized person.
- Approval binds to the frozen change/evidence contract. Edited proposals, stale revisions or changed sources require fresh preparation/review. Apply also repeats runtime safety checks.
- A committed definition is not proof of rule activation, successful evaluation or healthy SLI. Configured automatic rule delivery and the Kubernetes operator have their own reconciliation/verification boundaries.

## Observe and restore

```sh
nines intent conversations status CONVERSATION --context production --agent
nines intent conversations verify CONVERSATION --context production --agent
# Owner/editor can explicitly refresh bounded runtime evidence:
nines intent conversations observe CONVERSATION --context production -f observe.yaml --agent
```

- `observe.yaml` contains the latest `expectedVersion`, applied operation `digest` and `confirm: true`. Use the rollback digest after restoration.
- Background observation can advance the conversation version. Read it immediately before any mutation; handle conflicts by fetching and reviewing the current state.
- Missing telemetry or unverified runtime remains pending or insufficient evidence. It never becomes a successful result merely because an API call returned HTTP 200.
- If restoration is appropriate, prepare and inspect it before approving:

```sh
nines intent conversations rollback-plan CONVERSATION --context production-reviewer -f restore-plan.yaml --agent
nines intent conversations show CONVERSATION --context production-reviewer --agent
nines intent conversations rollback-approve CONVERSATION --context production-reviewer -f restore-approval.yaml --agent
nines intent conversations rollback-apply CONVERSATION --context production-applier -f restore-apply.yaml --agent
nines intent conversations verify CONVERSATION --context production --agent
nines intent conversations audit CONVERSATION --context production --agent
```

- Every mutation request pins the current version and exact relevant digest, with `confirm: true`. Restore planning references the original change-set digest and needs a reason; restore approval/application reference the returned rollback digest.
- Rollback restores supported previous configurations as new revisions. It does not recover spent error budget, reverse service deployments, remove newly created resources by implication or certify restored runtime.
- Audit export is paginated: preserve `through` and follow `nextAfter` using `audit --after N --through N` until no next page remains.
- See [approval, verification and rollback](intent-governance.md) for complete safety and audit contracts.

## Add a review to a CI job

For an existing authorized CI job with the Nines CLI installed, a tenant-scoped read credential and a conversation ID supplied through the environment, GitHub Actions can publish a review to its job summary:

```yaml
- name: Add Nines review to job summary
  env:
    NINES_API_KEY: ${{ secrets.NINES_API_KEY }}
    CONVERSATION: ${{ vars.NINES_CONVERSATION_ID }}
  run: nines intent conversations report "$CONVERSATION" --context production --markdown >> "$GITHUB_STEP_SUMMARY"
```

The named CLI context must already specify the correct server and tenant. Protect the job summary and credentials as sensitive. Set the conversation ID from the workflow that prepared the change; do not grant this job an approval identity. The same report can be stored as an access-controlled artifact by other CI systems. Exports over 3 MiB or 4,096 audit events fail rather than silently omit evidence; use the paginated audit API for larger histories.

## Automated acceptance

```sh
go test -race ./cmd/nines -run TestHeadlessCLIReviewedChangeAndRollback -count=1
```

- Runs the real Cobra commands, Go client, tenant HTTP middleware, workflow, deterministic compiler and repositories against an isolated loopback server.
- Uses synthetic telemetry and fixture setup for the existing definition; no browser, paid model, live backend or production credentials are involved.
- Covers preparation, measurement review, agent approval denial, independent application, wrong-digest denial, idempotent apply/rollback, runtime inspection, reviewed restoration and audit integrity.
- Deliberately verifies that absent external rule/runtime evidence does not produce a successful runtime outcome. Live backend activation remains covered by the separate Prometheus/Mimir integration suites.

## Follow-up integrations

- This change supplies a resumable status contract and headless lifecycle acceptance.
- Use [GitHub reviews](github-reviews.md) to plan and explicitly publish an exact proposal snapshot for a selected PR commit. GitHub reviews do not record Nines approval.
- Configure [signed workflow webhooks](workflow-webhooks.md) to notify an internal CI or incident integration. Automatic PR dispatch, native chat approval adapters and a generic subscription API remain separate work.
