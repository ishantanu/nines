# Recommendation review, audit and rollback

Intent workflows prepare proposals. They do not authorize production changes. The server requires an owner or editor to approve the exact change-set digest before applying definitions. The agent role cannot confirm measurement semantics, approve, reject, apply or roll back definitions. The workflow model has no approval or resource-write tool.

## Understand a recommendation before approving it

Use **SLO intents**, open a prepared conversation and select **Review changes**. The review freezes the following into the change-set digest:

- The goal, target, window, selected measurement revision and source fingerprint.
- Before-and-after target, window, alert policy and critical user journey changes, with explanations of their implications.
- Metric discovery interval and evidence digest, plus a matching historical backtest summary when one was actually run. Missing historical validation is stated explicitly.
- The preparation run, model route and skill version/digest. A guided run is identified separately from a model-assisted run.
- The visible preparation summary, declared basis, assumptions and limitations. This records the visible supporting evidence and explicit limitations; it does not guarantee that the recommendation is correct.

The definition form is a read-only preview. **Continue to review and approval** starts the conversation workflow, where the measurement is confirmed again and the same audited change-set approval applies. It no longer applies the SLO and journey independently.

The review also states whether the proposal creates a resource with no rollback baseline, changes restorable definitions, or matches the current configuration. Target diffs show the change in allowed bad-event fraction before approval.

The target and policies follow the reviewed goal. Nines does not infer acceptable business risk, certify telemetry completeness or guarantee that a different target prevents an incident. Review excluded traffic and observation points, not just whether the metric exists. A higher target reduces the error budget for the same traffic. Changing the rolling window changes the evaluation horizon and required coverage.

Changing the goal, measurement selection or source invalidates the existing review. Old proposals remain in saved history. A changed model/skill route fences unfinished preparation runs rather than silently continuing under a different execution contract.

## Approve, reject or withdraw

The portal asks for a reviewer note and acknowledgement of the rationale, measurements, settings and limitations. Approval records the reviewer, time, note and exact digest. **Apply approved changes** checks conversation, resource, source and measurement revisions again. The SLO and explicitly proposed critical user journey link commit together or neither does.

A reviewer can reject a proposal or withdraw an approval before apply, with a reason. A rejected proposal cannot be applied. Preparing a new review creates a new change-set identity; old approval is not reused. Review and apply are separate operations, but this alpha does not enforce a two-person approval policy. The same authorized owner/editor may perform both.

The API's optional `reason` field preserves older approval clients. The portal requires a note; an older programmatic approval without one is recorded as **No reviewer note supplied by this client.** New rejection and rollback-review operations require a reason. Use explicit notes in production automation. Use individual SSO accounts for personal attribution; a shared API credential identifies that credential rather than the engineer using it.

Permissions come from the authenticated server role. CLI `--agent` selects deterministic machine output; it does not downgrade an owner credential. Give automated assistants the server's `agent` role rather than an owner/editor credential.

Applying definitions does not deploy external recording or alert rules. Review and apply the destination separately in the SLO's **Rules** tab or with the rules CLI. A stored definition or receipt does not prove rule evaluation, SLO attainment or notification delivery.

## Audit records

The conversation's **Decision audit trail** identifies proposal, approval, rejection, application and rollback events, including authenticated actors, timestamps, conversation versions, exact digests, outcomes and review reasons. Authenticated governance denials and admitted conflicts are recorded without request bodies or raw provider errors. Invalid requests rejected before routing and infrastructure failures may only appear in operational logs.

**Download audit JSON** exports the decision events and frozen change-set definitions/recommendations. The server verifies the event sequence/hash chain and proposal/rollback digests. A failed integrity check blocks further change-control mutations until the records are inspected. The export reports `integrity: invalid` rather than concealing a detected mismatch.

Successful definition transactions include approval identity, reason, operation digest and recommendation digest in their database receipt. If resource commit succeeds but conversation persistence fails, an explicit retry reads the receipt without applying twice and preserves the original applying identity. Inspect readback and retry the exact request after an ambiguous response; never assume a transport error means no change occurred.

Events are append-only through the application. There is no audit-clear endpoint. They persist in PostgreSQL with their conversation, or only for the process lifetime with the explicit in-memory store. Records are bounded to 256 events per conversation and a 2 MiB conversation document; capacity errors do not prune older decisions. Export and use a new conversation before reaching those limits. Infrastructure failures or an exhausted log can prevent recording a failed attempt; successful mutations fail closed when their audit cannot be recorded.

This is an application integrity check, not signed storage or an externally protected immutable archive. A database administrator can modify or remove database contents. Include the database in backups and keep exports under your organization's retention/access policy. Exported actor names, review notes and configuration leave server-side access and retention controls.

## Reviewed configuration rollback

For an applied change set, select **Prepare rollback review** and provide a reason. The server constructs the restore plan from its saved previous definitions; a model does not supply the restore payload. Review the diff, enter a rollback approval note, approve its new digest, then explicitly apply it.

Rollback restores existing SLO/CUJ definitions as **new revisions**. It checks the exact revisions and digests from the original apply and refuses to overwrite any later edit. The restore batch is atomic and has its own retry receipt. Configuration verification identifies the restored revision, and the original apply and rollback remain in the audit trail. Historical change sets remain eligible for review while retained in the conversation.

Limits are explicit:

- **New resources:** creation has no previous definition. A rollback containing a newly created resource is blocked as a whole, with an explanation. It does not delete the SLO or partially unlink its journey. Resource retirement/deletion is not implemented by this feature.
- **External rules:** rollback restores Nines definitions only. Existing Prometheus/Mimir rules continue until a separate reviewed deployment updates them. Their integration status can show an out-of-date definition until then.
- **Measurements:** a restored definition follows normal revision-scoped measurement behavior. Fresh collection/coverage is required; cached observations are not reinterpreted as evidence for a different revision.
- **Error budgets:** restoring configuration cannot undo consumed budget, restore lost requests or erase an incident. There is no automatic rollback triggered by budget burn.
- **Intervening edits:** a conflict requires investigation and a new deliberate configuration change. Rollback does not rebase over another engineer's work.

Rollback approval can also be rejected or withdrawn before application. The rejected plan remains in history. A new preparation requires a new digest and review.

## CLI and Go client

All operations use the explicit server/tenant context. Operational YAML is separate from resource YAML. For example, inspect and approve an existing proposal:

```sh
nines intent conversations show CONVERSATION --context production --agent
nines intent conversations audit CONVERSATION --context production --agent

# approval.yaml
# expectedVersion: <conversation version>
# digest: <exact reviewed change-set digest>
# confirm: true
# reason: "Reviewed population, excluded traffic, target impact and available history."
nines intent conversations approve CONVERSATION --context production -f approval.yaml --agent

# Apply uses the new conversation version returned by approval.
nines intent conversations apply CONVERSATION --context production -f apply.yaml --agent
```

For rollback, `rollback-plan` takes the original applied change-set digest and a reason. `rollback-approve` takes the returned rollback digest and a reviewer reason. `rollback-apply` takes that rollback digest and the latest approved conversation version. Each request contains `confirm: true`; new rollback reviews never reuse original approval.

```sh
nines intent conversations reject CONVERSATION --context production -f rejection.yaml --agent
nines intent conversations rollback-plan CONVERSATION --context production -f rollback-plan.yaml --agent
nines intent conversations rollback-approve CONVERSATION --context production -f rollback-approval.yaml --agent
nines intent conversations rollback-apply CONVERSATION --context production -f rollback-apply.yaml --agent
nines intent conversations rollback-reject CONVERSATION --context production -f rollback-rejection.yaml --agent
```

The Go client exposes `IntentAudit`, `RejectIntentChangeSet`, `PrepareIntentRollback`, `ApproveIntentRollback`, `ApplyIntentRollback`, `RejectIntentRollback` and `ExportIntentRollbackKubernetes`. The MCP adapter exposes audit reads and existing read-only conversation evidence; it does not offer approval, apply or rollback mutation tools.

## Kubernetes operator and GitOps

For portal-managed operator destinations, apply the reviewed configuration rollback first, then review/publish its new SLO revision through **Rules**. The operator reconciles the new desired configuration; configuration persistence, rule activation and output evidence remain distinct.

For standalone operator/GitOps users, the rollback panel or CLI can export the exact restore definitions with the original/rollback digest annotations:

```sh
nines intent conversations rollback-kubernetes CONVERSATION \
  --context production --namespace team-commerce --digest ROLLBACK_DIGEST > rollback.yaml
```

This only creates a local artifact for a separate GitOps review. It does not connect to Kubernetes, apply resources, rewind Kubernetes generations or take ownership of portal-managed resources. Keep the existing GitOps revision and provider status evidence as part of the review.
